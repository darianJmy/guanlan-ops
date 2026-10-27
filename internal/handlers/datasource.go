package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/httpclient"
	"guanlan/pkg/resp"
)

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
	if _, ok := deleteDatasourceInstances(c, []uint{uint(numericID)}); !ok {
		return
	}
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

func categoryByType(dsType string) string {
	if definition, ok := datasourceDefinitionFor(dsType); ok {
		return definition.Category
	}
	return "other"
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
	if definition, ok := datasourceDefinitionFor(dsType); ok {
		return definition.Capabilities
	}
	return nil
}

func priorityOrDefault(priority int) int {
	if priority <= 0 {
		return 100
	}
	return priority
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

func datasourceHTTPAuth(authType, authConfig string) httpclient.Auth {
	var auth map[string]string
	_ = json.Unmarshal([]byte(authConfig), &auth)
	return httpclient.Auth{Type: authType, Username: auth["username"], Password: auth["password"], Token: auth["token"]}
}
