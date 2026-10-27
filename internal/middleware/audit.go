package middleware

import (
	"bytes"
	"encoding/json"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"guanlan/pkg/textutil"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxAuditSummaryBytes = 4096

type auditResponseWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *auditResponseWriter) Write(data []byte) (int, error) {
	if w.body.Len() < maxAuditSummaryBytes {
		remaining := maxAuditSummaryBytes - w.body.Len()
		if len(data) < remaining {
			remaining = len(data)
		}
		_, _ = w.body.Write(data[:remaining])
	}
	return w.ResponseWriter.Write(data)
}

func (w *auditResponseWriter) WriteString(value string) (int, error) {
	data := []byte(value)
	if w.body.Len() < maxAuditSummaryBytes {
		remaining := maxAuditSummaryBytes - w.body.Len()
		if len(data) < remaining {
			remaining = len(data)
		}
		_, _ = w.body.Write(data[:remaining])
	}
	return w.ResponseWriter.WriteString(value)
}

// Audit records meaningful authenticated operations. Ordinary list, paging and
// refresh requests are excluded to keep the evidence useful at scale.
func Audit() gin.HandlerFunc {
	return func(c *gin.Context) {
		fullPath := c.FullPath()
		if fullPath == "" {
			fullPath = c.Request.URL.Path
		}
		action, shouldRecord := auditAction(c.Request.Method, fullPath)
		if !shouldRecord || strings.HasPrefix(fullPath, "/api/audit-logs") {
			c.Next()
			return
		}

		requestBody := readAuditRequestBody(c)
		startedAt := time.Now()
		writer := &auditResponseWriter{ResponseWriter: c.Writer}
		c.Writer = writer
		c.Next()

		module, resourceType := auditResource(fullPath)
		result, message := auditResponseResult(writer.Status(), writer.body.Bytes())
		resourceID := auditResourceID(c, fullPath)
		workspaceID := auditWorkspaceID(c, requestBody)
		entry := models.AuditLog{
			UserID: c.GetUint("user_id"), Username: c.GetString("username"), WorkspaceID: workspaceID,
			Module: module, Action: action, ResourceType: resourceType, ResourceID: resourceID,
			ResourceName: auditResourceName(requestBody), Method: c.Request.Method, Path: c.Request.URL.Path,
			RequestSummary: auditRequestSummary(c, requestBody), Result: result, StatusCode: writer.Status(),
			ResponseMessage: message, DurationMS: time.Since(startedAt).Milliseconds(), IP: c.ClientIP(),
			UserAgent: textutil.Clip(c.Request.UserAgent(), 500),
		}
		writeAuditLog(entry)
	}
}

// RecordLogin records both successful and rejected login attempts without ever
// persisting the submitted password.
func RecordLogin(c *gin.Context, userID uint, username, result, message string) {
	writeAuditLog(models.AuditLog{
		UserID: userID, Username: textutil.Clip(strings.TrimSpace(username), 50), Module: "认证安全",
		Action: "登录", ResourceType: "session", ResourceName: textutil.Clip(strings.TrimSpace(username), 255),
		Method: c.Request.Method, Path: c.Request.URL.Path, RequestSummary: `{"username":"` + escapeAuditText(username) + `"}`,
		Result: result, StatusCode: http.StatusOK, ResponseMessage: textutil.Clip(message, 500), IP: c.ClientIP(),
		UserAgent: textutil.Clip(c.Request.UserAgent(), 500),
	})
}

func writeAuditLog(entry models.AuditLog) {
	if database.DB != nil {
		_ = database.DB.Create(&entry).Error
	}
}

func auditAction(method, path string) (string, bool) {
	for suffix, action := range map[string]string{
		"/reset-password": "重置密码", "/rotate-token": "轮换令牌", "/status": "启停",
		"/permissions": "分配权限", "/menus": "分配菜单", "/roles": "分配角色",
	} {
		if strings.HasSuffix(path, suffix) {
			return action, true
		}
	}
	if strings.Contains(path, "/:id/datasources") {
		return "分配数据源", true
	}
	if strings.HasSuffix(path, "/silence") {
		if method == http.MethodDelete {
			return "取消静默", true
		}
		return "静默", true
	}
	if method == http.MethodGet {
		if strings.HasSuffix(path, "/download") {
			return "下载", true
		}
		if strings.Contains(path, "/:id") || strings.Contains(path, "/:ruleId") {
			return "查看", true
		}
		return "", false
	}
	if method == http.MethodDelete {
		return "删除", true
	}
	if method == http.MethodPut || method == http.MethodPatch {
		return "编辑", true
	}
	if method != http.MethodPost {
		return "", false
	}
	for suffix, action := range map[string]string{
		"/run": "执行", "/run-all": "执行", "/test": "测试", "/check": "检查", "/check-all": "检查",
		"/deploy": "发布",
	} {
		if strings.HasSuffix(path, suffix) {
			return action, true
		}
	}
	if strings.Contains(path, "/query") || strings.Contains(path, "/resource") || strings.HasSuffix(path, "/chat") || strings.HasSuffix(path, "/instant-probe") || strings.HasSuffix(path, "/mcp") {
		return "调用", true
	}
	return "创建", true
}

