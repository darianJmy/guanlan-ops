package handlers

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"guanlan/internal/models"
)

type kubernetesConnectionConfig struct {
	TLSServerName      string `json:"tls_server_name,omitempty"`
	Namespace          string `json:"namespace"`
	CACertificate      string `json:"ca_certificate"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
}

var kubernetesNamespacePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func kubernetesConfig(ds models.Datasource) (kubernetesConnectionConfig, error) {
	var config struct {
		Kubernetes kubernetesConnectionConfig `json:"kubernetes"`
	}
	if ds.Config != "" && json.Unmarshal([]byte(ds.Config), &config) != nil {
		return config.Kubernetes, fmt.Errorf("Kubernetes 配置格式不正确")
	}
	cfg := config.Kubernetes
	if len(cfg.Namespace) > 63 || (cfg.Namespace != "" && !kubernetesNamespacePattern.MatchString(cfg.Namespace)) {
		return cfg, fmt.Errorf("命名空间必须为 1–63 位小写字母、数字或连字符，并以字母或数字开头和结尾")
	}
	return cfg, nil
}

func kubernetesTLSConfig(ds models.Datasource) (*tls.Config, error) {
	cfg, err := kubernetesConfig(ds)
	if err != nil {
		return nil, err
	}
	result := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.InsecureSkipVerify, ServerName: cfg.TLSServerName}
	if len(cfg.CACertificate) > 64<<10 {
		return nil, fmt.Errorf("CA 证书不能超过 64 KiB")
	}
	if cfg.CACertificate != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(cfg.CACertificate)) {
			return nil, fmt.Errorf("CA 证书必须为有效的 PEM 格式")
		}
		result.RootCAs = pool
	}
	if ds.AuthType == "client_certificate" {
		var auth DatasourceAuthConfig
		if json.Unmarshal([]byte(ds.AuthConfig), &auth) != nil {
			return nil, fmt.Errorf("客户端证书认证配置格式不正确")
		}
		if len(auth.ClientCertificate) > 64<<10 || len(auth.ClientKey) > 64<<10 {
			return nil, fmt.Errorf("客户端证书或私钥不能超过 64 KiB")
		}
		pair, err := tls.X509KeyPair([]byte(auth.ClientCertificate), []byte(auth.ClientKey))
		if err != nil {
			return nil, fmt.Errorf("请填写有效且匹配的 PEM 客户端证书和私钥")
		}
		result.Certificates = []tls.Certificate{pair}
	}
	return result, nil
}

func validateKubernetesConnection(ds models.Datasource) error {
	if !strings.HasPrefix(ds.URL, "https://") {
		return fmt.Errorf("Kubernetes API Server 请使用 HTTPS 地址")
	}
	if ds.AuthType != "bearer" && ds.AuthType != "client_certificate" {
		return fmt.Errorf("Kubernetes 支持 Bearer Token 或客户端证书认证")
	}
	_, err := kubernetesTLSConfig(ds)
	return err
}

// Do not return saved Kubernetes credentials to list/detail consumers.
func maskKubernetesAuth(ds *models.Datasource) {
	if ds.Type != "k8s" {
		return
	}
	var auth DatasourceAuthConfig
	_ = json.Unmarshal([]byte(ds.AuthConfig), &auth)
	masked, _ := json.Marshal(DatasourceAuthConfig{HasToken: auth.Token != "", HasClientCertificate: auth.ClientCertificate != "" && auth.ClientKey != ""})
	ds.AuthConfig = string(masked)
}

func testKubernetes(ds models.Datasource) (bool, string) {
	return testKubernetesContext(context.Background(), ds)
}

func testKubernetesContext(ctx context.Context, ds models.Datasource) (bool, string) {
	client, cleanup, err := datasourceHTTPClient(ds, 8*time.Second)
	if err != nil {
		return false, err.Error()
	}
	defer cleanup()
	// Never forward cluster credentials to a redirected endpoint.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	requestJSON := func(path string, target any) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(ds.URL, "/")+path, nil)
		if err != nil {
			return fmt.Errorf("Kubernetes 请求地址不正确")
		}
		req.Header.Set("Accept", "application/json")
		response, err := client.Do(req)
		if err != nil {
			var certError *tls.CertificateVerificationError
			if errors.As(err, &certError) {
				return fmt.Errorf("API Server 证书校验失败，请检查 CA 证书及接入地址")
			}
			return fmt.Errorf("无法连接 Kubernetes API Server，请检查网络、地址及 TLS 配置")
		}
		defer response.Body.Close()
		switch response.StatusCode {
		case http.StatusOK:
		case http.StatusUnauthorized:
			return fmt.Errorf("Kubernetes 认证失败（401），请检查 Token 是否过期或客户端证书是否有效")
		case http.StatusForbidden:
			return fmt.Errorf("Kubernetes 权限不足（403），请检查集群版本访问权限和目标命名空间的 Pod list 权限")
		case http.StatusNotFound:
			return fmt.Errorf("Kubernetes API 或命名空间不存在（404），请检查接入地址和命名空间")
		default:
			return fmt.Errorf("Kubernetes 返回 HTTP %d", response.StatusCode)
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(target); err != nil {
			return fmt.Errorf("不是有效的 Kubernetes API 响应")
		}
		return nil
	}
	var version struct {
		GitVersion string `json:"gitVersion"`
	}
	if err := requestJSON("/version", &version); err != nil {
		return false, err.Error()
	}
	if !strings.HasPrefix(version.GitVersion, "v") || len(version.GitVersion) > 100 {
		return false, "未返回有效的 Kubernetes 版本信息"
	}
	cfg, err := kubernetesConfig(ds)
	if err != nil {
		return false, err.Error()
	}
	path, scope := "/api/v1/pods?limit=1", "所有命名空间"
	if cfg.Namespace != "" {
		path = "/api/v1/namespaces/" + cfg.Namespace + "/pods?limit=1"
		scope = "命名空间 " + cfg.Namespace
	}
	var pods struct {
		Kind       string            `json:"kind"`
		APIVersion string            `json:"apiVersion"`
		Items      []json.RawMessage `json:"items"`
	}
	if err := requestJSON(path, &pods); err != nil {
		return false, err.Error()
	}
	if pods.Kind != "PodList" || pods.APIVersion != "v1" || pods.Items == nil {
		return false, "未返回有效的 Kubernetes Pod 列表"
	}
	return true, fmt.Sprintf("Kubernetes %s 连接成功，%s的 Pod 读取权限可用", version.GitVersion, scope)
}
