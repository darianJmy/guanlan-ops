package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"guanlan/pkg/resp"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	mcpServerName    = "aiops-operations-center"
	mcpServerVersion = "1.0.0"
	mcpMaxBodyBytes  = 1 << 20
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpTool struct {
	Name              string                 `json:"name"`
	Title             string                 `json:"title,omitempty"`
	Description       string                 `json:"description"`
	InputSchema       map[string]interface{} `json:"inputSchema"`
	Annotations       map[string]interface{} `json:"annotations,omitempty"`
	Permission        string                 `json:"-"`
	AssistantWritable bool                   `json:"-"`
}

var mcpTools = append(append(datasourceMCPTools(), platformMCPTools()...), []mcpTool{
	{
		Name: "query_alerts", Title: "查询告警", Permission: "alert:event:list",
		Description: "查询故障中心的活跃或历史告警，可按状态、等级、来源和关键字筛选。",
		InputSchema: mcpObjectSchema(map[string]interface{}{
			"status":          mcpStringEnum("active", "history", "pending", "firing", "recovering", "resolved", "terminated"),
			"severity":        map[string]interface{}{"type": "string", "description": "告警等级，例如 P0、P1、P2"},
			"source":          map[string]interface{}{"type": "string", "description": "告警来源"},
			"keyword":         map[string]interface{}{"type": "string", "description": "匹配告警名称、目标或标签"},
			"fault_center_id": map[string]interface{}{"type": "integer", "minimum": 1},
			"page":            mcpPageSchema(1), "page_size": mcpPageSizeSchema(),
		}),
		Annotations: mcpReadOnlyAnnotations(),
	},
	{
		Name: "query_alert_rules", Title: "查询告警规则", Permission: "alert:rule:list",
		Description: "查询 Prometheus 告警规则及其执行状态。",
		InputSchema: mcpObjectSchema(map[string]interface{}{
			"keyword":  map[string]interface{}{"type": "string", "description": "匹配规则名称或 PromQL"},
			"status":   mcpStringEnum("enabled", "disabled"),
			"group_id": map[string]interface{}{"type": "integer", "minimum": 1},
			"page":     mcpPageSchema(1), "page_size": mcpPageSizeSchema(),
		}),
		Annotations: mcpReadOnlyAnnotations(),
	},
	{
		Name: "query_probe_tasks", Title: "查询拨测任务", Permission: "inspection:list",
		Description: "查询拨测任务、目标、最近状态和告警状态。",
		InputSchema: mcpObjectSchema(map[string]interface{}{
			"keyword": map[string]interface{}{"type": "string", "description": "匹配任务名称、目标或标签"},
			"type":    mcpStringEnum("http", "tcp", "icmp", "certificate_expiry"),
			"status":  mcpStringEnum("enabled", "disabled"),
			"page":    mcpPageSchema(1), "page_size": mcpPageSizeSchema(),
		}),
		Annotations: mcpReadOnlyAnnotations(),
	},
	{
		Name: "query_automation_tasks", Title: "查询自动化任务", Permission: "automation-task:list",
		Description: "查询自动化任务及其执行节点、运行方式和启用状态。",
		InputSchema: mcpObjectSchema(map[string]interface{}{
			"keyword": map[string]interface{}{"type": "string", "description": "匹配任务名称或描述"},
			"status":  mcpStringEnum("enabled", "disabled"),
			"page":    mcpPageSchema(1), "page_size": mcpPageSizeSchema(),
		}),
		Annotations: mcpReadOnlyAnnotations(),
	},
	{
		Name: "run_automation_task", Title: "执行自动化任务", Permission: "automation-task:run",
		Description: "执行一条已启用的自动化任务。调用前应向用户确认任务和输入参数。",
		InputSchema: map[string]interface{}{
			"type": "object", "additionalProperties": false,
			"properties": map[string]interface{}{
				"task_id": map[string]interface{}{"type": "integer", "minimum": 1, "description": "自动化任务 ID"},
				"input":   map[string]interface{}{"type": "object", "description": "传递给任务的 JSON 输入", "additionalProperties": true},
			},
			"required": []string{"task_id"},
		},
		Annotations: map[string]interface{}{"readOnlyHint": false, "destructiveHint": true, "idempotentHint": false, "openWorldHint": false},
	},
}...)

