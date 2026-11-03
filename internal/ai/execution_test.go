package ai

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func testOrchestrator(t *testing.T, handler ToolHandler) *Orchestrator {
	t.Helper()
	registry := NewRegistry()
	if err := registry.Register(ToolSpec{Name: "query", Risk: RiskReadonlyLow, TimeoutSeconds: 1, ParamsSchema: map[string]*schema.ParameterInfo{"datasource_id": {Type: schema.Integer, Required: true}}}, handler); err != nil {
		t.Fatal(err)
	}
	return &Orchestrator{Registry: registry, Authorize: func(context.Context, ToolSpec, ToolCall) error { return nil }}
}

func TestExecutionTraceSurvivesModelFailure(t *testing.T) {
	calls := 0
	o := testOrchestrator(t, func(context.Context, ToolCall) ToolResult {
		calls++
		return ToolResult{OK: true, Data: "private-result"}
	})
	round := 0
	o.Model = fakeModel(func(context.Context, []*schema.Message) (*schema.Message, error) {
		round++
		if round == 1 {
			return toolMessage("query", `{"datasource_id":1}`), nil
		}
		return nil, errors.New("secret-provider-token")
	})
	result := o.Run(context.Background(), RunRequest{Question: "check"})
	trace := result.Metadata["execution"].(RunTrace)
	if calls != 1 || trace.Status != "failed" || len(trace.Steps) != 3 || trace.Steps[1].Tool != "query" {
		t.Fatalf("lost steps: %#v", trace)
	}
	metadata := MarshalJSONCompact(result.Metadata)
	for _, secret := range []string{"private-result", "secret-provider-token", "datasource_id"} {
		if strings.Contains(metadata, secret) {
			t.Fatalf("trace exposed %s", secret)
		}
	}
	if !result.Answered || len(trace.ID) != 32 {
		t.Fatalf("invalid response: %#v", result)
	}
}

func TestExecutionRejectsUnsafeCalls(t *testing.T) {
	for _, tc := range []struct {
		name, tool, risk, code string
		params                 map[string]interface{}
		denied                 bool
	}{
		{name: "unknown", tool: "missing", risk: string(RiskReadonlyLow), code: "UNKNOWN_TOOL"},
		{name: "write", tool: "query", risk: string(RiskWriteConfirm), code: "READ_ONLY_REQUIRED"},
		{name: "missing", tool: "query", risk: string(RiskReadonlyLow), code: "INVALID_PARAMS"},
		{name: "scope injection", tool: "query", risk: string(RiskReadonlyLow), code: "INVALID_PARAMS", params: map[string]interface{}{"datasource_id": float64(1), "_workspace_id": float64(2)}},
		{name: "fractional id", tool: "query", risk: string(RiskReadonlyLow), code: "INVALID_PARAMS", params: map[string]interface{}{"datasource_id": 1.5}},
		{name: "unauthorized", tool: "query", risk: string(RiskReadonlyLow), code: "FORBIDDEN", params: map[string]interface{}{"datasource_id": float64(1)}, denied: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := testOrchestrator(t, func(context.Context, ToolCall) ToolResult { t.Fatal("denied tool executed"); return ToolResult{} })
			tool, _ := o.Registry.Get("query")
			tool.Spec.Risk = RiskLevel(tc.risk)
			o.Registry.tools["query"] = tool
			if tc.denied {
				o.Authorize = nil
			}
			result := o.execute(context.Background(), ToolCall{Tool: tc.tool, Params: tc.params})
			if result.OK || result.ErrorCode != tc.code {
				t.Fatalf("unexpected result: %#v", result)
			}
		})
	}
}

