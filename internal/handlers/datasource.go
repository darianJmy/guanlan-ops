package handlers

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/httpclient"
	"guanlan/pkg/resp"
	"guanlan/pkg/textutil"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"gopkg.in/yaml.v3"
)

func ListDatasources(c *gin.Context) {
	var datasources []models.Datasource
	database.DB.Where("type IN ?", supportedDatasourceTypes()).Order("id ASC").Find(&datasources)
	resp.Success(c, http.StatusOK, "成功", datasources)
}

type CreateDatasourceReq struct {
	Name         string `json:"name" binding:"required"`
	Category     string `json:"category"`
	Type         string `json:"type" binding:"required"`
	URL          string `json:"url"`
	AuthType     string `json:"auth_type"`
	AuthConfig   string `json:"auth_config"`
	Labels       string `json:"labels"`
	Region       string `json:"region"`
	Env          string `json:"env"`
	Domain       string `json:"domain"`
	Capabilities string `json:"capabilities"`
	Priority     int    `json:"priority"`
	Status       int    `json:"status"`
	Remark       string `json:"remark"`
}

func CreateDatasource(c *gin.Context) { CreateDatasourceWithTunnel(c) }
func UpdateDatasource(c *gin.Context) { UpdateDatasourceWithTunnel(c) }

func DeleteDatasource(c *gin.Context) {
	id := c.Param("id")
	numericID, err := strconv.ParseUint(id, 10, 64)
	if err != nil || numericID == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "实例 ID 不正确")
		return
	}
	if !requireSupportedDatasourceIDs(c, []uint{uint(numericID)}) {
		return
	}
	var groupCount int64
	database.DB.Model(&models.DatasourceGroupMember{}).Where("datasource_id = ?", id).Count(&groupCount)
	if groupCount > 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "该接入仍属于数据源分组，请先从数据源分组移除")
		return
	}
	if !requireUnboundDatasources(c, []uint{uint(numericID)}) {
		return
	}
	database.DB.Delete(&models.Datasource{}, id)
	resp.Success(c, http.StatusOK, "删除成功")
}

type TestConnectionReq struct {
	Type       string `json:"type" binding:"required"`
	URL        string `json:"url"`
	AuthType   string `json:"auth_type"`
	AuthConfig string `json:"auth_config"`
	Config     string `json:"config"`
	TunnelID   uint   `json:"tunnel_id"`
}

type kubeconfigAuth struct {
	Kubeconfig string `json:"kubeconfig"`
	Token      string `json:"token"`
}

