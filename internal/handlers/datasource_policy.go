package handlers

import (
	"encoding/json"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

func validateDatasourceConnection(ds models.Datasource) error {
	if !isSupportedDatasourceType(ds.Type) {
		return fmt.Errorf("暂不支持该数据源类型")
	}
	endpoint, err := url.Parse(strings.TrimSpace(ds.URL))
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.Fragment != "" || endpoint.RawQuery != "" {
		return fmt.Errorf("请填写有效的 数据源 HTTP/HTTPS 地址，认证信息请单独配置")
	}
	if strings.ContainsAny(ds.URL, "\r\n\t ") {
		return fmt.Errorf("数据源地址不能包含空白字符")
	}
	authType := ds.AuthType
	if authType == "" {
		authType = "none"
	}
	if authType != "none" && authType != "basic" && authType != "bearer" {
		return fmt.Errorf("当前数据源仅支持无认证、Basic Auth 或 Bearer Token")
	}
	var auth DatasourceAuthConfig
	if ds.AuthConfig != "" && json.Unmarshal([]byte(ds.AuthConfig), &auth) != nil {
		return fmt.Errorf("认证配置格式不正确")
	}
	if authType == "basic" && (strings.TrimSpace(auth.Username) == "" || auth.Password == "") {
		return fmt.Errorf("请填写 Basic Auth 用户名和密码")
	}
	if authType == "bearer" && strings.TrimSpace(auth.Token) == "" {
		return fmt.Errorf("请填写 Bearer Token")
	}
	var config struct {
		CustomHeaders map[string]string `json:"custom_headers"`
	}
	if ds.Config != "" && json.Unmarshal([]byte(ds.Config), &config) != nil {
		return fmt.Errorf("HTTP 请求头配置格式不正确")
	}
	if len(config.CustomHeaders) > 20 {
		return fmt.Errorf("自定义 HTTP 头最多 20 个")
	}
	for name, value := range config.CustomHeaders {
		if strings.TrimSpace(name) == "" || len(name) > 128 || len(value) > 1000 || strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("自定义 HTTP 头格式不正确")
		}
		for _, r := range name {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
				return fmt.Errorf("HTTP 头名称格式不正确")
			}
		}
	}
	if ds.Type == "clickhouse" {
		var cfg struct {
			Logs *logSourceConfig `json:"log_query"`
		}
		if json.Unmarshal([]byte(ds.Config), &cfg) == nil && cfg.Logs == nil {
			return nil
		}
		if ds.Config == "" {
			return nil
		}
	}
	_, err = readLogSourceConfig(ds)
	return err
}

func normalizeDatasourceReq(req *CreateDatasourceWithTunnelReq, existing *models.Datasource) error {
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	if existing != nil && existing.Type != req.Type {
		return fmt.Errorf("已有实例不能变更数据源类型，请新增实例")
	}
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimRight(strings.TrimSpace(req.URL), "/")
	req.AuthType = strings.ToLower(strings.TrimSpace(req.AuthType))
	if req.AuthType == "" {
		req.AuthType = "none"
	}
	if req.Name == "" || len([]rune(req.Name)) > 100 {
		return fmt.Errorf("实例名称不能为空且不能超过 100 个字符")
	}
	if req.Status != 0 && req.Status != 1 {
		return fmt.Errorf("实例状态不正确")
	}
	if req.AuthType == "none" {
		req.AuthConfig = ""
	} else if existing != nil && strings.TrimSpace(req.AuthConfig) == "" && existing.AuthType == req.AuthType {
		req.AuthConfig = existing.AuthConfig
	}
	if err := validateDatasourceConnection(models.Datasource{Type: req.Type, URL: req.URL, AuthType: req.AuthType, AuthConfig: req.AuthConfig, Config: req.Config}); err != nil {
		return err
	}
	definition, _ := datasourceDefinitionFor(req.Type)
	req.Category = definition.Category
	req.Capabilities = strings.Join(definition.Capabilities, ",")
	return nil
}

// Archived connections remain in storage but cannot be selected or mutated through monitoring APIs.
func requireSupportedDatasourceIDs(c *gin.Context, ids []uint) bool {
	ids = normalizeChannelIDs(ids)
	if len(ids) == 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "请选择数据源实例")
		return false
	}
	var count int64
	if err := database.DB.Model(&models.Datasource{}).Where("id IN ? AND type IN ?", ids, supportedDatasourceTypes()).Count(&count).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "检查监控实例失败")
		return false
	}
	if count != int64(len(ids)) {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "实例不存在或类型暂不受支持")
		return false
	}
	return true
}

func requireUnboundDatasources(c *gin.Context, ids []uint) bool {
	var count int64
	if err := database.DB.Model(&models.AlertRuleDatasource{}).Where("datasource_id IN ?", ids).Count(&count).Error; err != nil {
		resp.Error(c, http.StatusInternalServerError, resp.CodeInternalServerError, "检查告警规则引用失败")
		return false
	}
	if count > 0 {
		resp.Error(c, http.StatusBadRequest, resp.CodeBadRequest, "实例已被告警规则引用，请先解除绑定")
		return false
	}
	return true
}