func TestExecutionCancellationStopsFurtherCalls(t *testing.T) {
	o := testOrchestrator(t, func(ctx context.Context, _ ToolCall) ToolResult { <-ctx.Done(); return ToolResult{OK: true} })
	o.Timeout = 10 * time.Millisecond
	calls := 0
	o.Model = fakeModel(func(context.Context, []*schema.Message) (*schema.Message, error) {
		calls++
		return toolMessage("query", `{"datasource_id":1}`), nil
	})
	result := o.Run(context.Background(), RunRequest{})
	trace := result.Metadata["execution"].(RunTrace)
	if calls != 1 || trace.Status != "timeout" || len(trace.Steps) != 2 || trace.Steps[1].ErrorCode != "TOOL_TIMEOUT" {
		t.Fatalf("unexpected trace: %#v", trace)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = o.Run(ctx, RunRequest{})
	if calls != 1 || result.Metadata["execution"].(RunTrace).Status != "cancelled" {
		t.Fatal("cancelled run called model")
	}
}

func TestExecutionRoundLimitAndSuccess(t *testing.T) {
	o := testOrchestrator(t, func(context.Context, ToolCall) ToolResult { return ToolResult{OK: true} })
	o.MaxRounds = 2
	calls := 0
	o.Model = fakeModel(func(context.Context, []*schema.Message) (*schema.Message, error) {
		calls++
		return toolMessage("query", `{"datasource_id":1}`), nil
	})
	result := o.Run(context.Background(), RunRequest{})
	if calls != 2 || result.Metadata["execution"].(RunTrace).Status != "limited" {
		t.Fatal("round limit ignored")
	}
	o.Model = fakeModel(func(context.Context, []*schema.Message) (*schema.Message, error) {
		return schema.AssistantMessage("完成", nil), nil
	})
	result = o.Run(context.Background(), RunRequest{})
	if result.Answer != "完成" || result.Metadata["execution"].(RunTrace).Status != "succeeded" {
		t.Fatalf("unexpected answer %#v", result)
	}
}

func TestBoundedResultIsValidJSON(t *testing.T) {
	text := boundedResult(ToolResult{OK: true, Data: strings.Repeat("中文数据", 10000)}, 1024)
	if len(text) > 1024 || !json.Valid([]byte(text)) || !utf8.ValidString(text) || !strings.Contains(text, "truncated_for_llm") {
		t.Fatalf("invalid truncated result %q", text)
	}
}

func TestToolReceivesDeclaredDeadline(t *testing.T) {
	o := testOrchestrator(t, func(ctx context.Context, _ ToolCall) ToolResult {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > time.Second {
			t.Fatal("declared timeout was not applied")
		}
		return ToolResult{OK: true}
	})
	result := o.execute(context.Background(), ToolCall{Tool: "query", Params: map[string]interface{}{"datasource_id": float64(1)}})
	if !result.OK {
		t.Fatalf("unexpected result: %#v", result)
	}
}

// Test double uses the same native Eino interface as production providers.
type fakeModel func(context.Context, []*schema.Message) (*schema.Message, error)

func (f fakeModel) Generate(ctx context.Context, m []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return f(ctx, m)
}
func (f fakeModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, errors.New("stream not used")
}
func (f fakeModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) { return f, nil }
func toolMessage(name, args string) *schema.Message {
	return schema.AssistantMessage("", []schema.ToolCall{{ID: "call_1", Type: "function", Function: schema.FunctionCall{Name: name, Arguments: args}}})
}

func TestEinoNativeToolMessagesAndUnknownTool(t *testing.T) {
	executed := 0
	o := testOrchestrator(t, func(context.Context, ToolCall) ToolResult { executed++; return ToolResult{OK: true, Data: "evidence"} })
	o.Model = fakeModel(func(_ context.Context, messages []*schema.Message) (*schema.Message, error) {
		last := messages[len(messages)-1]
		if last.Role == schema.User {
			return schema.AssistantMessage("", []schema.ToolCall{
				{ID: "one", Type: "function", Function: schema.FunctionCall{Name: "query", Arguments: `{"datasource_id":1}`}},
				{ID: "two", Type: "function", Function: schema.FunctionCall{Name: "unregistered", Arguments: `{}`}},
			}), nil
		}
		if last.Role != schema.Tool || last.ToolCallID != "two" || !strings.Contains(last.Content, "UNKNOWN_TOOL") {
			t.Fatalf("bad native tool result: %+v", last)
		}
		prior := messages[len(messages)-2]
		if prior.ToolCallID != "one" || !strings.Contains(prior.Content, "evidence") {
			t.Fatalf("missing evidence: %+v", prior)
		}
		return schema.AssistantMessage("完成", nil), nil
	})
	result := o.Run(context.Background(), RunRequest{})
	trace := result.Metadata["execution"].(RunTrace)
	if executed != 1 || result.Answer != "完成" || len(trace.Steps) != 4 || trace.Steps[2].Tool != "unknown" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestEinoPlainChatAndInvalidResponse(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message *schema.Message
		code    string
	}{
		{"plain", schema.AssistantMessage("你好", nil), ""},
		{"empty", nil, "INVALID_MODEL_RESPONSE"},
		{"missing call id", schema.AssistantMessage("", []schema.ToolCall{{Function: schema.FunctionCall{Name: "query", Arguments: "{}"}}}), "INVALID_MODEL_RESPONSE"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := &Orchestrator{Registry: NewRegistry(), Model: fakeModel(func(context.Context, []*schema.Message) (*schema.Message, error) { return tc.message, nil })}
			result := o.Run(context.Background(), RunRequest{Question: "hello"})
			if tc.code == "" {
				if result.Answer != "你好" {
					t.Fatalf("plain chat failed: %+v", result)
				}
			} else if result.Metadata["error_code"] != tc.code {
				t.Fatalf("invalid response accepted: %+v", result)
			}
		})
	}
}

