package handlers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

type kafkaConnectionConfig struct {
	BootstrapServers []string `json:"bootstrap_servers"`
	TLS              struct {
		Enabled       bool   `json:"enabled"`
		CACertificate string `json:"ca_certificate"`
	} `json:"tls"`
}

func kafkaBootstrapAddress(raw string, primary bool) (string, error) {
	value := raw
	if !primary {
		value = "kafka://" + raw
	}
	endpoint, err := url.Parse(value)
	if err != nil || endpoint.Scheme != "kafka" || endpoint.Hostname() == "" || endpoint.User != nil || (endpoint.Path != "" && endpoint.Path != "/") || endpoint.RawQuery != "" || endpoint.Fragment != "" || strings.ContainsAny(raw, "\r\n\t ,") {
		return "", fmt.Errorf("Kafka 地址格式不正确：接入地址使用 kafka://主机:端口，备用 Broker 使用主机:端口")
	}
	port := endpoint.Port()
	if port == "" {
		if !primary {
			return "", fmt.Errorf("备用 Broker 必须填写端口")
		}
		port = "9092"
	}
	valueInt, err := strconv.Atoi(port)
	if err != nil || valueInt < 1 || valueInt > 65535 {
		return "", fmt.Errorf("Kafka 端口必须在 1–65535 之间")
	}
	return net.JoinHostPort(endpoint.Hostname(), port), nil
}

