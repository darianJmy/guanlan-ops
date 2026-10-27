package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"guanlan/pkg/resp"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

// Only reviewed handlers in the generated allowlist can run. Callers cannot
// supply a URL, HTTP method, handler name, SQL statement or authentication context.
type platformOperation struct {
	Name, Title, Permission, Method, Path, SchemaJSON string
	AdminOnly                                         bool
	Handler                                           gin.HandlerFunc
}

const operationDirectoryPermission = "operation:scoped"

var platformOperations []platformOperation

func init() {
	platformOperations = generatedPlatformOperations()
	mcpTools = append(mcpTools, operationMCPTools()...)
}

func findPlatformOperation(name string) (platformOperation, bool) {
	for _, op := range platformOperations {
		if op.Name == name {
			return op, true
		}
	}
	return platformOperation{}, false
}
func operationSchema(op platformOperation) map[string]interface{} {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(op.SchemaJSON), &result); err != nil {
		panic(err)
	}
	return result
}
func authorizeOperation(ctx context.Context, userID uint, op platformOperation) error {
	if err := authorizeAIPermission(ctx, userID, op.Permission); err != nil {
		return errors.New("没有执行该操作的权限")
	}
	if op.AdminOnly {
		var count int64
		err := database.DB.WithContext(ctx).Table("roles AS r").Joins("JOIN user_roles AS ur ON ur.role_id = r.id").Where("ur.user_id = ? AND r.code = ? AND r.status = 1", userID, "admin").Count(&count).Error
		if err != nil || count == 0 {
			return errors.New("该操作仅向管理员开放")
		}
	}
	return nil
}
func authorizeOperationDirectory(ctx context.Context, userID uint) error {
	var count int64
	if err := database.DB.WithContext(ctx).Model(&models.User{}).Where("id = ? AND status = 1", userID).Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return errors.New("用户不存在或已停用")
	}
	return nil
}

