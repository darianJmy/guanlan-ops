package handlers

import (
	"context"
	"encoding/json"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	aicore "guanlan/internal/ai"
	"guanlan/internal/ai/provider"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAssistantMCPPermissionsAndProjection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	defer conn.Close()
	old := database.DB
	database.DB = db
	defer func() { database.DB = old }()
	if err := db.AutoMigrate(&models.User{}, &models.Role{}, &models.Permission{}, &models.AutomationTask{}); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"INSERT INTO users (id,username,password,status) VALUES (1,'tester','unused',1)",
		"INSERT INTO roles (id,name,code,status) VALUES (1,'reader','reader',1)",
		"INSERT INTO permissions (id,name,code,status) VALUES (1,'read','automation-task:list',1)",
		"INSERT INTO user_roles (user_id,role_id) VALUES (1,1)",
		"INSERT INTO role_permissions (role_id,permission_id) VALUES (1,1)",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.AutomationTask{Name: "demo", Command: "secret-command", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	registry := aicore.NewRegistry()
	registerAssistantMCPTools(context.Background(), registry, 1)
	if len(registry.Specs()) != 5 {
		t.Fatalf("wrong tools: %+v", registry.Specs())
	}
	if _, ok := registry.Get("run_automation_task"); ok {
		t.Fatal("write tool exposed")
	}
	tool, _ := registry.Get("query_automation_tasks")
	if err := aiToolPolicy(1)(context.Background(), tool.Spec, aicore.ToolCall{}); err != nil {
		t.Fatal(err)
	}
	result := tool.Handler(context.Background(), aicore.ToolCall{Tool: tool.Spec.Name, Params: map[string]interface{}{}})
	raw, _ := json.Marshal(result)
	if !result.OK || !strings.Contains(string(raw), "demo") || strings.Contains(string(raw), "secret-command") {
		t.Fatalf("invalid result: %s", raw)
	}

	oldConfig := config.AppConfig
	defer func() { config.AppConfig = oldConfig }()
	rounds := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rounds++
		var request struct {
			Messages []mockChatMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}

		if rounds == 1 {
			json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": map[string]interface{}{"role": "assistant", "tool_calls": []interface{}{map[string]interface{}{"id": "call_1", "type": "function", "function": map[string]interface{}{"name": "query_automation_tasks", "arguments": "{}"}}}}, "finish_reason": "tool_calls"}}})
			return
		}
		data, _ := json.Marshal(request)
		if !strings.Contains(string(data), "demo") || strings.Contains(string(data), "secret-command") {
			t.Error("tool result not safely delivered")
		}
		if request.Messages[len(request.Messages)-1].Role != "tool" {
			t.Error("native tool response missing")
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"choices": []interface{}{map[string]interface{}{"message": mockChatMessage{Role: "assistant", Content: "找到任务 demo"}, "finish_reason": "stop"}}})

	}))
	defer server.Close()
	chatModel, err := provider.New(context.Background(), provider.Config{Provider: "newapi", BaseURL: server.URL, APIKey: "test-key", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	orchestrator := aicore.Orchestrator{Registry: registry, Model: chatModel, Authorize: aiToolPolicy(1), MaxRounds: 3}
	response := orchestrator.Run(context.Background(), aicore.RunRequest{Question: "有哪些自动化任务？"})
	if response.Answer != "找到任务 demo" || rounds != 2 {
		t.Fatalf("round trip failed: %+v, rounds=%d", response, rounds)
	}
	db.Exec("UPDATE users SET status=0 WHERE id=1")
	if err := aiToolPolicy(1)(context.Background(), tool.Spec, aicore.ToolCall{}); err == nil {
		t.Fatal("permission revocation ignored")
	}
}

type mockChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
