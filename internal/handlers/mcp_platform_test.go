package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	aicore "guanlan/internal/ai"
	"guanlan/internal/ai/provider"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

func platformToolDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := datasourceToolDB(t)
	for _, query := range platformMCPQueries() {
		if err := db.AutoMigrate(query.model); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AutoMigrate(&models.DatasourceGroupMember{}); err != nil {
		t.Fatal(err)
	}
	for _, tool := range platformMCPTools() {
		var permission models.Permission
		if err := db.Where("code = ?", tool.Permission).Attrs(models.Permission{Name: tool.Permission, Code: tool.Permission, Status: 1}).FirstOrCreate(&permission).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("INSERT OR IGNORE INTO role_permissions(role_id,permission_id) VALUES(1,?)", permission.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func platformCall(t *testing.T, name string, args map[string]interface{}) map[string]interface{} {
	t.Helper()
	result, err := executePlatformMCPTool(context.Background(), 1, name, args)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return result.(map[string]interface{})
}

func TestPlatformMCPQueriesAndProjection(t *testing.T) {
	db := platformToolDB(t)
	now := time.Now()

	for _, row := range []interface{}{
		&models.FaultCenter{ID: 1, Name: "default", Status: 1, IsDefault: true},
		&models.DatasourceGroup{ID: 1, Name: "production"}, &models.DatasourceGroupMember{GroupID: 1, DatasourceID: 7},
		&models.AlertRuleGroup{ID: 1, Name: "alert group"}, &models.InspectionRuleGroup{ID: 1, Name: "probe group"},
		&models.NotificationChannel{ID: 1, Name: "mail", Type: "email", Status: 1, WebhookURLCipher: "hidden-secret", SignSecretCipher: "hidden-secret", EmailRecipientsJSON: `["hidden-secret@example.com"]`},
		&models.NotificationTemplate{ID: 1, Name: "mail template", Type: "email", Content: "告警 {{ .alertname }}", Status: 1},
		&models.NotificationRecord{ChannelID: 1, ChannelName: "mail", Status: "failed", Title: "failed alert", EventType: "alert", EventKey: "event", RequestBody: "hidden-secret", Response: "hidden-secret", Error: "hidden-secret", CreatedAt: now},
		&models.InspectionRecord{ItemID: 7, Status: "fail", Result: "hidden-secret", CreatedAt: now},
		&models.AlertEvent{AlertID: 7, EventType: "firing", Labels: "hidden-secret", Reason: "hidden-secret", CreatedAt: now},
		&models.AlertEvaluationRecord{RuleID: 7, DatasourceID: 7, Status: "error", Message: "hidden-secret", EvaluatedAt: now},
		&models.AutomationExecution{TaskID: 7, Status: "failed", Input: "hidden-secret", Output: "hidden-secret", Error: "hidden-secret", CreatedAt: now},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range platformMCPQueries() {
		t.Run(query.name, func(t *testing.T) {
			args := map[string]interface{}{}
			if query.name == "query_alert_events" {
				args["alert_id"] = 7
			}
			result := platformCall(t, query.name, args)
			raw, _ := json.Marshal(result)
			if strings.Contains(string(raw), "hidden-secret") {
				t.Fatalf("secret leaked: %s", raw)
			}
			if result["total"] != int64(1) {
				t.Fatalf("wrong total: %s", raw)
			}
			rows := result["list"].([]map[string]interface{})
			if len(rows) != 1 {
				t.Fatal("missing query result")
			}
			if _, err := executePlatformMCPTool(context.Background(), 999, query.name, args); err == nil {
				t.Fatal("unauthorized query succeeded")
			}
			args["sql"] = "SELECT * FROM users"
			if _, err := executePlatformMCPTool(context.Background(), 1, query.name, args); err == nil {
				t.Fatal("unknown parameter accepted")
			}
		})
	}
	if got := platformCall(t, "query_notification_records", map[string]interface{}{"status": "success"})["total"]; got != int64(0) {
		t.Fatal("status filter ignored")
	}
	if got := platformCall(t, "query_fault_centers", map[string]interface{}{"enabled": false})["total"]; got != int64(0) {
		t.Fatal("boolean filter ignored")
	}
	groups := platformCall(t, "query_datasource_groups", nil)["list"].([]map[string]interface{})
	if len(groups[0]["datasource_ids"].([]uint)) != 1 {
		t.Fatal("group members missing")
	}
	for _, args := range []map[string]interface{}{
		{"page_size": 101}, {"id": 0}, {"id": 1.5}, {"status": nil}, {"start_time": "yesterday"},
		{"start_time": "2026-10-23T00:00:00Z", "end_time": "2026-10-22T00:00:00Z"},
	} {
		if _, err := executePlatformMCPTool(context.Background(), 1, "query_notification_records", args); err == nil {
			t.Fatalf("bad filters accepted: %v", args)
		}
	}
}

func TestPlatformMCPCreationAndPermissions(t *testing.T) {
	db := platformToolDB(t)
	if err := db.Create(&models.Datasource{ID: 7, Name: "prom", Type: "prometheus", URL: "http://example.com", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.NotificationTemplate{ID: 7, Name: "existing email", Type: "email", Content: "message", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	registry := aicore.NewRegistry()
	registerAssistantMCPTools(context.Background(), registry, 1)
	for _, name := range []string{"run_automation_task", "delete_datasource", "delete_fault_center"} {
		if _, ok := registry.Get(name); ok {
			t.Fatalf("unexpected high-impact tool %s", name)
		}
	}
	cases := []struct {
		name  string
		args  map[string]interface{}
		model interface{}
	}{
		{"create_fault_center", map[string]interface{}{"name": "production"}, &models.FaultCenter{}},
		{"create_datasource_group", map[string]interface{}{"name": "metrics", "datasource_ids": []uint{7}}, &models.DatasourceGroup{}},
		{"create_alert_rule_group", map[string]interface{}{"name": "alerts"}, &models.AlertRuleGroup{}},
		{"create_probe_rule_group", map[string]interface{}{"name": "probes"}, &models.InspectionRuleGroup{}},
		{"create_notification_template", map[string]interface{}{"name": "new mail", "type": "email", "content": "告警 {{ .alertname }}"}, &models.NotificationTemplate{}},
		{"create_email_notification_channel", map[string]interface{}{"name": "ops", "template_id": 7, "email_recipients": []string{"ops@example.com", "oncall@example.com"}}, &models.NotificationChannel{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			registered, ok := registry.Get(tc.name)
			if !ok || registered.Spec.Risk != aicore.RiskWriteLow {
				t.Fatal("write tool missing from assistant")
			}
			if _, err := executePlatformMCPTool(context.Background(), 2, tc.name, tc.args); err == nil {
				t.Fatal("reader wrote platform resources")
			}
			result := platformCall(t, tc.name, tc.args)
			if result["id"].(uint) == 0 {
				t.Fatal("created ID missing")
			}
			if _, err := executePlatformMCPTool(context.Background(), 1, tc.name, tc.args); err == nil {
				t.Fatal("duplicate accepted")
			}
			var count int64
			db.Model(tc.model).Where("name = ?", tc.args["name"]).Count(&count)
			if count != 1 {
				t.Fatal("create not persisted once")
			}
		})
	}
	var auditCount int64
	db.Model(&models.AuditLog{}).Where("user_id = 1").Count(&auditCount)
	if auditCount != int64(len(cases)) {
		t.Fatal("missing creation audit")
	}
	var members int64
	db.Model(&models.DatasourceGroupMember{}).Count(&members)
	if members != 1 {
		t.Fatal("group membership not saved")
	}
	var channel models.NotificationChannel
	db.Where("name = ?", "ops").First(&channel)
	if channel.Type != "email" || channel.TemplateID != 7 || !strings.Contains(channel.EmailRecipientsJSON, "oncall@example.com") {
		t.Fatal("email channel not configured")
	}
	for _, bad := range []struct {
		name string
		args map[string]interface{}
	}{
		{"create_fault_center", map[string]interface{}{"name": " ", "is_default": true}},
		{"create_datasource_group", map[string]interface{}{"name": "bad", "datasource_ids": []uint{999}}},
		{"create_datasource_group", map[string]interface{}{"name": "bad", "datasource_ids": []uint{}}},
		{"create_notification_template", map[string]interface{}{"name": "bad", "type": "email", "content": "{{"}},
		{"create_email_notification_channel", map[string]interface{}{"name": "bad", "template_id": 999, "email_recipients": []string{"ops@example.com"}}},
		{"create_email_notification_channel", map[string]interface{}{"name": "bad", "template_id": 7, "email_recipients": []string{"not email"}}},
	} {
		if _, err := executePlatformMCPTool(context.Background(), 1, bad.name, bad.args); err == nil {
			t.Fatalf("invalid create accepted: %s", bad.name)
		}
	}
	db.Exec("UPDATE permissions SET status=0 WHERE code='alert:rule:create'")
	tool, _ := registry.Get("create_fault_center")
	if result := tool.Handler(context.Background(), aicore.ToolCall{Tool: "create_fault_center", Params: map[string]interface{}{"name": "revoked"}}); result.OK {
		t.Fatal("revoked permission ignored")
	}
}

func TestPlatformMCPAuditFailureRollsBack(t *testing.T) {
	db := platformToolDB(t)
	if err := db.Migrator().DropTable(&models.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	if _, err := executePlatformMCPTool(context.Background(), 1, "create_fault_center", map[string]interface{}{"name": "rollback"}); err == nil {
		t.Fatal("audit failure accepted")
	}
	var count int64
	db.Model(&models.FaultCenter{}).Count(&count)
	if count != 0 {
		t.Fatal("unlogged create was committed")
	}
}

func TestPlatformMCPRemoteCreation(t *testing.T) {
	db := platformToolDB(t)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	c.Set("user_id", uint(1))
	params, _ := json.Marshal(map[string]interface{}{"name": "create_fault_center", "arguments": map[string]interface{}{"name": "remote"}})
	mcpCallTool(c, mcpRequest{ID: json.RawMessage(`1`), Params: params})
	if !strings.Contains(recorder.Body.String(), "remote") || strings.Contains(recorder.Body.String(), `"isError":true`) {
		t.Fatal(recorder.Body.String())
	}
	var count int64
	db.Model(&models.FaultCenter{}).Where("name = ?", "remote").Count(&count)
	if count != 1 {
		t.Fatal("remote create did not persist")
	}
}

func TestPlatformMCPAssistantWorkflow(t *testing.T) {
	db := platformToolDB(t)
	db.Create(&models.Datasource{ID: 7, Name: "prom", Type: "prometheus", URL: "http://example.com", Status: 1})
	registry := aicore.NewRegistry()
	registerAssistantMCPTools(context.Background(), registry, 1)
	rounds := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rounds++
		var request map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if rounds == 1 {
			found := false
			for _, raw := range request["tools"].([]interface{}) {
				fn := raw.(map[string]interface{})["function"].(map[string]interface{})
				if fn["name"] == "create_datasource_group" {
					found = true
					properties := fn["parameters"].(map[string]interface{})["properties"].(map[string]interface{})
					if properties["datasource_ids"].(map[string]interface{})["type"] != "array" {
						t.Error("array schema lost")
					}
				}
			}
			if !found {
				t.Error("group creation tool not advertised")
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "tool_calls": []interface{}{map[string]interface{}{"id": "group_create", "type": "function", "function": map[string]interface{}{"name": "create_datasource_group", "arguments": `{"name":"production","datasource_ids":[7]}`}}}}, "finish_reason": "tool_calls"}}})
			return
		}
		raw, _ := json.Marshal(request)
		if !strings.Contains(string(raw), "production") {
			t.Error("tool result missing")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"已创建 production 分组，ID 为 1。"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	model, err := provider.New(context.Background(), provider.Config{Provider: "newapi", BaseURL: server.URL, APIKey: "test", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	orchestrator := aicore.Orchestrator{Registry: registry, Model: model, Authorize: aiToolPolicy(1), MaxRounds: 3}
	result := orchestrator.Run(context.Background(), aicore.RunRequest{Question: "创建 production 数据源分组，包含数据源 ID 7"})
	var count int64
	database.DB.Model(&models.DatasourceGroupMember{}).Where("datasource_id = 7").Count(&count)
	if rounds != 2 || count != 1 || !strings.Contains(result.Answer, "已创建") {
		t.Fatalf("workflow failed: %+v rounds=%d count=%d", result, rounds, count)
	}
}

func TestPlatformMCPLegacyQueryProjection(t *testing.T) {
	db := platformToolDB(t)
	if err := db.AutoMigrate(&models.AutomationTask{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.AutomationTask{Name: "safe-task", Command: "hidden-secret", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	c.Set("user_id", uint(1))
	mcpCallTool(c, mcpRequest{ID: json.RawMessage(`1`), Params: json.RawMessage(`{"name":"query_automation_tasks","arguments":{}}`)})
	body := recorder.Body.String()
	if !strings.Contains(body, "safe-task") || strings.Contains(body, "hidden-secret") || strings.Contains(body, `"isError":true`) {
		t.Fatalf("remote query projection failed: %s", body)
	}
	projected, err := projectMCPQueryData("query_alerts", map[string]interface{}{"list": []interface{}{map[string]interface{}{"id": 1, "started_at": "2026-10-22T00:00:00Z", "last_result": "hidden-secret"}}})
	raw, _ := json.Marshal(projected)
	if err != nil || !strings.Contains(string(raw), "started_at") || strings.Contains(string(raw), "hidden-secret") {
		t.Fatalf("incorrect alert projection: %s %v", raw, err)
	}
}

func TestRetiredCertificateToolsAreUnavailable(t *testing.T) {
	for _, name := range []string{"query_certificates", "query_certificate_domains"} {
		if _, ok := findMCPTool(name); ok {
			t.Fatalf("retired tool still registered: %s", name)
		}
		if _, err := executePlatformMCPTool(context.Background(), 1, name, nil); err == nil {
			t.Fatalf("retired tool executed: %s", name)
		}
	}
}