func operationMCPTools() []mcpTool {
	tools := []mcpTool{
		{Name: "search_platform_operations", Permission: operationDirectoryPermission, Title: "查找平台操作", Description: "按关键词查找当前用户可用的平台页面操作。支持告警规则、拨测、数据源、通知、自动化、脚本、工作流、节点、仪表盘、用户角色和系统设置。关键词可用中文模块名或英文操作名；空关键词分页浏览。获得操作名后用 describe_platform_operation 查看完整参数。", InputSchema: mcpObjectSchema(map[string]interface{}{"keyword": mcpText("模块中文名或操作英文名，如 告警、拨测、create_alert_rule", 128), "page": mcpPageSchema(1)}), Annotations: mcpReadOnlyAnnotations()},
		{Name: "describe_platform_operation", Permission: operationDirectoryPermission, Title: "查看操作参数", Description: "获取指定平台操作的 JSON Schema、权限和注意事项；执行前必须先查看，参数分为 path、query、body。", InputSchema: requiredOperationSchema(map[string]interface{}{"operation": mcpText("查找工具返回的精确操作名", 100)}, "operation"), Annotations: mcpReadOnlyAnnotations()},
		{Name: "execute_platform_operation", Permission: operationDirectoryPermission, Title: "执行平台操作", Description: "执行目录中的固定平台操作（包括创建、修改、删除、启停、运行、导入和查询）。仅执行用户明确要求的操作，先查阅 describe_platform_operation；arguments_json 是符合该操作 schema 的 JSON 对象字符串。修改为完整表单替换，先读取现值并保留未要求修改的字段。失败或超时不可盲目重试写操作。", InputSchema: requiredOperationSchema(map[string]interface{}{"operation": mcpText("精确操作名", 100), "arguments_json": mcpText("JSON 对象字符串，键为 path/query/body，按操作 schema 填写；无参数传 {}", 8192)}, "operation", "arguments_json"), Annotations: map[string]interface{}{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": false, "openWorldHint": true}, AssistantWritable: true},
	}
	for _, name := range []string{"create_alert_rule", "update_alert_rule"} {
		op, _ := findPlatformOperation(name)
		schema := operationSchema(op)["properties"].(map[string]interface{})["body"].(map[string]interface{})
		props := schema["properties"].(map[string]interface{})
		// Dynamic label dictionaries are available through the full operation schema.
		// Eino ParameterInfo does not represent additionalProperties dictionaries.
		delete(props, "external_labels")
		delete(props, "annotations")
		props["name"] = mcpText("规则名称，组内唯一", 128)
		props["expression"] = mcpText("返回数值的 PromQL；比较符和阈值放在 conditions，避免重复比较", 8192)
		props["status"] = map[string]interface{}{"type": "integer", "minimum": 0, "maximum": 1, "description": "必须明确指定：1 启用并参与评估，0 停用"}
		props["conditions"] = map[string]interface{}{"type": "array", "minItems": 1, "maxItems": 3, "items": requiredOperationSchema(map[string]interface{}{"severity": mcpStringEnum("P0", "P1", "P2"), "operator": mcpStringEnum(">", ">=", "<", "<=", "==", "!="), "threshold": map[string]interface{}{"type": "number"}}, "severity", "operator", "threshold")}
		schema["required"] = []string{"name", "expression", "group_id", "fault_center_id", "conditions", "status", "evaluation_interval_seconds", "for_seconds", "recovery_wait_seconds"}
		description := "创建具体 Prometheus/VictoriaMetrics 告警规则，包含 PromQL、阈值、等级及启用状态。先查询真实规则组、故障中心和数据源 ID；datasource_ids 与 datasource_group_ids 至少提供一个非空列表。评估间隔及持续时间单位为秒。"
		if name == "update_alert_rule" {
			props["id"] = mcpID("待修改规则 ID")
			schema["required"] = append(schema["required"].([]string), "id")
			description = "修改具体告警规则。先通过目录的 get_alert_rule 读取完整现值，再提交完整配置；未传入的标签和注解由工具保留。数据源列表必须明确提供。"
		}
		tools = append(tools, mcpTool{Name: name, Title: op.Title, Permission: op.Permission, Description: description, InputSchema: schema, AssistantWritable: true, Annotations: map[string]interface{}{"readOnlyHint": false, "destructiveHint": name == "update_alert_rule", "idempotentHint": false, "openWorldHint": false}})
	}
	return tools
}
func requiredOperationSchema(props map[string]interface{}, required ...string) map[string]interface{} {
	s := mcpObjectSchema(props)
	s["required"] = required
	return s
}
func isOperationMCPTool(name string) bool {
	switch name {
	case "search_platform_operations", "describe_platform_operation", "execute_platform_operation", "create_alert_rule", "update_alert_rule":
		return true
	}
	return false
}

func executeOperationMCPTool(ctx context.Context, userID uint, name string, args map[string]interface{}) (interface{}, error) {
	if err := authorizeOperationDirectory(ctx, userID); err != nil {
		return nil, err
	}
	tool, ok := findMCPTool(name)
	if !ok || !isOperationMCPTool(name) {
		return nil, errors.New("工具不存在")
	}
	normalized, err := validatePlatformMCPArgs(tool, args)
	if err != nil {
		return nil, err
	}
	switch name {
	case "search_platform_operations":
		keyword := strings.ToLower(strings.TrimSpace(mcpStringArg(normalized, "keyword")))
		page, _ := mcpPagination(normalized)
		matches := []map[string]interface{}{}
		for _, op := range platformOperations {
			if !strings.Contains(strings.ToLower(op.Name+" "+op.Title+" "+op.Path), keyword) || authorizeOperation(ctx, userID, op) != nil {
				continue
			}
			matches = append(matches, map[string]interface{}{"operation": op.Name, "title": op.Title, "permission": op.Permission, "read_only": op.Method == http.MethodGet})
		}
		start := (page - 1) * 20
		if start > len(matches) {
			start = len(matches)
		}
		end := start + 20
		if end > len(matches) {
			end = len(matches)
		}
		return map[string]interface{}{"list": matches[start:end], "total": len(matches), "page": page, "page_size": 20}, nil
	case "describe_platform_operation":
		op, ok := findPlatformOperation(mcpStringArg(normalized, "operation"))
		if !ok {
			return nil, errors.New("操作不存在")
		}
		if err := authorizeOperation(ctx, userID, op); err != nil {
			return nil, err
		}
		return map[string]interface{}{"operation": op.Name, "title": op.Title, "permission": op.Permission, "read_only": op.Method == http.MethodGet, "input_schema": operationSchema(op), "instructions": "仅执行用户明确要求的操作。path 是资源 ID，query 是筛选条件，body 是表单。PUT 为完整表单替换，先读取现有配置；省略字段可能清空原值。status 通常为 1 启用、0 停用。密钥字段和执行输出不会回传模型；不要求用户在聊天中提供密钥。执行超时后先查记录，不可自动重试。"}, nil
	case "execute_platform_operation":
		var arguments map[string]interface{}
		if json.Unmarshal([]byte(mcpStringArg(normalized, "arguments_json")), &arguments) != nil || arguments == nil {
			return nil, errors.New("arguments_json 必须是 JSON 对象")
		}
		return executePlatformOperation(ctx, userID, mcpStringArg(normalized, "operation"), arguments)
	default:
		body := map[string]interface{}{}
		for k, v := range normalized {
			if k != "id" {
				body[k] = v
			}
		}
		arguments := map[string]interface{}{"body": body}
		if name == "update_alert_rule" {
			op, _ := findPlatformOperation(name)
			if err := authorizeOperation(ctx, userID, op); err != nil {
				return nil, err
			}
			var existing models.AlertRule
			if database.DB.WithContext(ctx).First(&existing, normalized["id"]).Error != nil {
				return nil, errors.New("告警规则不存在")
			}
			body["external_labels"] = decodeAlertRuleExternalLabels(existing.Labels)
			body["annotations"] = decodeAlertRuleAnnotations(existing.Annotations)
			arguments["path"] = map[string]interface{}{"id": normalized["id"]}
		}
		// Normalize maps produced by model helpers to JSON value types.
		raw, _ := json.Marshal(arguments)
		_ = json.Unmarshal(raw, &arguments)
		return executePlatformOperation(ctx, userID, name, arguments)
	}
}

// Validate the full operation schema including dynamic JSON maps. The assistant
// transport carries it as JSON text because Eino's simple schema omits map types.
func validateOperationValue(def map[string]interface{}, value interface{}, depth int) error {
	if depth > 16 {
		return errors.New("参数嵌套过深")
	}
	kind, _ := def["type"].(string)
	switch kind {
	case "object":
		values, ok := value.(map[string]interface{})
		if !ok {
			return errors.New("必须是 JSON 对象")
		}
		props, _ := def["properties"].(map[string]interface{})
		if required, ok := def["required"].([]interface{}); ok {
			for _, raw := range required {
				key := raw.(string)
				if _, exists := values[key]; !exists {
					return fmt.Errorf("缺少参数 %s", key)
				}
			}
		}
		for key, v := range values {
			child, ok := props[key].(map[string]interface{})
			if !ok {
				if extra, ok := def["additionalProperties"].(map[string]interface{}); ok {
					child = extra
				} else if def["additionalProperties"] == true {
					child = map[string]interface{}{}
				} else {
					return fmt.Errorf("不支持的参数 %s", key)
				}
			}
			if err := validateOperationValue(child, v, depth+1); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
		}
	case "array":
		values, ok := value.([]interface{})
		if !ok || len(values) > 1000 {
			return errors.New("必须是不超过1000项的数组")
		}
		child, _ := def["items"].(map[string]interface{})
		for _, v := range values {
			if err := validateOperationValue(child, v, depth+1); err != nil {
				return err
			}
		}
	case "string":
		if s, ok := value.(string); !ok || len(s) > 65536 || strings.Contains(s, "[已隐藏]") {
			return errors.New("必须是有效字符串")
		}
	case "integer", "number":
		n, ok := value.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > 9007199254740991 || (kind == "integer" && math.Trunc(n) != n) {
			return errors.New("数字格式不正确")
		}
		if min, ok := def["minimum"].(float64); ok && n < min {
			return errors.New("数值小于下限")
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return errors.New("必须是布尔值")
		}
	case "": // map[string]interface{} intentionally accepts arbitrary JSON values.
	default:
		return errors.New("不支持的参数类型")
	}
	return nil
}

type operationResponse struct {
	header    http.Header
	status    int
	body      bytes.Buffer
	truncated bool
}

func (w *operationResponse) Header() http.Header { return w.header }
func (w *operationResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *operationResponse) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	remaining := (512 << 10) - w.body.Len()
	if len(b) > remaining {
		w.truncated = true
	} else {
		remaining = len(b)
	}
	if remaining > 0 {
		_, _ = w.body.Write(b[:remaining])
	}
	return len(b), nil
}

func executePlatformOperation(ctx context.Context, userID uint, name string, args map[string]interface{}) (result interface{}, err error) {
	op, ok := findPlatformOperation(name)
	if !ok {
		return nil, errors.New("操作不存在")
	}
	if err := authorizeOperation(ctx, userID, op); err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err := validateOperationValue(operationSchema(op), args, 0); err != nil {
		return nil, err
	}
	path := op.Path
	pathValues, _ := args["path"].(map[string]interface{})
	for key, value := range pathValues {
		pathValue := operationParameterString(value)
		if !validOperationPathValue(pathValue) {
			return nil, errors.New("资源 ID 格式不正确")
		}
		path = strings.ReplaceAll(path, ":"+key, pathValue)
	}
	query := url.Values{}
	if values, ok := args["query"].(map[string]interface{}); ok {
		for key, value := range values {
			query.Set(key, operationParameterString(value))
		}
	}
	var body []byte
	if value, exists := args["body"]; exists {
		body, _ = json.Marshal(value)
	}
	req, requestErr := http.NewRequestWithContext(ctx, op.Method, "http://platform.local/api"+path+"?"+query.Encode(), bytes.NewReader(body))
	if requestErr != nil {
		return nil, errors.New("操作请求参数格式不正确")
	}
	req.Header.Set("Content-Type", "application/json")
	writer := &operationResponse{header: make(http.Header)}
	c, _ := gin.CreateTestContext(writer)
	c.Request = req
	c.Set("user_id", userID)
	c.Set("auth_type", "assistant")
	var user models.User
	if database.DB.WithContext(ctx).Select("id", "username").First(&user, userID).Error != nil {
		return nil, errors.New("用户不存在")
	}
	c.Set("username", user.Username)
	for key, value := range pathValues {
		c.Params = append(c.Params, gin.Param{Key: key, Value: operationParameterString(value)})
	}
	started := time.Now()
	// Write intent first: an audit failure must not allow an unrecorded mutation.
	audit := models.AuditLog{UserID: userID, Username: user.Username, Module: "智能助手", Action: op.Name, ResourceType: "platform_operation", Method: op.Method, Path: "/api" + path, Result: "执行中", RequestSummary: `{"operation":"` + op.Name + `"}`}
	if err := database.DB.WithContext(ctx).Create(&audit).Error; err != nil {
		return nil, errors.New("无法记录操作审计，操作未执行")
	}
	defer func() {
		if recover() != nil {
			result = nil
			err = errors.New("平台操作异常；写操作请先查询当前状态，不要直接重试")
		}
		status := "成功"
		if err != nil {
			status = "失败"
		}
		if ctx.Err() != nil {
			status = "结果待核实"
		}
		// No request bodies, handler messages or secrets are persisted here.
		auditErr := database.DB.Model(&models.AuditLog{}).Where("id = ?", audit.ID).Updates(map[string]interface{}{"result": status, "status_code": writer.status, "duration_ms": time.Since(started).Milliseconds()}).Error
		if auditErr != nil && err == nil {
			result = nil
			err = errors.New("操作已执行，但审计结果更新失败；请查询实际状态，不要重复提交")
		}
	}()
	op.Handler(c)
	if ctx.Err() != nil {
		return nil, errors.New("操作超时或取消，执行结果待核实；请查询状态，不要重复提交")
	}
	if writer.truncated {
		return nil, errors.New("响应过大，无法确认操作结果；请缩小查询范围，写操作勿重复提交")
	}
	if writer.status < 400 && strings.Contains(writer.Header().Get("Content-Type"), "application/yaml") {
		var document map[string]interface{}
		if yaml.Unmarshal(writer.body.Bytes(), &document) != nil {
			return nil, errors.New("导出文档格式不正确")
		}
		return map[string]interface{}{"code": resp.CodeSuccess, "format": "yaml", "data": sanitizeOperationResult(document)}, nil
	}
	var envelope map[string]interface{}
	if json.Unmarshal(writer.body.Bytes(), &envelope) != nil {
		return nil, errors.New("操作未返回可解析的 JSON 结果，请查询状态")
	}
	code, hasCode := envelope["code"].(float64)
	if writer.status >= 400 || !hasCode || code != resp.CodeSuccess {
		message, _ := envelope["message"].(string)
		// Validation messages are useful, but upstream/server errors may contain secrets.
		if writer.status >= 500 || len(message) > 300 {
			message = "平台执行失败，请查看平台日志"
		}
		if message == "" {
			message = "平台操作失败"
		}
		return nil, errors.New(message)
	}
	return sanitizeOperationResult(envelope), nil
}

func sanitizeOperationResult(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		result := map[string]interface{}{}
		for key, child := range v {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
			sensitive := false
			for _, part := range []string{"password", "passwd", "secret", "token", "privatekey", "authconfig", "accesskey", "apikey", "credential", "webhook", "installcommand", "installscript"} {
				if strings.Contains(normalized, part) {
					sensitive = true
				}
			}
			switch normalized {
			case "config", "ldapconfig", "oidcconfig", "headers", "output", "stdout", "stderr", "requestbody", "response", "input", "error", "lasterror", "result":
				sensitive = true
			}
			if strings.HasSuffix(normalized, "id") || strings.HasSuffix(normalized, "ids") || strings.HasPrefix(normalized, "has") {
				sensitive = false
			}
			if sensitive {
				result[key] = "[已隐藏]"
			} else {
				result[key] = sanitizeOperationResult(child)
			}
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, child := range v {
			result[i] = sanitizeOperationResult(child)
		}
		return result
	case string:
		// Credentials sometimes arrive embedded in serialized configuration or URLs.
		var decoded interface{}
		if json.Unmarshal([]byte(v), &decoded) == nil {
			switch decoded.(type) {
			case map[string]interface{}, []interface{}:
				raw, _ := json.Marshal(sanitizeOperationResult(decoded))
				return string(raw)
			}
		}
		if u, err := url.Parse(v); err == nil && u.IsAbs() && u.Host != "" {
			u.User = nil
			u.RawQuery = ""
			u.Fragment = ""
			return u.String()
		}
		return v
	default:
		return value
	}
}

func operationParameterString(value interface{}) string {
	if number, ok := value.(float64); ok {
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
	return fmt.Sprint(value)
}
func validOperationPathValue(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