func auditResource(path string) (string, string) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	key := ""
	if len(segments) > 1 && segments[0] == "api" {
		key = segments[1]
	} else if len(segments) > 0 {
		key = segments[0]
	}
	items := map[string][2]string{
		"users": {"用户管理", "user"}, "roles": {"角色管理", "role"}, "menus": {"菜单管理", "menu"},
		"datasources": {"接入中心", "datasource"}, "datasource-groups": {"数据源分组", "workspace"}, "prometheus": {"指标查询", "prometheus_query"}, "query": {"查询中心", "log_query"},
		"dashboards": {"仪表盘", "dashboard"}, "cloud-monitor-workspaces": {"云监控2.0", "cloud_monitor_workspace"}, "inspection-items": {"拨测中心", "inspection_task"},
		"instant-probe": {"拨测中心", "instant_probe"}, "scripts": {"自动化脚本库", "script"},
		"workflows":       {"工作流", "workflow"},
		"mcp-service":     {"MCP 服务", "mcp_service"},
		"mcp-clients":     {"MCP 服务", "mcp_client"},
		"mcp":             {"MCP 服务", "mcp_tool"},
		"ai-models":       {"AI 模型", "ai_model"},
		"git-credentials": {"Git 认证", "git_credential"},
		"api-tokens":      {"访问令牌", "api_token"},
		"system-settings": {"基础设置", "system_setting"}, "auth-settings": {"登录认证", "auth_setting"},
		"notification-channels": {"通知对象", "notification_channel"}, "notification-templates": {"通知模板", "notification_template"},
		"alerts": {"故障中心", "alert"}, "fault-centers": {"故障中心", "fault_center"},
		"inspection-rule-groups": {"拨测中心", "inspection_rule_group"},
		"alert-rule-groups":      {"告警管理", "alert_rule_group"}, "alert-rules": {"告警管理", "alert_rule"},
		"certificate-executors": {"节点管理", "certificate_executor"}, "agents": {"节点管理", "agent"},
		"chat": {"智能运维", "chat"}, "conversations": {"智能运维", "conversation"},
	}
	if item, ok := items[key]; ok {
		return item[0], item[1]
	}
	return "平台操作", strings.ReplaceAll(key, "-", "_")
}

func readAuditRequestBody(c *gin.Context) map[string]interface{} {
	if c.Request.Body == nil {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		return nil
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	var value map[string]interface{}
	if len(body) > 0 && json.Unmarshal(body, &value) == nil {
		return sanitizeAuditMap(value)
	}
	return nil
}

func sanitizeAuditMap(value map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(value))
	for key, item := range value {
		if auditSensitiveKey(key) {
			result[key] = "***"
			continue
		}
		switch typed := item.(type) {
		case map[string]interface{}:
			result[key] = sanitizeAuditMap(typed)
		case []interface{}:
			items := make([]interface{}, 0, len(typed))
			for _, child := range typed {
				if object, ok := child.(map[string]interface{}); ok {
					items = append(items, sanitizeAuditMap(object))
				} else {
					items = append(items, child)
				}
			}
			result[key] = items
		default:
			result[key] = item
		}
	}
	return result
}

func auditSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
	for _, part := range []string{"password", "passwd", "token", "secret", "privatekey", "authconfig", "credential", "accesskey", "apikey"} {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}

func auditRequestSummary(c *gin.Context, body map[string]interface{}) string {
	value := map[string]interface{}{}
	if len(c.Request.URL.Query()) > 0 {
		query := map[string]interface{}{}
		for key, items := range c.Request.URL.Query() {
			if auditSensitiveKey(key) {
				query[key] = "***"
			} else {
				query[key] = items
			}
		}
		value["query"] = query
	}
	if len(body) > 0 {
		value["body"] = body
	}
	if len(value) == 0 {
		return ""
	}
	encoded, _ := json.Marshal(value)
	return textutil.Clip(string(encoded), maxAuditSummaryBytes)
}

func auditResponseResult(status int, body []byte) (string, string) {
	result := "成功"
	message := ""
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &payload) == nil {
		message = payload.Message
		if payload.Code != resp.CodeSuccess {
			result = "失败"
		}
	}
	if status >= http.StatusBadRequest {
		result = "失败"
	}
	return result, textutil.Clip(message, 500)
}

func auditResourceID(c *gin.Context, path string) string {
	for _, name := range []string{"ruleId", "id"} {
		if value := strings.TrimSpace(c.Param(name)); value != "" {
			return textutil.Clip(value, 128)
		}
	}
	return ""
}

func auditResourceName(body map[string]interface{}) string {
	for _, key := range []string{"name", "rule_name", "title", "username", "domain"} {
		if value, ok := body[key].(string); ok && strings.TrimSpace(value) != "" {
			return textutil.Clip(strings.TrimSpace(value), 255)
		}
	}
	return ""
}

func auditWorkspaceID(c *gin.Context, body map[string]interface{}) uint {
	for _, value := range []string{c.GetHeader("X-Workspace-ID"), c.Query("workspace_id")} {
		if parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64); err == nil {
			return uint(parsed)
		}
	}
	if value, ok := body["workspace_id"].(float64); ok && value > 0 {
		return uint(value)
	}
	return 0
}

func escapeAuditText(value string) string {
	encoded, _ := json.Marshal(strings.TrimSpace(value))
	return strings.Trim(string(encoded), `"`)
}