func kafkaConnectionOptions(ds models.Datasource) ([]string, *kafka.Dialer, error) {
	first, err := kafkaBootstrapAddress(ds.URL, true)
	if err != nil {
		return nil, nil, err
	}
	var settings kafkaConnectionConfig
	if ds.Config != "" && json.Unmarshal([]byte(ds.Config), &settings) != nil {
		return nil, nil, fmt.Errorf("Kafka 连接配置格式不正确")
	}
	if len(settings.BootstrapServers) > 9 {
		return nil, nil, fmt.Errorf("Kafka 最多配置 10 个引导 Broker（含接入地址）")
	}
	addresses := []string{first}
	seen := map[string]bool{first: true}
	for _, value := range settings.BootstrapServers {
		address, err := kafkaBootstrapAddress(value, false)
		if err != nil {
			return nil, nil, err
		}
		if !seen[address] {
			addresses = append(addresses, address)
			seen[address] = true
		}
	}
	dialer := &kafka.Dialer{ClientID: "guanlan-ops", Timeout: 3 * time.Second}
	if settings.TLS.Enabled {
		dialer.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	if len(settings.TLS.CACertificate) > 64<<10 {
		return nil, nil, fmt.Errorf("Kafka CA 证书不能超过 64 KiB")
	}
	if settings.TLS.CACertificate != "" {
		if !settings.TLS.Enabled {
			return nil, nil, fmt.Errorf("配置 Kafka CA 证书时请开启 TLS")
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(settings.TLS.CACertificate)) {
			return nil, nil, fmt.Errorf("Kafka CA 证书必须为有效的 PEM 格式")
		}
		dialer.TLS.RootCAs = pool
	}
	var auth DatasourceAuthConfig
	if ds.AuthConfig != "" && json.Unmarshal([]byte(ds.AuthConfig), &auth) != nil {
		return nil, nil, fmt.Errorf("Kafka 认证配置格式不正确")
	}
	switch ds.AuthType {
	case "", "none":
	case "sasl_plain", "scram_sha256", "scram_sha512":
		if strings.TrimSpace(auth.Username) == "" || auth.Password == "" {
			return nil, nil, fmt.Errorf("请填写 Kafka SASL 用户名和密码")
		}
		switch ds.AuthType {
		case "sasl_plain":
			dialer.SASLMechanism = plain.Mechanism{Username: auth.Username, Password: auth.Password}
		case "scram_sha256":
			dialer.SASLMechanism, err = scram.Mechanism(scram.SHA256, auth.Username, auth.Password)
		case "scram_sha512":
			dialer.SASLMechanism, err = scram.Mechanism(scram.SHA512, auth.Username, auth.Password)
		}
		if err != nil {
			return nil, nil, fmt.Errorf("Kafka SASL 认证参数不正确")
		}
	default:
		return nil, nil, fmt.Errorf("Kafka 支持无认证、SASL/PLAIN、SCRAM-SHA-256 和 SCRAM-SHA-512")
	}
	return addresses, dialer, nil
}

func readKafkaBrokers(ctx context.Context, dialer kafka.Dialer, address string) ([]kafka.Broker, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	baseDial := dialer.DialFunc
	if baseDial == nil {
		baseDial = (&net.Dialer{Timeout: 3 * time.Second}).DialContext
	}
	transport := &kafka.Transport{
		ClientID: dialer.ClientID, TLS: dialer.TLS, SASL: dialer.SASLMechanism,
		DialTimeout: 3 * time.Second, Context: ctx, MetadataTopics: []string{},
		Dial: func(dialCtx context.Context, network, target string) (net.Conn, error) {
			connection, err := baseDial(dialCtx, network, target)
			if err == nil {
				// Covers cancellation during TLS/SASL negotiation as well as metadata reads.
				// The attempt context is always cancelled on return, releasing this callback.
				context.AfterFunc(ctx, func() { _ = connection.Close() })
			}
			return connection, err
		},
	}
	defer transport.CloseIdleConnections()
	client := kafka.Client{Transport: transport}
	// Transport negotiates API versions; fixed Metadata v1 is rejected by newer brokers.
	response, err := client.Metadata(ctx, &kafka.MetadataRequest{Addr: kafka.TCP(address), Topics: []string{}})
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	brokers := response.Brokers
	if len(brokers) == 0 {
		return nil, fmt.Errorf("Kafka 未返回 Broker 元数据")
	}
	for _, broker := range brokers {
		if strings.TrimSpace(broker.Host) == "" || broker.Port < 1 || broker.Port > 65535 {
			return nil, fmt.Errorf("Kafka 返回的 Broker 元数据无效")
		}
	}
	return brokers, nil
}

func kafkaConnectionError(err error) string {
	if errors.Is(err, context.Canceled) {
		return "Kafka 连接测试已取消"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Kafka 连接测试超时，请检查 Broker 地址和网络"
	}
	var certificateError *tls.CertificateVerificationError
	if errors.As(err, &certificateError) {
		return "Kafka TLS 证书校验失败，请检查 CA 和 Broker 主机名"
	}
	switch {
	case errors.Is(err, kafka.SASLAuthenticationFailed):
		return "Kafka SASL 认证失败，请检查用户名和密码"
	case errors.Is(err, kafka.UnsupportedSASLMechanism):
		return "Broker 不支持所选 SASL 认证方式"
	case errors.Is(err, kafka.ClusterAuthorizationFailed), errors.Is(err, kafka.TopicAuthorizationFailed), errors.Is(err, kafka.BrokerAuthorizationFailed):
		return "Kafka 元数据访问权限不足"
	}
	return "Kafka 连接或元数据读取失败，请检查 Broker 地址、认证方式和 TLS 配置"
}

func testKafka(ds models.Datasource) (bool, string) {
	return testKafkaContext(context.Background(), ds)
}
func testKafkaContext(ctx context.Context, ds models.Datasource) (bool, string) {
	addresses, dialer, err := kafkaConnectionOptions(ds)
	if err != nil {
		return false, err.Error()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if ds.TunnelID != 0 {
		var tunnel models.Tunnel
		if database.DB.First(&tunnel, ds.TunnelID).Error != nil || tunnel.Status != 1 || (tunnel.Type != "" && tunnel.Type != "ssh") {
			return false, "SSH 隧道不存在、已禁用或类型不受支持"
		}
		sshConnection, err := openSSHClient(tunnel)
		if err != nil {
			return false, "连接 SSH 隧道失败"
		}
		defer sshConnection.Close()
		dialer.DialFunc = sshConnection.DialContext
	}
	var lastError error
	for _, address := range addresses {
		if ctx.Err() != nil {
			return false, kafkaConnectionError(ctx.Err())
		}
		brokers, err := readKafkaBrokers(ctx, *dialer, address)
		if err == nil {
			return true, fmt.Sprintf("Kafka 引导连接成功，可读取 %d 个 Broker 的元数据", len(brokers))
		}
		lastError = err
	}
	return false, kafkaConnectionError(lastError)
}
