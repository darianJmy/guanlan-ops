package handlers

import "guanlan/internal/models"

// Every query owns its model, projection and filters. Tool arguments cannot choose
// SQL tables/columns, and sensitive fields are never read into tool results.
type platformMCPQuery struct {
	name, title, permission, description string
	model                                interface{}
	columns                              []string
	keywordColumns                       []string
	filters                              map[string]string
	extra                                map[string]interface{}
	timeColumn                           string
}

func mcpText(description string, max int) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": description, "maxLength": max}
}
func mcpID(description string) map[string]interface{} {
	return map[string]interface{}{"type": "integer", "minimum": 1, "maximum": float64(9007199254740991), "description": description}
}
func mcpIDs(description string) map[string]interface{} {
	return map[string]interface{}{"type": "array", "items": mcpID("资源 ID"), "minItems": 1, "maxItems": 100, "description": description}
}
func mcpEnabled() map[string]interface{} {
	return map[string]interface{}{"type": "boolean", "description": "true 仅启用，false 仅停用；不填查询全部"}
}
func mcpStatus() map[string]interface{} {
	return mcpText("状态精确匹配；不填查询全部", 32)
}
func notificationTypeSchema() map[string]interface{} {
	return mcpStringEnum("feishu", "dingding", "qywechat", "email", "webhook")
}

