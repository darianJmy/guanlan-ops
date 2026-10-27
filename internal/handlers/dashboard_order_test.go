package handlers

import (
	"encoding/json"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDashboardGlobalOrderPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboards.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	previous := database.DB
	database.DB = db
	t.Cleanup(func() { sqlDB, _ := database.DB.DB(); _ = sqlDB.Close(); database.DB = previous })
	if err := db.AutoMigrate(&models.Dashboard{}, &models.SLSDashboardProject{}, &models.CloudMonitorWorkspace{}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []interface{}{
		&models.Dashboard{Name: "grafana", Provider: "grafana", URL: "https://example.test"},
		&models.SLSDashboardProject{Name: "logs", Provider: "aliyun_sls", Project: "logs", AccessKeySecretCipher: "cipher-secret"},
		&models.CloudMonitorWorkspace{Name: "cloud", Workspace: "metrics", Region: "cn-hangzhou", AccessKeySecretCipher: "cipher-secret"},
	} {
		if err := createDashboardAtFront(item); err != nil {
			t.Fatal(err)
		}
	}
	r := gin.New()
	r.GET("/cards", ListDashboardCards)
	r.PUT("/order", ReorderDashboardCards)
	call := func(method, path, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, res.Code, res.Body.String())
		}
		return res
	}
	order := func() []dashboardCard {
		t.Helper()
		res := call(http.MethodGet, "/cards", "", 200)
		if strings.Contains(res.Body.String(), "cipher-secret") {
			t.Fatal("card list exposed credentials")
		}
		var payload struct {
			Data []dashboardCard `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		return payload.Data
	}
	cards := order()
	if len(cards) != 3 || cards[0].Kind != "cloud" || cards[2].Kind != "grafana" {
		t.Fatalf("new cards must lead: %+v", cards)
	}
	desired := `{"items":[{"kind":"grafana","id":1},{"kind":"cloud","id":1},{"kind":"logs","id":1}]}`
	call(http.MethodPut, "/order", desired, 200)
	for index, card := range order() {
		if card.Sort != index {
			t.Fatalf("numeric positions not persisted: %+v", card)
		}
	}
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	db, err = gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	database.DB = db
	cards = order()
	if cards[0].Kind != "grafana" || cards[1].Kind != "cloud" || cards[2].Kind != "logs" {
		t.Fatalf("reopen lost shared order: %+v", cards)
	}
	call(http.MethodPut, "/order", `{"items":[{"kind":"grafana","id":1},{"kind":"grafana","id":1}]}`, 400)
	call(http.MethodPut, "/order", `{"items":[{"kind":"unknown","id":1}]}`, 400)
	call(http.MethodPut, "/order", `{"items":[{"kind":"grafana","id":1}]}`, 409)
	// A later creation gets the first global numeric position, preserving the layout behind it.
	added := models.CloudMonitorWorkspace{Name: "new", Workspace: "new-metrics", Region: "cn-hangzhou"}
	if err := createDashboardAtFront(&added); err != nil {
		t.Fatal(err)
	}
	call(http.MethodPut, "/order", desired, 409)
	cards = order()
	if cards[0].ID != added.ID || cards[0].Kind != "cloud" || cards[1].Kind != "grafana" {
		t.Fatalf("new item disturbed existing order: %+v", cards)
	}
	// Any write failure rolls back the entire mixed-provider order.
	if err := db.Exec(`CREATE TRIGGER reject_log_order BEFORE UPDATE OF sort ON sls_dashboard_projects BEGIN SELECT RAISE(ABORT, 'test failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	call(http.MethodPut, "/order", `{"items":[{"kind":"grafana","id":1},{"kind":"logs","id":1},{"kind":"cloud","id":1},{"kind":"cloud","id":2}]}`, 500)
	after := order()
	for i := range cards {
		if cards[i].Kind != after[i].Kind || cards[i].ID != after[i].ID || cards[i].Sort != after[i].Sort {
			t.Fatal("failed reorder partially committed")
		}
	}
}
