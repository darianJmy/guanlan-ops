package ai

import (
	"context"
	"fmt"
	"github.com/cloudwego/eino/schema"
	"math"
	"time"
	"unicode/utf8"
)

// StepTrace deliberately excludes arguments, results and provider error messages.
type StepTrace struct {
	Round      int    `json:"round"`
	Kind       string `json:"kind"`
	Tool       string `json:"tool,omitempty"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	ErrorCode  string `json:"error_code,omitempty"`
}

type RunTrace struct {
	ID         string      `json:"run_id"`
	Status     string      `json:"status"`
	DurationMS int64       `json:"duration_ms"`
	Steps      []StepTrace `json:"steps"`
}

func (o *Orchestrator) execute(ctx context.Context, call ToolCall) ToolResult {
	denied := func(code, message string) ToolResult {
		return ToolResult{Tool: call.Tool, ErrorCode: code, Message: message}
	}
	tool, ok := o.Registry.Get(call.Tool)
	if !ok {
		return denied("UNKNOWN_TOOL", "工具未注册")
	}
	if tool.Spec.Risk != RiskReadonlyLow && tool.Spec.Risk != RiskReadonlyExpensive && tool.Spec.Risk != RiskWriteLow && tool.Spec.Risk != RiskWriteAuthorized {
		return denied("READ_ONLY_REQUIRED", "该风险等级的操作尚未向 AI 助手开放")
	}
	if err := validateParams(tool.Spec, call.Params); err != nil {
		return denied("INVALID_PARAMS", err.Error())
	}
	if o.Authorize == nil {
		return denied("FORBIDDEN", "未配置工具授权策略")
	}
	if err := o.Authorize(ctx, tool.Spec, call); err != nil {
		return denied("FORBIDDEN", "没有执行该工具的权限")
	}
	seconds := tool.Spec.TimeoutSeconds
	if seconds <= 0 {
		seconds = 15
	}
	toolCtx, cancel := context.WithTimeout(ctx, time.Duration(seconds)*time.Second)
	defer cancel()
	if toolCtx.Err() != nil {
		return denied("CANCELLED", "调用已停止")
	}
	result := tool.Handler(toolCtx, call)
	if toolCtx.Err() != nil {
		return denied("TOOL_TIMEOUT", "工具调用已超时或取消")
	}
	return result
}

// ValidateToolParams exposes the execution contract to local MCP adapters as well.
func ValidateToolParams(spec ToolSpec, params map[string]interface{}) error {
	return validateParams(spec, params)
}

// Validate the same Eino parameter contract used to generate the model's JSON Schema.
func validateParams(spec ToolSpec, params map[string]interface{}) error {
	return validateObject(spec.ParamsSchema, params, 0)
}
func validateObject(defs map[string]*schema.ParameterInfo, values map[string]interface{}, depth int) error {
	if depth > 16 {
		return fmt.Errorf("参数嵌套过深")
	}
	for name := range values {
		if _, ok := defs[name]; !ok {
			return fmt.Errorf("不支持的参数")
		}
	}
	for name, def := range defs {
		if def == nil {
			return fmt.Errorf("工具参数定义无效")
		}
		value, exists := values[name]
		if !exists {
			if def.Required {
				return fmt.Errorf("缺少参数 %s", name)
			}
			continue
		}
		if err := validateValue(def, value, depth+1); err != nil {
			return fmt.Errorf("参数 %s: %w", name, err)
		}
		if name == "datasource_id" {
			if n, ok := value.(float64); !ok || n <= 0 {
				return fmt.Errorf("数据源 ID 必须是正整数")
			}
		}
	}
	return nil
}
func validateValue(def *schema.ParameterInfo, value interface{}, depth int) error {
	if def == nil || depth > 16 {
		return fmt.Errorf("参数定义无效或嵌套过深")
	}
	switch def.Type {
	case schema.String:
		text, ok := value.(string)
		if !ok || len(text) > 8192 {
			return fmt.Errorf("必须是有效字符串")
		}
		if len(def.Enum) > 0 {
			for _, allowed := range def.Enum {
				if text == allowed {
					return nil
				}
			}
			return fmt.Errorf("不在允许范围")
		}
	case schema.Integer, schema.Number:
		n, ok := value.(float64)
		if !ok || math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > float64(1<<53-1) {
			return fmt.Errorf("必须是有效数字")
		}
		if def.Type == schema.Integer && math.Trunc(n) != n {
			return fmt.Errorf("必须是整数")
		}
	case schema.Boolean:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("必须是布尔值")
		}
	case schema.Object:
		values, ok := value.(map[string]interface{})
		if !ok {
			return fmt.Errorf("必须是对象")
		}
		return validateObject(def.SubParams, values, depth)
	case schema.Array:
		values, ok := value.([]interface{})
		if !ok || len(values) > 1000 {
			return fmt.Errorf("必须是不超过1000项的数组")
		}
		for _, item := range values {
			if err := validateValue(def.ElemInfo, item, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("不支持的参数类型")
	}
	return nil
}

func boundedResult(result ToolResult, limit int) string {
	text := MarshalJSONCompact(result)
	if limit <= 0 {
		limit = 64 * 1024
	}
	if len(text) <= limit {
		return text
	}
	// Keep a valid JSON envelope. Do not send a partial JSON document to the model.
	result.Data = nil
	result.Summary = "查询结果过大，请缩小范围后重新查询。"
	result.Message = ""
	result.Meta = map[string]interface{}{"truncated_for_llm": true, "original_bytes": len(text)}
	text = MarshalJSONCompact(result)
	if len(text) > limit {
		text = `{"ok":false,"error_code":"RESULT_TOO_LARGE"}`
	}
	if !utf8.ValidString(text) {
		return `{"ok":false,"error_code":"INVALID_ENCODING"}`
	}
	return text
}
