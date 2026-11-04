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

	"github.com/redis/go-redis/v9"
	"github.com/redis/go-redis/v9/maintnotifications"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

type redisConnectionConfig struct {
	Database int `json:"database"`
	TLS      struct {
		CACertificate string `json:"ca_certificate"`
	} `json:"tls"`
}

func redisClientOptions(ds models.Datasource) (*redis.Options, error) {
	endpoint, err := url.Parse(ds.URL)
	if err != nil || (endpoint.Scheme != "redis" && endpoint.Scheme != "rediss") || endpoint.Hostname() == "" || endpoint.User != nil || (endpoint.Path != "" && endpoint.Path != "/") || endpoint.RawQuery != "" || endpoint.Fragment != "" || strings.ContainsAny(ds.URL, "\r\n\t ") {
		return nil, fmt.Errorf("请填写 redis://主机:端口 或 rediss://主机:端口，认证和数据库编号请单独配置")
	}
	port := endpoint.Port()
	if port == "" {
		port = "6379"
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return nil, fmt.Errorf("Redis 端口必须在 1–65535 之间")
	}
	var settings redisConnectionConfig
	if ds.Config != "" && json.Unmarshal([]byte(ds.Config), &settings) != nil {
		return nil, fmt.Errorf("Redis 连接配置格式不正确")
	}
	if settings.Database < 0 || settings.Database > 65535 {
		return nil, fmt.Errorf("Redis 数据库编号必须是 0–65535 之间的整数")
	}
	var auth DatasourceAuthConfig
	if ds.AuthConfig != "" && json.Unmarshal([]byte(ds.AuthConfig), &auth) != nil {
		return nil, fmt.Errorf("Redis 认证配置格式不正确")
	}
	switch ds.AuthType {
	case "", "none":
		auth = DatasourceAuthConfig{}
	case "password":
		if auth.Password == "" {
			return nil, fmt.Errorf("请填写 Redis 密码")
		}
		auth.Username = ""
	case "basic":
		if strings.TrimSpace(auth.Username) == "" || auth.Password == "" {
			return nil, fmt.Errorf("请填写 Redis ACL 用户名和密码")
		}
	default:
		return nil, fmt.Errorf("Redis 支持无认证、密码认证或 ACL 用户名认证")
	}
	cfg := &redis.Options{
		Addr: net.JoinHostPort(endpoint.Hostname(), port), Username: auth.Username, Password: auth.Password, DB: settings.Database,
		Protocol: 2, DisableIdentity: true, ContextTimeoutEnabled: true, MaxRetries: -1,
		DialTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second,
		PoolSize: 1, MaxActiveConns: 1,
		MaintNotificationsConfig: &maintnotifications.Config{Mode: maintnotifications.ModeDisabled},
	}
	if endpoint.Scheme == "rediss" {
		cfg.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: endpoint.Hostname()}
	}
	if len(settings.TLS.CACertificate) > 64<<10 {
		return nil, fmt.Errorf("Redis CA 证书不能超过 64 KiB")
	}
	if settings.TLS.CACertificate != "" {
		if cfg.TLSConfig == nil {
			return nil, fmt.Errorf("配置 CA 证书时请使用 rediss:// 地址启用 TLS")
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(settings.TLS.CACertificate)) {
			return nil, fmt.Errorf("Redis CA 证书必须为有效的 PEM 格式")
		}
		cfg.TLSConfig.RootCAs = pool
	}
	return cfg, nil
}

func openRedis(ds models.Datasource) (*redis.Client, func(), error) {
	cfg, err := redisClientOptions(ds)
	if err != nil {
		return nil, func() {}, err
	}
	dialer := (&net.Dialer{Timeout: 5 * time.Second}).DialContext
	cleanup := func() {}
	if ds.TunnelID != 0 {
		var tunnel models.Tunnel
		if database.DB.First(&tunnel, ds.TunnelID).Error != nil || tunnel.Status != 1 || (tunnel.Type != "" && tunnel.Type != "ssh") {
			return nil, cleanup, fmt.Errorf("SSH 隧道不存在、已禁用或类型不受支持")
		}
		connection, err := openSSHClient(tunnel)
		if err != nil {
			return nil, cleanup, fmt.Errorf("连接 SSH 隧道失败")
		}
		dialer = connection.DialContext
		cleanup = func() { _ = connection.Close() }
	}
	// A custom dialer also owns TLS; go-redis does not wrap custom connections.
	cfg.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		conn, err := dialer(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		if cfg.TLSConfig == nil {
			return conn, nil
		}
		secured := tls.Client(conn, cfg.TLSConfig)
		if err := secured.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return secured, nil
	}
	client := redis.NewClient(cfg)
	return client, func() { _ = client.Close(); cleanup() }, nil
}

func redisConnectionError(err error) string {
	if errors.Is(err, context.Canceled) {
		return "Redis 连接测试已取消"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Redis 连接测试超时"
	}
	var certificateError *tls.CertificateVerificationError
	if errors.As(err, &certificateError) {
		return "Redis TLS 证书校验失败，请检查 CA 和主机名"
	}
	switch {
	case redis.HasErrorPrefix(err, "WRONGPASS"), redis.HasErrorPrefix(err, "NOAUTH"):
		return "Redis 认证失败，请检查认证方式、用户名和密码"
	case redis.HasErrorPrefix(err, "NOPERM"):
		return "Redis 权限不足，请检查账号的 PING 和数据库选择权限"
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "db index") || strings.Contains(message, "invalid database") {
		return "Redis 数据库编号无效或超出服务端配置范围"
	}
	if strings.Contains(message, "auth") {
		return "Redis 认证失败，请检查服务端是否启用密码和所选认证方式"
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "Redis 连接测试超时"
	}
	return "Redis 连接失败，请检查地址、网络和 TLS 配置"
}

func testRedis(ds models.Datasource) (bool, string) {
	return testRedisContext(context.Background(), ds)
}
func testRedisContext(ctx context.Context, ds models.Datasource) (bool, string) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	client, cleanup, err := openRedis(ds)
	if err != nil {
		return false, err.Error()
	}
	defer cleanup()
	pong, err := client.Ping(ctx).Result()
	if err != nil {
		return false, redisConnectionError(err)
	}
	if pong != "PONG" {
		return false, "Redis 未返回有效的 PONG 响应"
	}
	return true, fmt.Sprintf("Redis 连接成功，数据库 %d 可用（PING → PONG）", client.Options().DB)
}