func GetMCPService(c *gin.Context) {
	tools := make([]gin.H, 0, len(mcpTools))
	for _, tool := range mcpTools {
		tools = append(tools, gin.H{
			"name": tool.Name, "title": tool.Title, "description": tool.Description,
			"permission": tool.Permission, "read_only": tool.Annotations["readOnlyHint"],
		})
	}
	resp.Success(c, http.StatusOK, "成功", gin.H{
		"enabled": true, "endpoint": mcpEndpointURL(c), "transport": "streamable-http",
		"protocol_version": currentMCPProtocolVersion, "supported_protocol_versions": sortedMCPProtocolVersions(),
		"authentication": "Bearer API Token", "server_name": mcpServerName, "server_version": mcpServerVersion,
		"tools": tools,
	})
}

// MCPServer handles the stateless JSON response form of MCP Streamable HTTP.
// GET and DELETE intentionally return 405 because this server does not expose
// an SSE listening stream or stateful sessions.
func MCPServer(c *gin.Context) {
	if c.Request.Method != http.MethodPost {
		c.Header("Allow", http.MethodPost)
		c.Status(http.StatusMethodNotAllowed)
		return
	}
	if c.GetString("auth_type") != "api_token" {
		c.JSON(http.StatusUnauthorized, gin.H{"jsonrpc": "2.0", "error": gin.H{"code": -32001, "message": "MCP 服务必须使用 API Token 认证"}, "id": nil})
		return
	}
	client, ok := resolveMCPClient(c)
	if !ok {
		return
	}
	if !validMCPOrigin(c) {
		c.JSON(http.StatusForbidden, gin.H{"jsonrpc": "2.0", "error": gin.H{"code": -32002, "message": "Origin 不受信任"}, "id": nil})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, mcpMaxBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var req mcpRequest
	if err := decoder.Decode(&req); err != nil || req.JSONRPC != "2.0" || strings.TrimSpace(req.Method) == "" {
		mcpWriteError(c, nil, -32600, "无效的 JSON-RPC 请求", http.StatusBadRequest)
		return
	}
	if len(req.ID) == 0 || string(req.ID) == "null" {
		if req.Method == "notifications/initialized" || strings.HasPrefix(req.Method, "notifications/") {
			c.Status(http.StatusAccepted)
			return
		}
		mcpWriteError(c, nil, -32600, "请求必须包含非空 id", http.StatusBadRequest)
		return
	}
	if req.Method != "initialize" {
		protocol := strings.TrimSpace(c.GetHeader("MCP-Protocol-Version"))
		if protocol == "" {
			protocol = "2025-03-26"
		}
		if _, supported := supportedMCPProtocolVersions[protocol]; !supported {
			mcpWriteError(c, req.ID, -32600, "不支持的 MCP 协议版本", http.StatusBadRequest)
			return
		}
		updateMCPClientConnection(c, &client, "", "", protocol)
	}

	switch req.Method {
	case "initialize":
		mcpInitialize(c, req, client)
	case "ping":
		mcpWriteResult(c, req.ID, gin.H{})
	case "tools/list":
		mcpListTools(c, req.ID)
	case "tools/call":
		mcpCallTool(c, req)
	default:
		mcpWriteError(c, req.ID, -32601, "不支持的方法", http.StatusOK)
	}
}

func mcpInitialize(c *gin.Context, req mcpRequest, client models.MCPClient) {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
		ClientInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"clientInfo"`
	}
	if json.Unmarshal(req.Params, &params) != nil || strings.TrimSpace(params.ProtocolVersion) == "" {
		mcpWriteError(c, req.ID, -32602, "initialize 参数不正确", http.StatusOK)
		return
	}
	protocol := negotiateMCPProtocol(params.ProtocolVersion)
	updateMCPClientConnection(c, &client, params.ClientInfo.Name, params.ClientInfo.Version, protocol)
	mcpWriteResult(c, req.ID, gin.H{
		"protocolVersion": protocol,
		"capabilities":    gin.H{"tools": gin.H{"listChanged": false}},
		"serverInfo":      gin.H{"name": mcpServerName, "title": "观澜运维平台 MCP 服务", "version": mcpServerVersion},
		"instructions":    "使用平台工具查询告警、规则、拨测、自动化任务和数据源；用户明确要求且参数齐全时可创建无认证直连数据源，也可测试已保存数据源连接。还可查询故障中心、各类分组、通知记录、证书和执行记录，并按权限创建故障中心、分组、通知模板和邮件通知对象。可用 search_platform_operations 查找平台业务操作，describe_platform_operation 查看参数，再 execute_platform_operation 执行。具体告警规则使用 create_alert_rule/update_alert_rule。所有写操作须符合用户明确请求，失败或超时先查状态，不直接重试。",
	})
}

func mcpListTools(c *gin.Context, id json.RawMessage) {
	available := make([]mcpTool, 0, len(mcpTools))
	for _, tool := range mcpTools {
		if authorizeAssistantMCPTool(c.Request.Context(), c.GetUint("user_id"), tool) == nil {
			available = append(available, tool)
		}
	}
	mcpWriteResult(c, id, gin.H{"tools": available})
}

func mcpCallTool(c *gin.Context, req mcpRequest) {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if json.Unmarshal(req.Params, &params) != nil || strings.TrimSpace(params.Name) == "" {
		mcpWriteError(c, req.ID, -32602, "tools/call 参数不正确", http.StatusOK)
		return
	}
	tool, exists := findMCPTool(params.Name)
	if !exists {
		mcpWriteError(c, req.ID, -32602, "工具不存在", http.StatusOK)
		return
	}
	if authorizeAssistantMCPTool(c.Request.Context(), c.GetUint("user_id"), tool) != nil {
		mcpWriteToolResult(c, req.ID, gin.H{"error": "没有调用该工具的权限"}, true)
		return
	}
	var result interface{}
	var err error
	if isOperationMCPTool(tool.Name) {
		result, err = executeOperationMCPTool(c.Request.Context(), c.GetUint("user_id"), tool.Name, params.Arguments)
	} else if isPlatformMCPTool(tool.Name) {
		result, err = executePlatformMCPTool(c.Request.Context(), c.GetUint("user_id"), tool.Name, params.Arguments)
	} else if isDatasourceMCPTool(tool.Name) {
		result, err = executeDatasourceMCPTool(c.Request.Context(), c.GetUint("user_id"), tool.Name, params.Arguments)
	} else {
		result, err = executeMCPTool(tool.Name, params.Arguments)
		if err == nil && tool.Annotations["readOnlyHint"] == true {
			result, err = projectMCPQueryData(tool.Name, result)
		}
	}
	if err != nil {
		mcpWriteToolResult(c, req.ID, gin.H{"error": err.Error()}, true)
		return
	}
	mcpWriteToolResult(c, req.ID, result, false)
}

func executeMCPTool(name string, args map[string]interface{}) (interface{}, error) {
	switch name {
	case "query_alerts":
		return mcpQueryAlerts(args)
	case "query_alert_rules":
		return mcpQueryAlertRules(args)
	case "query_probe_tasks":
		return mcpQueryProbeTasks(args)
	case "query_automation_tasks":
		return mcpQueryAutomationTasks(args)
	case "run_automation_task":
		return mcpRunAutomationTask(args)
	default:
		return nil, fmt.Errorf("工具不存在")
	}
}

func mcpQueryAlerts(args map[string]interface{}) (interface{}, error) {
	return mcpQueryAlertsContext(context.Background(), args)
}

func mcpQueryAlertsContext(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	page, pageSize := mcpPagination(args)
	query := database.DB.WithContext(ctx).Model(&models.AlertInstance{})
	switch mcpStringArg(args, "status") {
	case "active":
		query = query.Where("status IN ?", []string{alertStatusPending, alertStatusFiring, alertStatusRecovering})
	case "history":
		query = query.Where("status IN ?", []string{alertStatusResolved, alertStatusTerminated})
	case alertStatusPending, alertStatusFiring, alertStatusRecovering, alertStatusResolved, alertStatusTerminated:
		query = query.Where("status = ?", mcpStringArg(args, "status"))
	}
	if value := mcpStringArg(args, "severity"); value != "" {
		query = query.Where("LOWER(severity) = ?", strings.ToLower(value))
	}
	if value := mcpStringArg(args, "source"); value != "" {
		query = query.Where("source = ?", value)
	}
	if value := mcpStringArg(args, "keyword"); value != "" {
		like := "%" + value + "%"
		query = query.Where("title LIKE ? OR target LIKE ? OR labels LIKE ?", like, like, like)
	}
	if value := mcpIntArg(args, "fault_center_id"); value > 0 {
		query = query.Where("fault_center_id = ?", value)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("查询告警失败")
	}
	var rows []models.AlertInstance
	if err := query.Order("last_event_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询告警失败")
	}
	attachAlertRuleDetails(rows)
	return gin.H{"list": rows, "total": total, "page": page, "page_size": pageSize}, nil
}

func mcpQueryAlertRules(args map[string]interface{}) (interface{}, error) {
	return mcpQueryAlertRulesContext(context.Background(), args)
}

func mcpQueryAlertRulesContext(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	page, pageSize := mcpPagination(args)
	query := database.DB.WithContext(ctx).Model(&models.AlertRule{})
	if value := mcpStringArg(args, "keyword"); value != "" {
		like := "%" + value + "%"
		query = query.Where("name LIKE ? OR expression LIKE ?", like, like)
	}
	if status := mcpStringArg(args, "status"); status == "enabled" {
		query = query.Where("status = 1")
	} else if status == "disabled" {
		query = query.Where("status = 0")
	}
	if value := mcpIntArg(args, "group_id"); value > 0 {
		query = query.Where("group_id = ?", value)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("查询告警规则失败")
	}
	var rows []models.AlertRule
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询告警规则失败")
	}
	items := make([]gin.H, 0, len(rows))
	for i := range rows {
		items = append(items, alertRuleResponse(rows[i]))
	}
	return gin.H{"list": items, "total": total, "page": page, "page_size": pageSize}, nil
}

func mcpQueryProbeTasks(args map[string]interface{}) (interface{}, error) {
	return mcpQueryProbeTasksContext(context.Background(), args)
}

func mcpQueryProbeTasksContext(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	page, pageSize := mcpPagination(args)
	query := database.DB.WithContext(ctx).Model(&models.InspectionItem{})
	if value := mcpStringArg(args, "type"); value != "" {
		query = query.Where("type = ?", value)
	}
	if status := mcpStringArg(args, "status"); status == "enabled" {
		query = query.Where("status = 1")
	} else if status == "disabled" {
		query = query.Where("status = 0")
	}
	if value := mcpStringArg(args, "keyword"); value != "" {
		like := "%" + value + "%"
		query = query.Where("name LIKE ? OR target LIKE ? OR labels LIKE ?", like, like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("查询拨测任务失败")
	}
	var rows []models.InspectionItem
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询拨测任务失败")
	}
	return gin.H{"list": inspectionItemListViews(rows), "total": total, "page": page, "page_size": pageSize}, nil
}

func mcpQueryAutomationTasks(args map[string]interface{}) (interface{}, error) {
	return mcpQueryAutomationTasksContext(context.Background(), args)
}

func mcpQueryAutomationTasksContext(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	page, pageSize := mcpPagination(args)
	query := database.DB.WithContext(ctx).Model(&models.AutomationTask{})
	if value := mcpStringArg(args, "keyword"); value != "" {
		like := "%" + value + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	if status := mcpStringArg(args, "status"); status == "enabled" {
		query = query.Where("status = 1")
	} else if status == "disabled" {
		query = query.Where("status = 0")
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("查询自动化任务失败")
	}
	var rows []models.AutomationTask
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询自动化任务失败")
	}
	return gin.H{"list": rows, "total": total, "page": page, "page_size": pageSize}, nil
}

func mcpRunAutomationTask(args map[string]interface{}) (interface{}, error) {
	taskID := mcpIntArg(args, "task_id")
	if taskID < 1 {
		return nil, fmt.Errorf("task_id 必须是正整数")
	}
	var task models.AutomationTask
	if err := database.DB.Where("id = ? AND status = 1", taskID).First(&task).Error; err != nil {
		return nil, fmt.Errorf("自动化任务不存在或已停用")
	}
	input := map[string]interface{}{}
	if value, ok := args["input"].(map[string]interface{}); ok {
		input = value
	} else if args["input"] != nil {
		return nil, fmt.Errorf("input 必须是 JSON 对象")
	}
	return executeAutomationTask(task, "mcp", 0, input), nil
}

func resolveMCPClient(c *gin.Context) (models.MCPClient, bool) {
	var client models.MCPClient
	if err := database.DB.Preload("APIToken.User").Where("api_token_id = ?", c.GetString("api_token_id")).First(&client).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"jsonrpc": "2.0", "error": gin.H{"code": -32003, "message": "当前 API Token 未登记为 MCP 连接"}, "id": nil})
		return client, false
	}
	if client.Status != 1 {
		c.JSON(http.StatusForbidden, gin.H{"jsonrpc": "2.0", "error": gin.H{"code": -32004, "message": "MCP 连接已停用"}, "id": nil})
		return client, false
	}
	return client, true
}

func updateMCPClientConnection(c *gin.Context, client *models.MCPClient, reportedName, clientVersion, protocolVersion string) {
	if client == nil {
		return
	}
	now := time.Now()
	clientVersion = truncateMCPText(clientVersion, 64)
	reportedName = truncateMCPText(reportedName, 100)
	changed := client.ProtocolVersion != protocolVersion ||
		(clientVersion != "" && client.ClientVersion != clientVersion) ||
		(reportedName != "" && client.ReportedName != reportedName)
	if !changed && client.LastSeenAt != nil && client.LastSeenAt.After(now.Add(-time.Minute)) {
		return
	}
	updates := map[string]interface{}{
		"last_seen_at": now, "last_ip": truncateMCPText(c.ClientIP(), 64),
		"last_user_agent": truncateMCPText(c.Request.UserAgent(), 500), "protocol_version": protocolVersion,
	}
	if clientVersion != "" {
		updates["client_version"] = clientVersion
	}
	if reportedName != "" {
		updates["reported_name"] = reportedName
	}
	_ = database.DB.Model(client).Updates(updates).Error
}

func mcpUserHasPermission(userID uint, permission string) bool {
	if userID == 0 || permission == "" {
		return false
	}
	var count int64
	database.DB.Table("permissions AS p").
		Joins("JOIN role_permissions AS rp ON rp.permission_id = p.id").
		Joins("JOIN roles AS r ON r.id = rp.role_id AND r.status = 1").
		Joins("JOIN user_roles AS ur ON ur.role_id = r.id").
		Where("ur.user_id = ? AND p.status = 1 AND p.code = ?", userID, permission).
		Count(&count)
	return count > 0
}

func validMCPOrigin(c *gin.Context) bool {
	origin := strings.TrimSpace(c.GetHeader("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	return strings.EqualFold(parsed.Host, c.Request.Host)
}

func negotiateMCPProtocol(requested string) string {
	if _, ok := supportedMCPProtocolVersions[requested]; ok {
		return requested
	}
	return currentMCPProtocolVersion
}

func findMCPTool(name string) (mcpTool, bool) {
	for _, tool := range mcpTools {
		if tool.Name == name {
			return tool, true
		}
	}
	return mcpTool{}, false
}

func mcpWriteResult(c *gin.Context, id json.RawMessage, result interface{}) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"jsonrpc": "2.0", "id": id, "result": result})
}

func mcpWriteError(c *gin.Context, id json.RawMessage, code int, message string, status int) {
	c.Header("Cache-Control", "no-store")
	c.JSON(status, gin.H{"jsonrpc": "2.0", "id": id, "error": gin.H{"code": code, "message": message}})
}

func mcpWriteToolResult(c *gin.Context, id json.RawMessage, result interface{}, isError bool) {
	encoded, err := json.Marshal(result)
	if err != nil {
		mcpWriteError(c, id, -32603, "工具结果序列化失败", http.StatusOK)
		return
	}
	mcpWriteResult(c, id, gin.H{
		"content":           []gin.H{{"type": "text", "text": string(encoded)}},
		"structuredContent": result, "isError": isError,
	})
}

func mcpEndpointURL(c *gin.Context) string {
	scheme := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		} else {
			scheme = "http"
		}
	}
	return scheme + "://" + c.Request.Host + "/api/mcp"
}

func sortedMCPProtocolVersions() []string {
	return []string{"2025-03-26", "2025-06-18", "2025-11-25"}
}

func mcpObjectSchema(properties map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"type": "object", "additionalProperties": false, "properties": properties}
}

func mcpStringEnum(values ...string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "enum": values}
}

func mcpPageSchema(defaultValue int) map[string]interface{} {
	return map[string]interface{}{"type": "integer", "minimum": 1, "default": defaultValue}
}

func mcpPageSizeSchema() map[string]interface{} {
	return map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}
}

func mcpReadOnlyAnnotations() map[string]interface{} {
	return map[string]interface{}{"readOnlyHint": true, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false}
}

func mcpPagination(args map[string]interface{}) (int, int) {
	page, pageSize := mcpIntArg(args, "page"), mcpIntArg(args, "page_size")
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func mcpStringArg(args map[string]interface{}, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func mcpIntArg(args map[string]interface{}, key string) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case json.Number:
		result, _ := strconv.Atoi(value.String())
		return result
	case int:
		return value
	default:
		return 0
	}
}
