package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"guanlan/internal/auth"
	"guanlan/internal/config"
	"guanlan/internal/database"
	"guanlan/internal/models"
)

func TestAutomationConsumersCannotManageTasks(t *testing.T) {
	previous := database.DB
	db, cleanup, err := database.New(config.Config{DBDSN: filepath.Join(t.TempDir(), "permissions.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.DB = previous; cleanup() })
	permission := models.Permission{Name: "拨测编辑", Code: "inspection:edit", Status: 1}
	if err := db.Where("code = ?", permission.Code).FirstOrCreate(&permission).Error; err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "probe-consumer", Password: "unused", Status: 1, Roles: []models.Role{{Name: "probe caller", Code: "probe-caller", Status: 1, Permissions: []models.Permission{permission}}}}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	task := models.AutomationTask{Name: "callable task", ScriptID: 1, Command: "private-command", NodeType: "ssh", NodeID: 7, Status: 1}
	db.Create(&task)
	token, err := auth.GenerateToken(user.ID, user.Username)
	if err != nil {
		t.Fatal(err)
	}
	router := SetupRouter()
	call := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	result := call(http.MethodGet, "/api/automation-tasks/options")
	if result.Code != 200 {
		t.Fatalf("caller cannot select task: %s", result.Body.String())
	}
	var body struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &body); err != nil || len(body.Data) == 0 {
		t.Fatalf("invalid options: %s", result.Body.String())
	}
	for _, option := range body.Data {
		if len(option) != 3 || option["id"] == nil || option["name"] == nil || option["status"] == nil {
			t.Fatalf("execution settings exposed: %#v", option)
		}
	}
	for _, tc := range []struct{ method, path string }{{"GET", "/api/automation-tasks"}, {"POST", "/api/automation-tasks"}, {"PUT", "/api/automation-tasks/1"}, {"DELETE", "/api/automation-tasks/1"}, {"POST", "/api/automation-tasks/1/run"}} {
		if got := call(tc.method, tc.path); got.Code != http.StatusForbidden {
			t.Fatalf("consumer can manage %s %s: %d", tc.method, tc.path, got.Code)
		}
	}
}
