package ai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

var errRoundLimit = errors.New("agent round limit")
var errInvalidModelResponse = errors.New("invalid model response")

// Run creates an isolated Eino graph and policy adapters for each conversation run.
// Eino owns message accumulation, native tool-call routing and the ReAct loop.
func (o *Orchestrator) Run(parent context.Context, req RunRequest) (response RunResponse) {
	started := time.Now()
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return RunResponse{Answered: true, Answer: "无法创建分析任务，请重试。"}
	}
	trace := RunTrace{ID: hex.EncodeToString(id), Status: "failed", Steps: []StepTrace{}}
	response.Metadata = map[string]interface{}{"orchestrator": true, "engine": "eino"}
	defer func() { trace.DurationMS = time.Since(started).Milliseconds(); response.Metadata["execution"] = trace }()
	fail := func(code, answer string) RunResponse {
		response.Answered = true
		response.Answer = answer
		response.Metadata["error_code"] = code
		return response
	}
	budget := o.Timeout
	if budget <= 0 {
		budget = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, budget)
	defer cancel()
	stopped := func() RunResponse {
		trace.Status = "cancelled"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			trace.Status = "timeout"
			return fail("TIMEOUT", "本次分析已超时，请缩小查询范围后重试。")
		}
		return fail("CANCELLED", "本次分析已停止。")
	}
	if ctx.Err() != nil {
		return stopped()
	}
	if o.Registry == nil || o.Model == nil {
		return fail("NOT_CONFIGURED", "AI 分析服务未配置完整。")
	}
	rounds := o.MaxRounds
	if rounds <= 0 {
		rounds = 8
	}
	runtime := &agentRuntime{orchestrator: o, trace: &trace, maxRounds: rounds}
	traced := &tracedModel{ToolCallingChatModel: o.Model, runtime: runtime}
	tools := make([]tool.BaseTool, 0, len(o.Registry.Specs()))
	for _, spec := range o.Registry.Specs() {
		if spec.Risk != RiskReadonlyLow && spec.Risk != RiskReadonlyExpensive && spec.Risk != RiskWriteLow && spec.Risk != RiskWriteAuthorized {
			continue
		}
		tools = append(tools, &policyTool{spec: spec, runtime: runtime})
	}
	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: traced,
		// Sequential execution preserves deterministic traces and avoids unbounded fan-out.
		ToolsConfig: compose.ToolsNodeConfig{Tools: tools, ExecuteSequentially: true, UnknownToolsHandler: runtime.invoke},
		MaxStep:     rounds*2 + 2, GraphName: "GuanlanAssistant",
	})
	if err != nil {
		return fail("NOT_CONFIGURED", "AI 分析服务未配置完整。")
	}
	answer, err := agent.Generate(ctx, []*schema.Message{schema.SystemMessage(o.systemPrompt()), schema.UserMessage(o.initialPrompt(req))})
	if ctx.Err() != nil {
		return stopped()
	}
	if errors.Is(err, errRoundLimit) {
		trace.Status = "limited"
		return fail("ROUND_LIMIT", "已达到本次分析的调用轮次上限，请缩小问题范围后重试。")
	}
	if errors.Is(err, errInvalidModelResponse) {
		return fail("INVALID_MODEL_RESPONSE", "AI 返回格式不正确，本次分析已停止，请重试。")
	}
	if err != nil {
		return fail("MODEL_ERROR", "AI 服务暂时无法完成分析，请检查模型配置或稍后重试。")
	}
	if answer == nil || strings.TrimSpace(answer.Content) == "" {
		return fail("INVALID_MODEL_RESPONSE", "AI 返回为空，请重试。")
	}
	trace.Status = "succeeded"
	response.Answered = true
	response.Answer = strings.TrimSpace(answer.Content)
	return response
}

// All adapters below are private to one Generate invocation. Tools execute serially.
type agentRuntime struct {
	orchestrator     *Orchestrator
	trace            *RunTrace
	round, maxRounds int
}

type tracedModel struct {
	model.ToolCallingChatModel
	runtime *agentRuntime
}

func (m *tracedModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	bound, err := m.ToolCallingChatModel.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &tracedModel{ToolCallingChatModel: bound, runtime: m.runtime}, nil
}
func (m *tracedModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	r := m.runtime
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if r.round >= r.maxRounds {
		return nil, errRoundLimit
	}
	r.round++
	start := time.Now()
	message, err := m.ToolCallingChatModel.Generate(ctx, input, opts...)
	step := StepTrace{Round: r.round, Kind: "model", Status: "succeeded", DurationMS: time.Since(start).Milliseconds()}
	if err == nil {
		if message == nil || (len(message.ToolCalls) == 0 && strings.TrimSpace(message.Content) == "") || (message.ResponseMeta != nil && message.ResponseMeta.FinishReason == "length") {
			err = errInvalidModelResponse
		}
		if message != nil && len(message.ToolCalls) > 8 {
			err = errInvalidModelResponse
		}
		seen := map[string]bool{}
		if message != nil {
			for _, call := range message.ToolCalls {
				if call.ID == "" || seen[call.ID] || call.Function.Name == "" {
					err = errInvalidModelResponse
				}
				seen[call.ID] = true
			}
		}
	}
	if err != nil {
		step.Status = "failed"
		step.ErrorCode = "MODEL_ERROR"
		if errors.Is(err, errInvalidModelResponse) {
			step.ErrorCode = "INVALID_MODEL_RESPONSE"
		}
	}
	r.trace.Steps = append(r.trace.Steps, step)
	return message, err
}