type kubeconfigFile struct {
	Clusters []struct {
		Name    string `yaml:"name"`
		Cluster struct {
			Server string `yaml:"server"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
	Users []struct {
		Name string `yaml:"name"`
		User struct {
			Token string `yaml:"token"`
		} `yaml:"user"`
	} `yaml:"users"`
}

func TestDatasourceConnection(c *gin.Context) {
	var req TestConnectionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "参数错误")
		return
	}

	ds := models.Datasource{
		Type:       req.Type,
		URL:        req.URL,
		AuthType:   req.AuthType,
		AuthConfig: req.AuthConfig,
		Config:     req.Config,
		TunnelID:   req.TunnelID,
	}
	ok, msg := testConnectionForDatasource(ds)
	if ok {
		resp.Success(c, http.StatusOK, msg)
	} else {
		resp.Error(c, http.StatusOK, resp.CodeFailure, msg)
	}
}

func TestSavedDatasourceConnection(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var ds models.Datasource
	if err := database.DB.Where("type IN ?", supportedDatasourceTypes()).First(&ds, id).Error; err != nil {
		resp.Error(c, http.StatusOK, resp.CodeNotFound, "数据源不存在")
		return
	}

	var req TestConnectionReq
	_ = c.ShouldBindJSON(&req)
	if strings.TrimSpace(req.URL) != "" {
		ds.URL = req.URL
	}
	if strings.TrimSpace(req.Config) != "" {
		ds.Config = req.Config
	}
	if strings.TrimSpace(req.Type) != "" {
		ds.Type = req.Type
	}
	if strings.TrimSpace(req.AuthType) != "" {
		if ds.AuthType != req.AuthType {
			ds.AuthConfig = ""
		}
		ds.AuthType = req.AuthType
	}
	if strings.TrimSpace(req.AuthConfig) != "" {
		ds.AuthConfig = req.AuthConfig
	}

	ok, msg := testConnectionForDatasource(ds)
	if ok {
		resp.Success(c, http.StatusOK, msg)
	} else {
		resp.Error(c, http.StatusOK, resp.CodeFailure, msg)
	}
}

func testConnection(dsType, url, authType, authConfig, config string) (bool, string) {
	return testConnectionForDatasource(models.Datasource{
		Type:       dsType,
		URL:        url,
		AuthType:   authType,
		AuthConfig: authConfig,
		Config:     config,
	})
}

func testConnectionForDatasource(ds models.Datasource) (bool, string) {
	if err := validateDatasourceConnection(ds); err != nil {
		return false, err.Error()
	}
	definition, _ := datasourceDefinitionFor(ds.Type)
	return definition.testConnection(ds)
}

func testK8s(endpoint, authType, authConfigJSON string) (bool, string) {
	var token string
	if authType == "kubeconfig" {
		endpoint = firstNonEmpty(strings.TrimSpace(endpoint), k8sEndpointFromAuthConfig(authConfigJSON))
		token = k8sTokenFromAuthConfig(authConfigJSON)
		if endpoint == "" {
			return false, "Kubernetes kubeconfig 中未找到 clusters[].cluster.server"
		}
	} else if authType == "bearer" {
		var auth kubeconfigAuth
		_ = json.Unmarshal([]byte(authConfigJSON), &auth)
		token = auth.Token
		if endpoint == "" || token == "" {
			return false, "Kubernetes Bearer Token 模式请填写 API Server 地址和 Token"
		}
	}
	if endpoint == "" {
		return false, "Kubernetes 接入地址为空"
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	req, err := http.NewRequest("GET", strings.TrimRight(endpoint, "/")+"/version", nil)
	if err != nil {
		return false, "Kubernetes 请求构造失败: " + err.Error()
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, "Kubernetes 连接失败: " + err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return true, "连接成功 (Kubernetes)"
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false, "Kubernetes 认证失败或权限不足，HTTP " + strconv.Itoa(resp.StatusCode)
	}
	return true, "Kubernetes API Server 可访问，HTTP " + strconv.Itoa(resp.StatusCode)
}

func k8sEndpointFromAuthConfig(authConfigJSON string) string {
	var auth kubeconfigAuth
	if err := json.Unmarshal([]byte(authConfigJSON), &auth); err != nil || strings.TrimSpace(auth.Kubeconfig) == "" {
		return ""
	}
	var cfg kubeconfigFile
	if err := yaml.Unmarshal([]byte(auth.Kubeconfig), &cfg); err != nil {
		return ""
	}
	for _, cluster := range cfg.Clusters {
		if strings.TrimSpace(cluster.Cluster.Server) != "" {
			return strings.TrimSpace(cluster.Cluster.Server)
		}
	}
	return ""
}

func k8sTokenFromAuthConfig(authConfigJSON string) string {
	var auth kubeconfigAuth
	if err := json.Unmarshal([]byte(authConfigJSON), &auth); err != nil {
		return ""
	}
	if strings.TrimSpace(auth.Token) != "" {
		return strings.TrimSpace(auth.Token)
	}
	var cfg kubeconfigFile
	if err := yaml.Unmarshal([]byte(auth.Kubeconfig), &cfg); err != nil {
		return ""
	}
	for _, user := range cfg.Users {
		if strings.TrimSpace(user.User.Token) != "" {
			return strings.TrimSpace(user.User.Token)
		}
	}
	return ""
}

func categoryByType(dsType string) string {
	switch dsType {
	case "prometheus", "victoriametrics", "loki", "elasticsearch", "jaeger", "opentelemetry", "sls", "cls", "trace":
		return "observability"
	case "mysql", "clickhouse", "postgresql", "redis", "influxdb":
		return "database"
	case "aliyun_ecs", "aliyun_cdn", "aliyun_dcdn", "aliyun_slb", "aliyun_alb", "aliyun_nlb":
		return "cloud"
	case "k8s":
		return "kubernetes"
	case "http", "grpc", "tcp", "icmp", "dns":
		return "network"
	case "kafka", "rabbitmq", "nacos", "consul":
		return "middleware"
	default:
		return "other"
	}
}

func normalizeCapabilities(value, dsType string) string {
	seen := map[string]bool{}
	items := make([]string, 0)
	add := func(raw string) {
		capability := strings.ToLower(strings.TrimSpace(raw))
		if capability == "" || seen[capability] {
			return
		}
		seen[capability] = true
		items = append(items, capability)
	}
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == ' '
	}) {
		add(part)
	}
	if len(items) == 0 {
		for _, capability := range defaultCapabilitiesByType(dsType) {
			add(capability)
		}
	}
	return strings.Join(items, ",")
}

func defaultCapabilitiesByType(dsType string) []string {
	switch dsType {
	case "prometheus", "victoriametrics":
		return []string{"metrics"}
	case "sls", "cls", "loki", "elasticsearch":
		return []string{"logs"}
	case "jaeger", "opentelemetry":
		return []string{"traces"}
	case "k8s":
		return []string{"k8s"}
	case "nacos", "consul":
		return []string{"registry"}
	case "kafka":
		return []string{"kafka"}
	case "http", "grpc", "tcp", "icmp", "dns":
		return []string{"probe"}
	case "mysql", "clickhouse", "postgresql", "redis", "influxdb":
		return []string{"database"}
	default:
		return nil
	}
}

func priorityOrDefault(priority int) int {
	if priority <= 0 {
		return 100
	}
	return priority
}

func isHTTPURL(raw string) bool {
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}

func testPrometheus(ds models.Datasource) (bool, string) {
	client, cleanup, err := datasourceHTTPClient(ds, 5*time.Second)
	if err != nil {
		return false, err.Error()
	}
	defer cleanup()
	response, err := client.Get(strings.TrimRight(ds.URL, "/") + "/api/v1/status/buildinfo")
	if err != nil {
		return false, "连接失败: " + err.Error()
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false, fmt.Sprintf("Prometheus 返回 HTTP %d", response.StatusCode)
	}
	var result struct {
		Status string `json:"status"`
		Data   struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&result); err != nil {
		return false, "不是有效的 Prometheus API 响应"
	}
	if result.Status != "success" || result.Data.Version == "" {
		return false, "未返回有效的 Prometheus 版本信息，请检查接入地址"
	}
	return true, "连接成功 (Prometheus " + result.Data.Version + ")"
}

func testHTTP(ds models.Datasource) (bool, string) {
	client, cleanup, err := datasourceHTTPClient(ds, 5*time.Second)
	if err != nil {
		return false, err.Error()
	}
	defer cleanup()

	resp, err := client.Get(ds.URL)
	if err != nil {
		return false, "连接失败: " + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return true, "连接成功 (HTTP " + strconv.Itoa(resp.StatusCode) + ")"
	}
	return false, "返回异常状态码: " + strconv.Itoa(resp.StatusCode)
}

func testNacos(ds models.Datasource) (bool, string) {
	baseURL := normalizeNacosBaseURL(ds.URL)
	if baseURL == "" {
		return false, "Nacos 接入地址为空"
	}

	client, cleanup, err := datasourceHTTPClient(ds, 5*time.Second)
	if err != nil {
		return false, err.Error()
	}
	defer cleanup()

	paths := []string{
		"/v1/ns/operator/metrics",
		"/v1/console/health/readiness",
		"/v1/console/health/liveness",
	}
	if token := nacosAccessToken(client, baseURL, ds.AuthType, ds.AuthConfig); token != "" {
		for _, path := range paths {
			if ok, msg := testNacosPath(client, baseURL, path, ds.AuthType, ds.AuthConfig, token); ok {
				return true, msg + "，认证成功"
			}
		}
	}

	var lastMsg string
	for _, path := range paths {
		ok, msg := testNacosPath(client, baseURL, path, ds.AuthType, ds.AuthConfig, "")
		if ok {
			return true, msg
		}
		lastMsg = msg
	}
	if lastMsg == "" {
		lastMsg = "Nacos 连接失败"
	}
	return false, lastMsg
}

func normalizeNacosBaseURL(rawURL string) string {
	value := strings.TrimRight(strings.TrimSpace(rawURL), "/")
	if value == "" {
		return ""
	}
	if strings.HasSuffix(value, "/nacos") {
		return value
	}
	return value + "/nacos"
}

func nacosAccessToken(client *http.Client, baseURL, authType, authConfig string) string {
	if authType != "basic" {
		return ""
	}
	var auth map[string]string
	if err := json.Unmarshal([]byte(authConfig), &auth); err != nil {
		return ""
	}
	username := strings.TrimSpace(auth["username"])
	password := strings.TrimSpace(auth["password"])
	if username == "" || password == "" {
		return ""
	}

	form := strings.NewReader("username=" + url.QueryEscape(username) + "&password=" + url.QueryEscape(password))
	req, err := http.NewRequest("POST", baseURL+"/v1/auth/users/login", form)
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	body, _ := io.ReadAll(resp.Body)
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ""
	}
	if token, ok := parsed["accessToken"].(string); ok {
		return strings.TrimSpace(token)
	}
	return ""
}

func testNacosPath(client *http.Client, baseURL, path, authType, authConfig, token string) (bool, string) {
	endpoint := baseURL + path
	if token != "" {
		separator := "?"
		if strings.Contains(endpoint, "?") {
			separator = "&"
		}
		endpoint += separator + "accessToken=" + url.QueryEscape(token)
	}
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return false, "Nacos 请求构造失败: " + err.Error()
	}
	setAuthHeader(req, authType, authConfig)

	resp, err := client.Do(req)
	if err != nil {
		return false, "Nacos 连接失败: " + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return true, "连接成功 (Nacos " + path + ", HTTP " + strconv.Itoa(resp.StatusCode) + ")"
	}
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false, "Nacos 认证失败或权限不足，HTTP " + strconv.Itoa(resp.StatusCode) + ": " + textutil.Ellipsis(string(body), 200)
	}
	return false, "Nacos 返回异常状态码 HTTP " + strconv.Itoa(resp.StatusCode) + " (" + path + "): " + textutil.Ellipsis(string(body), 200)
}

type nacosServiceListResult struct {
	Namespace string   `json:"namespace,omitempty"`
	Group     string   `json:"group,omitempty"`
	Count     int      `json:"count"`
	Services  []string `json:"services"`
}

func listNacosServices(ds models.Datasource, namespace, group string, pageSize int) (nacosServiceListResult, error) {
	if pageSize <= 0 || pageSize > 1000 {
		pageSize = 500
	}
	baseURL := normalizeNacosBaseURL(ds.URL)
	if baseURL == "" {
		return nacosServiceListResult{}, fmt.Errorf("Nacos 接入地址为空")
	}
	client, cleanup, err := datasourceHTTPClient(ds, 10*time.Second)
	if err != nil {
		return nacosServiceListResult{}, err
	}
	defer cleanup()
	token := nacosAccessToken(client, baseURL, ds.AuthType, ds.AuthConfig)

	query := url.Values{}
	query.Set("pageNo", "1")
	query.Set("pageSize", strconv.Itoa(pageSize))
	if strings.TrimSpace(namespace) != "" {
		query.Set("namespaceId", strings.TrimSpace(namespace))
	}
	if strings.TrimSpace(group) != "" {
		query.Set("groupName", strings.TrimSpace(group))
	}
	if token != "" {
		query.Set("accessToken", token)
	}

	endpoint := baseURL + "/v1/ns/service/list?" + query.Encode()
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nacosServiceListResult{}, fmt.Errorf("Nacos 请求构造失败: %w", err)
	}
	setAuthHeader(req, ds.AuthType, ds.AuthConfig)
	resp, err := client.Do(req)
	if err != nil {
		return nacosServiceListResult{}, fmt.Errorf("Nacos 服务列表查询失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nacosServiceListResult{}, fmt.Errorf("Nacos HTTP %d: %s", resp.StatusCode, textutil.Ellipsis(string(body), 300))
	}

	var parsed struct {
		Count int      `json:"count"`
		Doms  []string `json:"doms"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nacosServiceListResult{}, fmt.Errorf("解析 Nacos 服务列表失败: %w", err)
	}
	return nacosServiceListResult{
		Namespace: namespace,
		Group:     group,
		Count:     parsed.Count,
		Services:  parsed.Doms,
	}, nil
}

func testMySQL(dsn string) (bool, string) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return false, "MySQL DSN 解析失败: " + err.Error()
	}
	defer db.Close()
	db.SetConnMaxLifetime(5 * time.Second)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		return false, "MySQL 连接失败: " + err.Error()
	}
	return true, "连接成功 (MySQL)"
}