func platformMCPQueries() []platformMCPQuery {
	return []platformMCPQuery{
		{name: "query_fault_centers", title: "查询故障中心", permission: "alert:event:list", description: "查询故障中心 ID、名称、描述、启用状态及默认中心标记，供创建规则等操作选择。", model: &models.FaultCenter{}, columns: []string{"id", "name", "description", "status", "is_default", "created_at"}, keywordColumns: []string{"name", "description"}, extra: map[string]interface{}{"enabled": mcpEnabled()}},
		{name: "query_datasource_groups", title: "查询数据源分组", permission: "datasource:list", description: "查询数据源分组及成员数据源 ID；可与 query_datasources 配合查询实例名称。", model: &models.DatasourceGroup{}, columns: []string{"id", "name", "description", "created_at"}, keywordColumns: []string{"name", "description"}},
		{name: "query_alert_rule_groups", title: "查询告警规则组", permission: "alert:rule:list", description: "查询告警规则组 ID、名称和描述，区别于数据源分组与拨测规则组。", model: &models.AlertRuleGroup{}, columns: []string{"id", "name", "description", "created_at"}, keywordColumns: []string{"name", "description"}},
		{name: "query_probe_rule_groups", title: "查询拨测规则组", permission: "inspection:list", description: "查询拨测规则组 ID、名称和描述。", model: &models.InspectionRuleGroup{}, columns: []string{"id", "name", "description", "created_at"}, keywordColumns: []string{"name", "description"}},
		{name: "query_notification_channels", title: "查询通知对象", permission: "notification:channel:list", description: "查询通知对象名称、渠道类型、模板 ID 和启用状态；不返回 Webhook、签名密钥、邮箱地址或 SMTP 配置。", model: &models.NotificationChannel{}, columns: []string{"id", "name", "type", "template_id", "status", "created_at"}, keywordColumns: []string{"name"}, filters: map[string]string{"type": "type"}, extra: map[string]interface{}{"type": notificationTypeSchema(), "enabled": mcpEnabled()}},
		{name: "query_notification_templates", title: "查询通知模板", permission: "notification:template:list", description: "查询通知模板及正文，便于选用邮件等渠道模板；不执行或发送模板内容。", model: &models.NotificationTemplate{}, columns: []string{"id", "name", "type", "description", "content", "status", "created_at"}, keywordColumns: []string{"name", "description"}, filters: map[string]string{"type": "type"}, extra: map[string]interface{}{"type": notificationTypeSchema(), "enabled": mcpEnabled()}},
		{name: "query_notification_records", title: "查询通知记录", permission: "notification:record:list", description: "查询通知发送记录的状态、标题、渠道和时间。不返回原始请求、响应或可能含凭据的错误文本。", model: &models.NotificationRecord{}, columns: []string{"id", "channel_id", "channel_name", "event_type", "title", "status", "created_at"}, keywordColumns: []string{"channel_name", "title"}, filters: map[string]string{"channel_id": "channel_id", "status": "status"}, extra: map[string]interface{}{"channel_id": mcpID("通知对象 ID"), "status": mcpStringEnum("success", "failed")}, timeColumn: "created_at"},
		{name: "query_certificates", title: "查询证书资产", permission: "certificate:asset:list", description: "查询证书名称、域名、签发者和有效期；expires_within_days 查询未来指定天数内到期的证书（含已过期）。不返回证书 PEM 或私钥。", model: &models.Certificate{}, columns: []string{"id", "name", "domains", "issuer", "not_before", "not_after", "status", "environment", "created_at"}, keywordColumns: []string{"name", "domains"}, filters: map[string]string{"status": "status"}, extra: map[string]interface{}{"status": mcpStatus(), "expires_within_days": map[string]interface{}{"type": "integer", "minimum": 0, "maximum": 3650}}},
		{name: "query_certificate_domains", title: "查询域名证书状态", permission: "certificate:domain:list", description: "查询被监控域名的证书到期时间、剩余天数及最近检查状态；不返回部署参数、命令和错误原文。", model: &models.CertificateDomain{}, columns: []string{"id", "domain", "port", "certificate_id", "fault_center_id", "monitor_enabled", "observed_expires_at", "days_remaining", "last_check_status", "last_checked_at", "status"}, keywordColumns: []string{"domain"}, filters: map[string]string{"status": "status", "fault_center_id": "fault_center_id"}, extra: map[string]interface{}{"status": mcpStatus(), "fault_center_id": mcpID("故障中心 ID"), "expires_within_days": map[string]interface{}{"type": "integer", "minimum": 0, "maximum": 3650}}},
		{name: "query_probe_records", title: "查询拨测记录", permission: "inspection:list", description: "查询拨测执行记录的任务 ID、节点、状态、耗时和时间；不返回响应正文及原始结果。", model: &models.InspectionRecord{}, columns: []string{"id", "item_id", "node_type", "node_id", "node_name", "status", "duration_ms", "created_at"}, filters: map[string]string{"item_id": "item_id", "status": "status"}, extra: map[string]interface{}{"item_id": mcpID("拨测任务 ID"), "status": mcpStringEnum("success", "fail", "error")}, timeColumn: "created_at"},
		{name: "query_alert_events", title: "查询告警时间线", permission: "alert:event:list", description: "查询某条告警的状态、等级变化和评估状态，按时间倒序；不返回原始标签和载荷。", model: &models.AlertEvent{}, columns: []string{"id", "alert_id", "event_type", "from_status", "to_status", "from_severity", "to_severity", "evaluation_status", "created_at"}, filters: map[string]string{"alert_id": "alert_id"}, extra: map[string]interface{}{"alert_id": mcpID("告警 ID，先从 query_alerts 获取")}, timeColumn: "created_at"},
		{name: "query_alert_evaluations", title: "查询告警评估记录", permission: "alert:rule:list", description: "查询告警规则评估状态、命中数量、数据源及耗时，不触发评估、不返回错误原文。", model: &models.AlertEvaluationRecord{}, columns: []string{"id", "rule_id", "datasource_id", "status", "result_count", "duration_ms", "evaluated_at"}, filters: map[string]string{"rule_id": "rule_id", "datasource_id": "datasource_id", "status": "status"}, extra: map[string]interface{}{"rule_id": mcpID("规则 ID"), "datasource_id": mcpID("数据源 ID"), "status": mcpStatus()}, timeColumn: "evaluated_at"},
		{name: "query_automation_executions", title: "查询自动化执行记录", permission: "automation-task:list", description: "查询自动化任务执行状态、退出码、耗时和触发方式；不执行任务，不返回脚本、输入参数、输出或错误原文。", model: &models.AutomationExecution{}, columns: []string{"id", "task_id", "trigger_type", "trigger_id", "status", "exit_code", "duration_ms", "created_at"}, filters: map[string]string{"task_id": "task_id", "status": "status"}, extra: map[string]interface{}{"task_id": mcpID("自动化任务 ID"), "status": mcpStatus()}, timeColumn: "created_at"},
	}
}

