package handlers

import (
	"encoding/json"
	"guanlan/internal/database"
	"guanlan/internal/models"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDashboardListsExposePreciseCreationTime(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	previous := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previous; _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.Dashboard{}, &models.SLSDashboardProject{}, &models.CloudMonitorWorkspace{}); err != nil {
		t.Fatal(err)
	}
	created := time.Date(2026, 10, 23, 9, 0, 0, 123456789, time.FixedZone("CST", 8*3600))
	for _, item := range []interface{}{
		&models.Dashboard{Name: "grafana", Provider: "grafana", URL: "https://example.test", CreatedAt: created},
		&models.SLSDashboardProject{Name: "logs", Provider: "aliyun_sls", Project: "logs", Region: "cn-hangzhou", CreatedAt: created},
		&models.CloudMonitorWorkspace{Name: "cloud", Workspace: "metrics", Region: "cn-hangzhou", CreatedAt: created},
	} {
		if err := db.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := gin.New()
	router.GET("/grafana", ListDashboards)
	router.GET("/logs", ListSLSDashboardProjects)
	router.GET("/cloud", ListCloudMonitorWorkspaces)
	for _, path := range []string{"/grafana", "/logs", "/cloud"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s: %s", path, response.Body.String())
		}
		var payload struct {
			Data []struct {
				CreatedAt string `json:"created_at"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Data) != 1 {
			t.Fatalf("%s: expected one dashboard", path)
		}
		parsed, err := time.Parse(time.RFC3339Nano, payload.Data[0].CreatedAt)
		if err != nil || !parsed.Equal(created) {
			t.Fatalf("%s: creation timestamp lost precision or zone: %q (%v)", path, payload.Data[0].CreatedAt, err)
		}
	}
}
