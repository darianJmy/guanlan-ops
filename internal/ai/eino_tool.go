package ai

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

var _ tool.InvokableTool = (*policyTool)(nil)

type policyTool struct {
	spec    ToolSpec
	runtime *agentRuntime
}

func (t *policyTool) Info(context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: t.spec.Name, Desc: t.spec.Description, ParamsOneOf: schema.NewParamsOneOfByParams(t.spec.ParamsSchema)}, nil
}

func (t *policyTool) InvokableRun(ctx context.Context, input string, _ ...tool.Option) (string, error) {
	return t.runtime.invoke(ctx, t.spec.Name, input)
}
func (r *agentRuntime) invoke(ctx context.Context, name, input string) (string, error) {
	start := time.Now()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	r.toolCalls++
	if r.orchestrator.MaxToolCalls > 0 && r.toolCalls > r.orchestrator.MaxToolCalls {
		return "", errRoundLimit
	}
	var params map[string]interface{}
	result := ToolResult{Tool: name, ErrorCode: "INVALID_PARAMS", Message: "工具参数必须是 JSON 对象"}
	if len(input) <= 64*1024 && json.Unmarshal([]byte(input), &params) == nil && params != nil {
		result = r.orchestrator.execute(ctx, ToolCall{Tool: name, Params: params})
	}
	status := "succeeded"
	if !result.OK {
		status = "failed"
	}
	safeName := name
	if _, ok := r.orchestrator.Registry.Get(name); !ok {
		safeName = "unknown"
	}
	r.trace.Steps = append(r.trace.Steps, StepTrace{Round: r.round, Kind: "tool", Tool: safeName, Status: status, DurationMS: time.Since(start).Milliseconds(), ErrorCode: result.ErrorCode})
	return boundedResult(result, r.orchestrator.MaxResultBytes), nil
}