func testTCP(address string) (bool, string) {
	conn, err := net.DialTimeout("tcp", address, 5*time.Second)
	if err != nil {
		return false, "TCP 连接失败: " + err.Error()
	}
	conn.Close()
	return true, "连接成功 (TCP)"
}

func testDNS(domain string) (bool, string) {
	ips, err := net.LookupHost(domain)
	if err != nil {
		return false, "DNS 解析失败: " + err.Error()
	}
	return true, fmt.Sprintf("解析成功 (%d 条记录)", len(ips))
}

func testICMP(host string) (bool, string) {
	conn, err := net.DialTimeout("ip4:icmp", host, 3*time.Second)
	if err != nil {
		return false, "ICMP 探测失败: " + err.Error()
	}
	conn.Close()
	return true, "ICMP 探测可用"
}

// ── Jaeger ──

func testJaeger(ds models.Datasource) (bool, string) {
	client, cleanup, err := datasourceHTTPClient(ds, 5*time.Second)
	if err != nil {
		return false, err.Error()
	}
	defer cleanup()

	endpoint := strings.TrimRight(ds.URL, "/") + "/api/services"
	req, _ := http.NewRequest("GET", endpoint, nil)

	resp, err := client.Do(req)
	if err != nil {
		return false, "Jaeger 连接失败: " + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		var services []string
		if json.Unmarshal(body, &services) == nil {
			return true, fmt.Sprintf("连接成功 (Jaeger, %d services)", len(services))
		}
		return true, "连接成功 (Jaeger)"
	}
	return false, "Jaeger 返回异常状态码: " + strconv.Itoa(resp.StatusCode)
}

