package handlers

import (
	"context"
	"encoding/json"
	"errors"
	aicore "guanlan/internal/ai"
)

// Reuse the local MCP business handlers; no loopback HTTP or service token is needed.
func registerAssistantMCPTools(ctx context.Context, registry *aicore.Registry, userID uint) {
	for _, tool := range mcpTools {
		if (tool.Annotations["readOnlyHint"] != true && !tool.AssistantWritable) || authorizeAssistantMCPTool(ctx, userID, tool) != nil {
			continue
		}
		params := mcpParameterSchema(tool.InputSchema)
		risk := aicore.RiskReadonlyLow
		if tool.AssistantWritable {
			risk = aicore.RiskWriteLow
		}
		handler := assistantMCPTool
		if isOperationMCPTool(tool.Name) {
			handler = func(ctx context.Context, call aicore.ToolCall) aicore.ToolResult {
				data, err := executeOperationMCPTool(ctx, userID, call.Tool, call.Params)
				if err != nil {
					return aicore.ToolResult{Tool: call.Tool, ErrorCode: "PLATFORM_OPERATION_FAILED", Message: err.Error()}
				}
				return aicore.ToolResult{Tool: call.Tool, OK: true, Summary: "平台操作完成", Data: data}
			}
		} else if isPlatformMCPTool(tool.Name) {
			handler = func(ctx context.Context, call aicore.ToolCall) aicore.ToolResult {
				data, err := executePlatformMCPTool(ctx, userID, call.Tool, call.Params)
				if err != nil {
					return aicore.ToolResult{Tool: call.Tool, ErrorCode: "PLATFORM_OPERATION_FAILED", Message: err.Error()}
				}
				return aicore.ToolResult{Tool: call.Tool, OK: true, Summary: "平台操作完成", Data: data}
			}
		} else if isDatasourceMCPTool(tool.Name) {
			handler = func(ctx context.Context, call aicore.ToolCall) aicore.ToolResult {
				return assistantDatasourceTool(ctx, userID, call)
			}
		}
		timeout := 10
		if isOperationMCPTool(tool.Name) {
			timeout = 120
		}
		if isOperationMCPTool(tool.Name) && tool.AssistantWritable {
			risk = aicore.RiskWriteAuthorized
		}
		mustRegisterTool(registry, aicore.ToolSpec{Name: tool.Name, Description: tool.Description, Risk: risk, TimeoutSeconds: timeout, ParamsSchema: params}, handler)
	}
}

func assistantMCPTool(ctx context.Context, call aicore.ToolCall) aicore.ToolResult {
	var data interface{}
	var err error
	switch call.Tool {
	case "query_alerts":
		data, err = mcpQueryAlertsContext(ctx, call.Params)
	case "query_alert_rules":
		data, err = mcpQueryAlertRulesContext(ctx, call.Params)
	case "query_probe_tasks":
		data, err = mcpQueryProbeTasksContext(ctx, call.Params)
	case "query_automation_tasks":
		data, err = mcpQueryAutomationTasksContext(ctx, call.Params)
	default:
		return aicore.ToolResult{Tool: call.Tool, ErrorCode: "TOOL_NOT_ALLOWED", Message: "工具不可用"}
	}
	if err != nil {
		return aicore.ToolResult{Tool: call.Tool, ErrorCode: "QUERY_FAILED", Message: "平台查询失败"}
	}
	result, err := projectMCPQueryData(call.Tool, data)
	if err != nil {
		return aicore.ToolResult{Tool: call.Tool, ErrorCode: "RESULT_INVALID"}
	}
	return aicore.ToolResult{Tool: call.Tool, OK: true, Summary: "平台查询成功（平台级范围）", Data: result}
}

// Share the same projection between the assistant and remote MCP callers.
func projectMCPQueryData(name string, data interface{}) (map[string]interface{}, error) {
	// Explicit projection keeps commands, configuration and raw payloads out of model context.
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, errors.New("查询结果格式不正确")
	}
	var result map[string]interface{}
	if json.Unmarshal(raw, &result) != nil {
		return nil, errors.New("查询结果格式不正确")
	}
	fields := map[string][]string{
		"query_alerts":           {"id", "title", "status", "severity", "source", "fault_center_id", "started_at", "last_event_at", "fired_at", "resolved_at"},
		"query_alert_rules":      {"id", "name", "status", "severity", "group_id", "datasource_id", "fault_center_id", "expression", "for_seconds", "evaluation_interval_seconds", "last_status"},
		"query_probe_tasks":      {"id", "name", "type", "status", "last_status", "last_run_at", "alert_active", "group_id", "fault_center_id", "severity", "interval_seconds", "failure_threshold"},
		"query_automation_tasks": {"id", "name", "status", "source_type", "node_type", "node_id", "timeout_seconds"},
	}
	items := []map[string]interface{}{}
	if rows, ok := result["list"].([]interface{}); ok {
		for _, raw := range rows {
			row, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			item := map[string]interface{}{}
			for _, key := range fields[name] {
				if value, ok := row[key]; ok {
					item[key] = value
				}
			}
			items = append(items, item)
		}
	}
	result["list"] = items
	return result, nil
}

func authorizeAssistantMCPTool(ctx context.Context, userID uint, tool mcpTool) error {
	if isOperationMCPTool(tool.Name) && tool.Permission == operationDirectoryPermission {
		return authorizeOperationDirectory(ctx, userID)
	}
	return authorizeAIPermission(ctx, userID, tool.Permission)
}