func TestRegistryRejectsDuplicateTool(t *testing.T) {
	o := testOrchestrator(t, func(context.Context, ToolCall) ToolResult { return ToolResult{OK: true} })
	original, _ := o.Registry.Get("query")
	if err := o.Registry.Register(original.Spec, original.Handler); err == nil {
		t.Fatal("duplicate tool silently overwrote registration")
	}
}

func TestNativeParameterContract(t *testing.T) {
	spec := ToolSpec{ParamsSchema: map[string]*schema.ParameterInfo{
		"filter": {Type: schema.Object, Required: true, SubParams: map[string]*schema.ParameterInfo{
			"statuses": {Type: schema.Array, Required: true, ElemInfo: &schema.ParameterInfo{Type: schema.String, Enum: []string{"active", "history"}}},
			"enabled":  {Type: schema.Boolean},
		}},
	}}
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{`{"filter":{"statuses":["active"],"enabled":true}}`, true},
		{`{"filter":{"statuses":["invalid"]}}`, false},
		{`{"filter":{"statuses":null}}`, false},
		{`{"filter":{"statuses":[],"unexpected":1}}`, false},
		{`{"filter":{}}`, false},
		{`{}`, false},
	} {
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(tc.raw), &params); err != nil {
			t.Fatal(err)
		}
		if err := validateParams(spec, params); (err == nil) != tc.valid {
			t.Fatalf("%s: %v", tc.raw, err)
		}
	}
	info, err := (&policyTool{spec: spec}).Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.ParamsOneOf == nil {
		t.Fatal("native schema not exposed")
	}
}

func TestExecutionDiagnosticBudgetsAndPrompt(t *testing.T) {
	calls := 0
	o := testOrchestrator(t, func(context.Context, ToolCall) ToolResult { calls++; return ToolResult{OK: true} })
	o.SystemPrompt = "diagnostic-only"
	o.MaxToolCalls = 1
	o.Model = fakeModel(func(_ context.Context, messages []*schema.Message) (*schema.Message, error) {
		if messages[0].Content != "diagnostic-only" {
			t.Fatal("diagnostic prompt not applied")
		}
		return toolMessage("query", `{"datasource_id":1}`), nil
	})
	result := o.Run(context.Background(), RunRequest{})
	if calls != 1 || result.Metadata["execution"].(RunTrace).Status != "limited" {
		t.Fatal("tool budget not enforced")
	}
	o.MaxInputBytes = 10
	o.Model = fakeModel(func(context.Context, []*schema.Message) (*schema.Message, error) {
		t.Fatal("oversized prompt reached provider")
		return nil, nil
	})
	result = o.Run(context.Background(), RunRequest{Question: strings.Repeat("x", 100)})
	if result.Metadata["execution"].(RunTrace).Status != "limited" {
		t.Fatal("input budget ignored")
	}
}
