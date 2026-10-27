package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"guanlan/internal/ai/provider"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	aicore "guanlan/internal/ai"
	"guanlan/internal/models"
)

func operationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := platformToolDB(t)
	if err := db.AutoMigrate(&models.AlertRule{}, &models.AlertRuleCondition{}, &models.AlertRuleBinding{}, &models.AlertRuleDatasource{}, &models.AlertRuleDatasourceGroup{}, &models.AlertInstance{}, &models.AlertEvent{}, &models.APIAccessToken{}, &models.MCPClient{}); err != nil {
		t.Fatal(err)
	}
	for _, op := range platformOperations {
		var permission models.Permission
		if err := db.Where("code = ?", op.Permission).Attrs(models.Permission{Name: op.Permission, Code: op.Permission, Status: 1}).FirstOrCreate(&permission).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("INSERT OR IGNORE INTO role_permissions(role_id,permission_id) VALUES(1,?)", permission.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []interface{}{&models.AlertRuleGroup{ID: 1, Name: "CPU"}, &models.FaultCenter{ID: 1, Name: "ops", Status: 1}, &models.Datasource{ID: 1, Name: "metrics", Type: "prometheus", URL: "http://metrics.invalid", Status: 1}} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}
func cpuRuleArgs() map[string]interface{} {
	return map[string]interface{}{
		"name": "CPU over 80%", "expression": "100 * (1 - avg by(instance)(rate(node_cpu_seconds_total{mode=\"idle\"}[5m])))",
		"group_id": 1, "fault_center_id": 1, "datasource_ids": []uint{1}, "conditions": []map[string]interface{}{{"severity": "P0", "operator": ">", "threshold": 80}},
		"status": 0, "evaluation_interval_seconds": 60, "for_seconds": 300, "recovery_wait_seconds": 60,
	}
}
func TestOperationCreateAndUpdateCPUAlert(t *testing.T) {
	db := operationTestDB(t)
	ctx := context.Background()
	registry := aicore.NewRegistry()
	registerAssistantMCPTools(ctx, registry, 1)
	tool, ok := registry.Get("create_alert_rule")
	if !ok {
		t.Fatal("CPU rule tool missing")
	}
	args := cpuRuleArgs()
	result := tool.Handler(ctx, aicore.ToolCall{Tool: "create_alert_rule", Params: args})
	if !result.OK {
		t.Fatalf("create failed: %+v", result)
	}
	var rule models.AlertRule
	if db.First(&rule).Error != nil || rule.Status != 0 || rule.Severity != "P0" || rule.Expression != args["expression"] {
		t.Fatalf("wrong rule: %+v", rule)
	}
	var conditions []models.AlertRuleCondition
	db.Where("rule_id = ?", rule.ID).Find(&conditions)
	if len(conditions) != 1 || conditions[0].Threshold != 80 || conditions[0].Operator != ">" || conditions[0].Severity != "P0" {
		t.Fatalf("threshold not saved: %+v", conditions)
	}
	var binding models.AlertRuleBinding
	if db.Where("rule_id = ?", rule.ID).First(&binding).Error != nil || binding.DatasourceID != 1 {
		t.Fatalf("binding not saved: %+v", binding)
	}
	db.Model(&rule).Updates(map[string]interface{}{"labels": `{"team":"ops"}`, "annotations": `{"description":"CPU"}`})
	args["id"] = rule.ID
	args["status"] = 1
	args["conditions"] = []map[string]interface{}{{"severity": "P1", "operator": ">=", "threshold": 90}}
	if _, err := executeOperationMCPTool(ctx, 1, "update_alert_rule", args); err != nil {
		t.Fatal(err)
	}
	db.First(&rule, rule.ID)
	db.Where("rule_id = ?", rule.ID).Find(&conditions)
	if rule.Status != 1 || rule.Severity != "P1" || conditions[0].Threshold != 90 || !strings.Contains(rule.Labels, "ops") || !strings.Contains(rule.Annotations, "CPU") {
		t.Fatalf("update lost fields: %+v %+v", rule, conditions)
	}
	var audit []models.AuditLog
	db.Where("resource_type = ?", "platform_operation").Find(&audit)
	if len(audit) != 2 || audit[0].Result != "成功" || audit[1].Result != "成功" {
		t.Fatalf("audit missing: %+v", audit)
	}
	raw, _ := json.Marshal(audit)
	if strings.Contains(string(raw), "node_cpu") {
		t.Fatal("request body leaked into audit")
	}
}

func TestOperationPermissionsValidationAndRevocation(t *testing.T) {
	db := operationTestDB(t)
	ctx := context.Background()
	// The directory itself is available to a reader, but write operations are not.
	listed, err := executeOperationMCPTool(ctx, 2, "search_platform_operations", map[string]interface{}{"keyword": "create_alert_rule"})
	if err != nil || listed.(map[string]interface{})["total"] != 0 {
		t.Fatalf("write operation disclosed: %v %v", listed, err)
	}
	raw, _ := json.Marshal(map[string]interface{}{"body": cpuRuleArgs()})
	args := map[string]interface{}{"operation": "create_alert_rule", "arguments_json": string(raw)}
	if _, err := executeOperationMCPTool(ctx, 2, "execute_platform_operation", args); err == nil {
		t.Fatal("reader wrote a rule")
	}
	for _, bad := range []map[string]interface{}{
		{"operation": "not_registered", "arguments_json": "{}"},
		{"operation": "create_alert_rule", "arguments_json": `{"body":{"user_id":1}}`},
		{"operation": "delete_alert_rule", "arguments_json": `{"path":{"id":"1 OR 1=1"}}`},
		{"operation": "delete_alert_rule", "arguments_json": `{"path":{"id":1.5}}`},
		{"operation": "delete_alert_rule", "arguments_json": `{"path":{"id":0}}`},
		{"operation": "create_alert_rule", "arguments_json": "null"},
		{"operation": "create_alert_rule", "arguments_json": "{}", "user_id": 1},
	} {
		if _, err := executeOperationMCPTool(ctx, 1, "execute_platform_operation", bad); err == nil {
			t.Fatalf("invalid args accepted: %v", bad)
		}
	}
	var count int64
	db.Model(&models.AlertRule{}).Count(&count)
	if count != 0 {
		t.Fatal("validation caused writes")
	}
	registry := aicore.NewRegistry()
	registerAssistantMCPTools(ctx, registry, 1)
	tool, _ := registry.Get("create_alert_rule")
	db.Exec("DELETE FROM role_permissions WHERE permission_id IN (SELECT id FROM permissions WHERE code='alert:rule:create')")
	result := tool.Handler(ctx, aicore.ToolCall{Tool: "create_alert_rule", Params: cpuRuleArgs()})
	if result.OK {
		t.Fatal("revoked permission accepted")
	}
	db.Model(&models.User{}).Where("id = 1").Update("status", 0)
	if _, err := executeOperationMCPTool(ctx, 1, "search_platform_operations", nil); err == nil {
		t.Fatal("disabled user can use directory")
	}
}

func TestOperationDirectoryCRUDAndStringIDs(t *testing.T) {
	db := operationTestDB(t)
	ctx := context.Background()
	// Verify wrapper, schema discovery, nested request decoding and real business handlers.
	result, err := executeOperationMCPTool(ctx, 1, "describe_platform_operation", map[string]interface{}{"operation": "create_notification_template"})
	if err != nil {
		t.Fatal(err)
	}
	if result.(map[string]interface{})["input_schema"] == nil {
		t.Fatal("missing schema")
	}
	args := map[string]interface{}{"operation": "create_notification_template", "arguments_json": `{"body":{"name":"ops","type":"email","content":"Alert {{ .alertname }}","status":1}}`}
	if _, err := executeOperationMCPTool(ctx, 1, "execute_platform_operation", args); err != nil {
		t.Fatal(err)
	}
	var template models.NotificationTemplate
	if db.First(&template).Error != nil {
		t.Fatal("template not created")
	}
	body, _ := json.Marshal(map[string]interface{}{"path": map[string]interface{}{"id": template.ID}})
	args = map[string]interface{}{"operation": "delete_notification_template", "arguments_json": string(body)}
	if _, err := executeOperationMCPTool(ctx, 1, "execute_platform_operation", args); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&models.NotificationTemplate{}).Count(&count)
	if count != 0 {
		t.Fatal("delete not executed")
	}
	id := "0123456789abcdef0123456789abcdef"
	if err := db.Create(&models.MCPClient{ID: id, Name: "test-client", APITokenID: "test-token", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	args = map[string]interface{}{"operation": "update_mcp_client_status", "arguments_json": `{"path":{"id":"` + id + `"},"body":{"status":0}}`}
	if _, err := executeOperationMCPTool(ctx, 1, "execute_platform_operation", args); err != nil {
		t.Fatal(err)
	}
	var client models.MCPClient
	db.First(&client, "id = ?", id)
	if client.Status != 0 {
		t.Fatal("string ID not forwarded")
	}
}

func TestOperationResponseRedactionAndFailure(t *testing.T) {
	db := operationTestDB(t)
	ctx := context.Background()
	secret := "NEVER-RETURN-THIS"
	value := map[string]interface{}{"token": secret, "api_token_id": "id-is-safe", "auth_config": `{"password":"` + secret + `"}`, "nested": []interface{}{map[string]interface{}{"private_key": secret, "url": "https://u:" + secret + "@example.com/?key=" + secret}}, "output": secret, "name": "visible"}
	raw, _ := json.Marshal(sanitizeOperationResult(value))
	if strings.Contains(string(raw), secret) || !strings.Contains(string(raw), "id-is-safe") || !strings.Contains(string(raw), "visible") {
		t.Fatalf("bad redaction: %s", raw)
	}
	original := platformOperations
	defer func() { platformOperations = original }()
	platformOperations = append(platformOperations, platformOperation{Name: "test_failure", Permission: "datasource:list", Method: "POST", Path: "/test", SchemaJSON: `{"type":"object","properties":{},"additionalProperties":false}`, Handler: func(c *gin.Context) { c.JSON(200, gin.H{"code": 400, "message": "validation failed"}) }})
	if _, err := executePlatformOperation(ctx, 1, "test_failure", map[string]interface{}{}); err == nil {
		t.Fatal("HTTP 200 business error reported as success")
	}
	var log models.AuditLog
	db.Order("id DESC").First(&log)
	if log.Result != "失败" {
		t.Fatal("failure audit missing")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := executePlatformOperation(cancelled, 1, "test_failure", map[string]interface{}{}); err == nil {
		t.Fatal("cancelled call executed")
	}
}

func TestOperationCatalogHasReviewedRouteCoverage(t *testing.T) {
	raw, err := os.ReadFile("../../cmd/generate-operation-catalog/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Operations []struct{ Name, Method, Path, Handler, Permission string }
		Excluded   []struct{ Method, Path, Reason string }
	}
	if json.Unmarshal(raw, &manifest) != nil {
		t.Fatal("invalid manifest")
	}
	routes := map[string]bool{}
	seen := map[string]bool{}
	for _, op := range manifest.Operations {
		routes[op.Method+" "+op.Path] = true
		if seen[op.Name] {
			t.Fatal("duplicate operation", op.Name)
		}
		seen[op.Name] = true
		runtime, ok := findPlatformOperation(op.Name)
		if !ok || runtime.Permission != op.Permission {
			t.Fatal("stale generated operation", op.Name)
		}
		if op.Method == "POST" || op.Method == "PUT" {
			schema := operationSchema(runtime)
			if schema["properties"] == nil {
				t.Fatal("missing request schema", op.Name)
			}
		}
	}
	for _, name := range []string{"create_alert_rule", "update_alert_rule", "create_inspection_item", "run_automation_task", "create_workflow", "update_system_settings", "assign_user_roles", "import_alert_rules"} {
		if !seen[name] {
			t.Fatal("missing business operation", name)
		}
	}
	for _, name := range []string{"create_api_access_token", "regenerate_api_access_token", "create_agent", "rotate_agent_token", "reset_password"} {
		if seen[name] {
			t.Fatal("one-time credential must not be lost in model redaction", name)
		}
	}
	for _, op := range manifest.Excluded {
		routes[op.Method+" "+op.Path] = true
		if op.Reason == "" {
			t.Fatal("unexplained exclusion")
		}
	}
	router, err := os.ReadFile("../routes/router.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range regexp.MustCompile(`auth\.(GET|POST|PUT|DELETE)\("([^"]+)"`).FindAllStringSubmatch(string(router), -1) {
		if !routes[match[1]+" "+match[2]] {
			t.Fatal("route missing reviewed tool decision:", match[1], match[2])
		}
	}
}

func TestOperationAssistantNativeRoundTrip(t *testing.T) {
	db := operationTestDB(t)
	registry := aicore.NewRegistry()
	registerAssistantMCPTools(context.Background(), registry, 1)
	arguments, _ := json.Marshal(map[string]interface{}{"body": cpuRuleArgs()})
	calls := []aicore.ToolCall{
		{Tool: "search_platform_operations", Params: map[string]interface{}{"keyword": "create_alert_rule"}},
		{Tool: "describe_platform_operation", Params: map[string]interface{}{"operation": "create_alert_rule"}},
		{Tool: "execute_platform_operation", Params: map[string]interface{}{"operation": "create_alert_rule", "arguments_json": string(arguments)}},
	}
	rounds := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rounds++
		var request struct {
			Messages []mockChatMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if rounds > 1 {
			last := request.Messages[len(request.Messages)-1]
			if last.Role != "tool" || !strings.Contains(last.Content, `"ok":true`) {
				t.Errorf("tool failed in round %d: %+v", rounds, last)
			}
		}
		if rounds <= len(calls) {
			call := calls[rounds-1]
			params, _ := json.Marshal(call.Params)
			json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "tool_calls": []interface{}{map[string]interface{}{"id": fmt.Sprintf("call_%d", rounds), "type": "function", "function": map[string]interface{}{"name": call.Tool, "arguments": string(params)}}}}, "finish_reason": "tool_calls"}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": mockChatMessage{Role: "assistant", Content: "CPU P0 规则已创建为停用状态"}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	model, err := provider.New(context.Background(), provider.Config{Provider: "newapi", BaseURL: server.URL, APIKey: "test", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	orchestrator := aicore.Orchestrator{Registry: registry, Model: model, Authorize: aiToolPolicy(1), MaxRounds: 5}
	response := orchestrator.Run(context.Background(), aicore.RunRequest{Question: "创建一条停用的 CPU 80% P0 告警规则，组和故障中心及数据源均使用 ID 1，间隔60秒持续300秒恢复60秒"})
	if rounds != 4 || response.Answer != "CPU P0 规则已创建为停用状态" {
		t.Fatalf("native roundtrip failed: %+v", response)
	}
	var count int64
	db.Model(&models.AlertRule{}).Where("severity = ? AND status = 0", "P0").Count(&count)
	if count != 1 {
		t.Fatal("native tool did not persist rule")
	}
	// External MCP calls use the same permission and business path.
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/api/mcp", nil)
	c.Set("user_id", uint(1))
	payload, _ := json.Marshal(map[string]interface{}{"name": "describe_platform_operation", "arguments": map[string]interface{}{"operation": "create_alert_rule"}})
	mcpCallTool(c, mcpRequest{JSONRPC: "2.0", ID: json.RawMessage("1"), Params: payload})
	if !strings.Contains(recorder.Body.String(), "input_schema") || strings.Contains(recorder.Body.String(), `"isError":true`) {
		t.Fatal("MCP bridge failed", recorder.Body.String())
	}
}

func TestOperationExportAndAuditFailure(t *testing.T) {
	db := operationTestDB(t)
	ctx := context.Background()
	if _, err := executeOperationMCPTool(ctx, 1, "create_alert_rule", cpuRuleArgs()); err != nil {
		t.Fatal(err)
	}
	result, err := executeOperationMCPTool(ctx, 1, "execute_platform_operation", map[string]interface{}{"operation": "export_alert_rules", "arguments_json": `{"query":{"group_id":"1"}}`})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	if !strings.Contains(string(raw), "CPU over 80%") || !strings.Contains(string(raw), "yaml") {
		t.Fatal("export not usable", string(raw))
	}
	if err := db.Migrator().DropTable(&models.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	args := cpuRuleArgs()
	args["name"] = "must not exist"
	if _, err := executeOperationMCPTool(ctx, 1, "create_alert_rule", args); err == nil {
		t.Fatal("write allowed without audit")
	}
	var count int64
	db.Model(&models.AlertRule{}).Where("name = ?", "must not exist").Count(&count)
	if count != 0 {
		t.Fatal("unrecorded mutation")
	}
}

func TestOperationPathIDs(t *testing.T) {
	if operationParameterString(float64(1000000)) != "1000000" {
		t.Fatal("numeric resource ID rendered in exponent notation")
	}
	for _, value := range []string{"", "..", "a/b", "x?y", "x\ny", "x%2fy"} {
		if validOperationPathValue(value) {
			t.Fatal("invalid path value accepted", value)
		}
	}
}