// ── Loki ──

func testLoki(ds models.Datasource) (bool, string) {
	client, cleanup, err := datasourceHTTPClient(ds, 5*time.Second)
	if err != nil {
		return false, "无法连接 Loki 数据源"
	}
	defer cleanup()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Get(strings.TrimRight(ds.URL, "/") + "/loki/api/v1/labels")
	if err != nil {
		return false, "Loki 连接失败或超时"
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, "Loki 返回异常状态码: " + strconv.Itoa(resp.StatusCode)
	}
	var result struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result) != nil || result.Status != "success" {
		return false, "不是有效的 Loki 标签 API 响应"
	}
	return true, "连接成功 (Loki)"
}

// ── SLS (Alibaba Cloud) ──

type slsConfig struct {
	Project  string `json:"project"`
	Logstore string `json:"logstore"`
	Region   string `json:"region"`
}

type slsAuth struct {
	AccessKeyID     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	SecurityToken   string `json:"security_token"`
}

func testSLS(endpoint, authConfigJSON, configJSON string) (bool, string) {
	var auth slsAuth
	if err := json.Unmarshal([]byte(authConfigJSON), &auth); err != nil || auth.AccessKeyID == "" {
		return false, "SLS 认证信息不完整，请填写 AccessKey ID 和 Secret"
	}
	var cfg slsConfig
	json.Unmarshal([]byte(configJSON), &cfg)
	if cfg.Project == "" {
		return false, "SLS 配置不完整，请填写 Project"
	}
	if strings.TrimSpace(endpoint) == "" {
		if cfg.Region == "" {
			return false, "SLS 配置不完整，请填写 Region 或 Endpoint"
		}
		endpoint = fmt.Sprintf("https://%s.%s.log.aliyuncs.com", cfg.Project, cfg.Region)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	uri := "/logstores"
	if cfg.Logstore != "" {
		uri = "/logstores/" + cfg.Logstore
	}
	fullURL := strings.TrimRight(endpoint, "/") + uri
	req, _ := http.NewRequest("GET", fullURL, nil)
	if strings.TrimSpace(auth.SecurityToken) != "" {
		req.Header.Set("x-acs-security-token", auth.SecurityToken)
	}

	// Alibaba Cloud Log Service Signature V1
	signSLSRequest(req, auth.AccessKeyID, auth.AccessKeySecret)

	resp, err := client.Do(req)
	if err != nil {
		return false, "SLS 连接失败: " + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		extra := ""
		if cfg.Logstore != "" {
			extra = ", logstore 存在"
		}
		return true, "连接成功 (SLS" + extra + ")"
	}
	if resp.StatusCode == http.StatusNotFound && cfg.Logstore != "" {
		return false, "Logstore 不存在: " + cfg.Logstore
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "InvalidAccessKeyId") || strings.Contains(string(body), "SignatureDoesNotMatch") {
		return false, "SLS 认证失败: AccessKey 无效或签名错误"
	}
	return false, "SLS 返回异常状态码: " + strconv.Itoa(resp.StatusCode) + " " + string(body)[:min(100, len(body))]
}

func listSLSLogstores(ds models.Datasource) ([]string, error) {
	var auth slsAuth
	if err := json.Unmarshal([]byte(ds.AuthConfig), &auth); err != nil || auth.AccessKeyID == "" || auth.AccessKeySecret == "" {
		return nil, fmt.Errorf("SLS 认证信息不完整")
	}
	var cfg slsConfig
	if err := json.Unmarshal([]byte(ds.Config), &cfg); err != nil {
		return nil, fmt.Errorf("SLS 配置解析失败: %w", err)
	}
	if cfg.Project == "" {
		return nil, fmt.Errorf("SLS 配置不完整，请填写 Project")
	}
	endpoint := strings.TrimSpace(ds.URL)
	if endpoint == "" {
		if cfg.Region == "" {
			return nil, fmt.Errorf("SLS 配置不完整，请填写 Region 或 Endpoint")
		}
		endpoint = fmt.Sprintf("https://%s.%s.log.aliyuncs.com", cfg.Project, cfg.Region)
	}

	req, err := http.NewRequest("GET", strings.TrimRight(endpoint, "/")+"/logstores", nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(auth.SecurityToken) != "" {
		req.Header.Set("x-acs-security-token", auth.SecurityToken)
	}
	signSLSRequest(req, auth.AccessKeyID, auth.AccessKeySecret)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 SLS 失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SLS 返回 HTTP %d: %s", resp.StatusCode, textutil.Ellipsis(string(body), 300))
	}

	var parsed struct {
		Logstores []string `json:"logstores"`
		Count     int      `json:"count"`
		Total     int      `json:"total"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("解析 SLS 响应失败: %w", err)
	}
	return parsed.Logstores, nil
}

// ── CLS (Tencent Cloud) ──

type clsConfig struct {
	TopicID string `json:"topic_id"`
	Region  string `json:"region"`
}

type clsAuth struct {
	SecretID  string `json:"secret_id"`
	SecretKey string `json:"secret_key"`
}

type clsLogEntry struct {
	Time     string `json:"time"`
	TopicID  string `json:"topic_id,omitempty"`
	Source   string `json:"source,omitempty"`
	FileName string `json:"filename,omitempty"`
	Content  string `json:"content"`
}

type clsSearchResult struct {
	Context  string        `json:"context"`
	Listover bool          `json:"listover"`
	Entries  []clsLogEntry `json:"entries"`
	RawCount int           `json:"raw_count"`
}

func testCLS(endpoint, authConfigJSON, configJSON string) (bool, string) {
	var auth clsAuth
	if err := json.Unmarshal([]byte(authConfigJSON), &auth); err != nil || auth.SecretID == "" {
		return false, "CLS 认证信息不完整，请填写 Secret ID 和 Key"
	}
	var cfg clsConfig
	json.Unmarshal([]byte(configJSON), &cfg)
	cfg.Region = normalizeTencentRegion(cfg.Region)
	if cfg.Region == "" {
		return false, "CLS 配置不完整，请填写 Region，如 ap-shanghai"
	}
	if strings.TrimSpace(endpoint) == "" {
		endpoint = "https://cls.tencentcloudapi.com"
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	// Call DescribeTopics to verify connection
	payload := `{}`
	req, err := http.NewRequest("POST", strings.TrimRight(endpoint, "/"), strings.NewReader(payload))
	if err != nil {
		return false, "CLS 请求构造失败: " + err.Error()
	}

	signCLSRequest(req, auth.SecretID, auth.SecretKey, "DescribeTopics", cfg.Region, payload)

	resp, err := client.Do(req)
	if err != nil {
		return false, "CLS 连接失败: " + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, "连接成功 (CLS)"
	}
	body, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(body), "AuthFailure") || strings.Contains(string(body), "InvalidParameterValue.SecretId") {
		return false, "CLS 认证失败: SecretId 或 SecretKey 无效"
	}
	// Any non-auth error means connection is OK (credentials work)
	return true, "连接成功 (CLS HTTP " + strconv.Itoa(resp.StatusCode) + ")"
}

func searchCLSLogs(ds models.Datasource, from, to time.Time, query string, limit int) (clsSearchResult, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var auth clsAuth
	if err := json.Unmarshal([]byte(ds.AuthConfig), &auth); err != nil || auth.SecretID == "" || auth.SecretKey == "" {
		return clsSearchResult{}, fmt.Errorf("CLS 认证信息不完整")
	}
	var cfg clsConfig
	if err := json.Unmarshal([]byte(ds.Config), &cfg); err != nil {
		return clsSearchResult{}, fmt.Errorf("CLS 配置解析失败: %w", err)
	}
	cfg.Region = normalizeTencentRegion(cfg.Region)
	if cfg.TopicID == "" {
		return clsSearchResult{}, fmt.Errorf("CLS 配置不完整，请填写 Topic ID")
	}
	if cfg.Region == "" {
		return clsSearchResult{}, fmt.Errorf("CLS 配置不完整，请填写 Region")
	}

	payloadBytes, _ := json.Marshal(map[string]interface{}{
		"TopicId":        cfg.TopicID,
		"From":           from.UnixMilli(),
		"To":             to.UnixMilli(),
		"QueryString":    query,
		"Limit":          limit,
		"Sort":           "desc",
		"HighLight":      true,
		"UseNewAnalysis": true,
	})
	payload := string(payloadBytes)
	endpoint := strings.TrimSpace(ds.URL)
	if endpoint == "" {
		endpoint = "https://cls.tencentcloudapi.com"
	}
	req, err := http.NewRequest("POST", strings.TrimRight(endpoint, "/"), strings.NewReader(payload))
	if err != nil {
		return clsSearchResult{}, fmt.Errorf("CLS 请求构造失败: %w", err)
	}
	signCLSRequest(req, auth.SecretID, auth.SecretKey, "SearchLog", cfg.Region, payload)

	client := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return clsSearchResult{}, fmt.Errorf("CLS 查询失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return clsSearchResult{}, fmt.Errorf("读取 CLS 响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return clsSearchResult{}, fmt.Errorf("CLS HTTP %d: %s", resp.StatusCode, textutil.Ellipsis(string(body), 500))
	}

	var parsed struct {
		Response struct {
			Context  string `json:"Context"`
			Listover bool   `json:"Listover"`
			Results  []struct {
				Timestamp string                 `json:"Timestamp"`
				TopicID   string                 `json:"TopicId"`
				Source    string                 `json:"Source"`
				FileName  string                 `json:"FileName"`
				LogJson   string                 `json:"LogJson"`
				Data      string                 `json:"Data"`
				Content   string                 `json:"Content"`
				Values    map[string]interface{} `json:"Values"`
			} `json:"Results"`
			RequestID string `json:"RequestId"`
			Error     *struct {
				Code    string `json:"Code"`
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"Response"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return clsSearchResult{}, fmt.Errorf("解析 CLS 响应失败: %w; body=%s", err, textutil.Ellipsis(string(body), 500))
	}
	if parsed.Response.Error != nil {
		return clsSearchResult{}, fmt.Errorf("CLS 查询失败: %s %s", parsed.Response.Error.Code, parsed.Response.Error.Message)
	}

	result := clsSearchResult{
		Context:  parsed.Response.Context,
		Listover: parsed.Response.Listover,
		RawCount: len(parsed.Response.Results),
		Entries:  make([]clsLogEntry, 0, len(parsed.Response.Results)),
	}
	for _, item := range parsed.Response.Results {
		content := firstNonEmpty(item.LogJson, item.Content, item.Data)
		if content == "" && len(item.Values) > 0 {
			raw, _ := json.Marshal(item.Values)
			content = string(raw)
		}
		result.Entries = append(result.Entries, clsLogEntry{
			Time:     item.Timestamp,
			TopicID:  item.TopicID,
			Source:   item.Source,
			FileName: item.FileName,
			Content:  content,
		})
	}
	return result, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeTencentRegion(region string) string {
	value := strings.TrimSpace(region)
	switch strings.ToLower(value) {
	case "shanghai":
		return "ap-shanghai"
	case "guangzhou":
		return "ap-guangzhou"
	case "beijing":
		return "ap-beijing"
	case "chengdu":
		return "ap-chengdu"
	case "chongqing":
		return "ap-chongqing"
	case "nanjing":
		return "ap-nanjing"
	case "hongkong":
		return "ap-hongkong"
	case "singapore":
		return "ap-singapore"
	case "bangkok":
		return "ap-bangkok"
	case "seoul":
		return "ap-seoul"
	case "tokyo":
		return "ap-tokyo"
	}
	switch value {
	case "上海":
		return "ap-shanghai"
	case "广州":
		return "ap-guangzhou"
	case "北京":
		return "ap-beijing"
	case "成都":
		return "ap-chengdu"
	case "重庆":
		return "ap-chongqing"
	case "南京":
		return "ap-nanjing"
	case "香港":
		return "ap-hongkong"
	default:
		return value
	}
}

// ── Auth helper ──

// ── SLS request signing (Alibaba Cloud Signature V1) ──

func signSLSRequest(req *http.Request, accessKeyID, accessKeySecret string) {
	req.Header.Set("x-log-apiversion", "0.6.0")
	req.Header.Set("x-log-bodyrawsize", "0")
	req.Header.Set("x-log-signaturemethod", "hmac-sha1")

	date := time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	req.Header.Set("Date", date)

	contentMD5 := req.Header.Get("Content-MD5")
	contentType := req.Header.Get("Content-Type")
	signStr := req.Method + "\n" +
		contentMD5 + "\n" +
		contentType + "\n" +
		date + "\n" +
		canonicalizedSLSHeaders(req) +
		canonicalizedSLSResource(req)

	mac := hmac.New(sha1.New, []byte(accessKeySecret))
	mac.Write([]byte(signStr))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req.Header.Set("Authorization", "LOG "+accessKeyID+":"+signature)
}

func canonicalizedSLSHeaders(req *http.Request) string {
	var keys []string
	for key := range req.Header {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "x-log-") || strings.HasPrefix(lower, "x-acs-") {
			keys = append(keys, lower)
		}
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, key := range keys {
		values := req.Header.Values(key)
		if len(values) == 0 {
			values = req.Header.Values(http.CanonicalHeaderKey(key))
		}
		sb.WriteString(key)
		sb.WriteString(":")
		sb.WriteString(strings.Join(values, ","))
		sb.WriteString("\n")
	}
	return sb.String()
}

func canonicalizedSLSResource(req *http.Request) string {
	resource := req.URL.EscapedPath()
	if resource == "" {
		resource = "/"
	}
	query := req.URL.Query()
	if len(query) == 0 {
		return resource
	}

	var keys []string
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var parts []string
	for _, key := range keys {
		values := query[key]
		sort.Strings(values)
		if len(values) == 0 {
			parts = append(parts, key)
			continue
		}
		for _, value := range values {
			if value == "" {
				parts = append(parts, key)
			} else {
				parts = append(parts, key+"="+value)
			}
		}
	}
	return resource + "?" + strings.Join(parts, "&")
}

// ── CLS request signing (Tencent Cloud TC3-HMAC-SHA256) ──

func signCLSRequest(req *http.Request, secretID, secretKey, action, region, payload string) {
	host := req.URL.Host
	algorithm := "TC3-HMAC-SHA256"
	service := "cls"
	timestamp := time.Now().Unix()
	date := time.Unix(timestamp, 0).UTC().Format("2006-01-02")

	// Step 1: Canonical request
	canonicalHeaders := fmt.Sprintf("content-type:application/json\nhost:%s\n", host)
	signedHeaders := "content-type;host"
	hashedPayload := sha256Hex(payload)
	canonicalRequest := fmt.Sprintf("POST\n/\n\n%s\n%s\n%s",
		canonicalHeaders, signedHeaders, hashedPayload)

	// Step 2: String to sign
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)
	hashedCanonicalRequest := sha256Hex(canonicalRequest)
	stringToSign := fmt.Sprintf("%s\n%d\n%s\n%s",
		algorithm, timestamp, credentialScope, hashedCanonicalRequest)

	// Step 3: Signature
	secretDate := hmacSHA256([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256(secretDate, service)
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))

	// Step 4: Authorization header
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, secretID, credentialScope, signedHeaders, signature)

	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Host", host)
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Version", "2020-10-16")
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("X-TC-Region", region)
}

func sha256Hex(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func datasourceHTTPAuth(authType, authConfig string) httpclient.Auth {
	var auth map[string]string
	_ = json.Unmarshal([]byte(authConfig), &auth)
	return httpclient.Auth{Type: authType, Username: auth["username"], Password: auth["password"], Token: auth["token"]}
}
func setAuthHeader(req *http.Request, authType, authConfig string) {
	datasourceHTTPAuth(authType, authConfig).Apply(req)
}