func platformMCPTools() []mcpTool {
	tools := []mcpTool{}
	for _, query := range platformMCPQueries() {
		properties := map[string]interface{}{"id": mcpID("按记录 ID 精确查询"), "page": map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 1000000}, "page_size": mcpPageSizeSchema()}
		if len(query.keywordColumns) > 0 {
			properties["keyword"] = mcpText("名称或描述关键词", 200)
		}
		for key, def := range query.extra {
			properties[key] = def
		}
		if query.timeColumn != "" {
			properties["start_time"] = mcpText("开始时间，RFC3339，例如 2026-10-22T00:00:00+08:00", 64)
			properties["end_time"] = mcpText("结束时间，RFC3339", 64)
		}
		input := mcpObjectSchema(properties)
		if query.name == "query_alert_events" {
			input["required"] = []string{"alert_id"}
		}
		tools = append(tools, mcpTool{Name: query.name, Title: query.title, Permission: query.permission, Description: query.description + " 平台级范围，分页最多 100 条。", InputSchema: input, Annotations: mcpReadOnlyAnnotations()})
	}
	for _, write := range []struct {
		name, title, permission, description string
		extra                                map[string]interface{}
		required                             []string
	}{
		{"create_fault_center", "创建故障中心", "alert:rule:create", "按用户明确请求创建启用的普通故障中心，不配置通知或修改默认中心。", nil, nil},
		{"create_datasource_group", "创建数据源分组", "datasource:create", "创建包含指定已启用指标数据源的数据源分组。先查询真实数据源 ID；不修改现有分组。", map[string]interface{}{"datasource_ids": mcpIDs("已启用的 Prometheus/VictoriaMetrics 实例 ID 列表")}, []string{"datasource_ids"}},
		{"create_alert_rule_group", "创建告警规则组", "alert:rule:create", "创建空的告警规则组，不创建或启用告警规则。", nil, nil},
		{"create_probe_rule_group", "创建拨测规则组", "inspection:create", "创建空的拨测规则组，不创建或执行拨测任务。", nil, nil},
		{"create_notification_template", "创建通知模板", "notification:template:edit", "创建通知模板；正文必须由用户提供或用户明确要求生成。支持飞书、钉钉、企业微信、邮件和自定义 Webhook，保存不发送消息。", map[string]interface{}{"type": notificationTypeSchema(), "content": mcpText("通知正文，使用平台支持的 Go 模板语法", 12000)}, []string{"type", "content"}},
		{"create_email_notification_channel", "创建邮件通知对象", "notification:channel:edit", "使用全局 SMTP 配置创建邮件通知对象；需要真实启用的邮件模板 ID 和用户指定的收件人。仅保存，不发送邮件。", map[string]interface{}{"template_id": mcpID("启用的邮件模板 ID"), "email_recipients": map[string]interface{}{"type": "array", "items": mcpText("收件人邮箱", 320), "minItems": 1, "maxItems": 100}}, []string{"template_id", "email_recipients"}},
	} {
		props := map[string]interface{}{"name": mcpText("用户指定的名称", 128), "description": mcpText("可选描述", 500)}
		for key, def := range write.extra {
			props[key] = def
		}
		input := mcpObjectSchema(props)
		input["required"] = append([]string{"name"}, write.required...)
		tools = append(tools, mcpTool{Name: write.name, Title: write.title, Permission: write.permission, Description: write.description + " 参数不足先询问；不能编造资源 ID。返回实际创建 ID。", InputSchema: input, AssistantWritable: true, Annotations: map[string]interface{}{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": false, "openWorldHint": false}})
	}
	return tools
}

func isPlatformMCPTool(name string) bool {
	for _, tool := range platformMCPTools() {
		if tool.Name == name {
			return true
		}
	}
	return false
}
