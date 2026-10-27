package database

import (
	"guanlan/internal/config"
	"guanlan/internal/models"
	"path/filepath"
	"testing"
)

func TestProbeMenuUnderAlertCenter(t *testing.T) {
	previous := DB
	t.Cleanup(func() { DB = previous })
	cfg := config.Config{DBDSN: filepath.Join(t.TempDir(), "menu.db")}
	db, cleanup, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	var probe, parent models.Menu
	if err := db.Where("path = ?", "/inspection/items").First(&probe).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("path = ?", "/alert-center").First(&parent).Error; err != nil {
		t.Fatal(err)
	}
	if probe.ParentID != parent.ID || probe.Sort != 3 {
		t.Fatalf("fresh menu has wrong parent/order: %+v", probe)
	}
	var dashboards models.Menu
	if err := db.Where("path = ?", "/dashboards").First(&dashboards).Error; err != nil {
		t.Fatal(err)
	}
	if dashboards.Type != 2 || dashboards.Component != "monitor/DashboardHub" || dashboards.Permission != "dashboard:list" {
		t.Fatalf("expected a unified dashboard entry: %+v", dashboards)
	}
	var dashboardChildren int64
	if err := db.Model(&models.Menu{}).Where("parent_id = ?", dashboards.ID).Count(&dashboardChildren).Error; err != nil {
		t.Fatal(err)
	}
	if dashboardChildren != 0 {
		t.Fatal("dashboard provider entries should be inside the unified page")
	}
	if db.Migrator().HasTable("scheduled_tasks") || db.Migrator().HasTable("scheduled_task_executions") {
		t.Fatal("fresh database created retired scheduler tables")
	}
	// Reopening a database must not rewrite a user's menu configuration.
	if err := db.Model(&probe).Updates(map[string]interface{}{"parent_id": 0, "sort": 42}).Error; err != nil {
		t.Fatal(err)
	}
	if err := prepareDatabase(db); err != nil {
		t.Fatal(err)
	}
	var reopened models.Menu
	if err := db.First(&reopened, probe.ID).Error; err != nil {
		t.Fatal(err)
	}
	if reopened.ParentID != 0 || reopened.Sort != 42 {
		t.Fatal("opening an existing database rewrote its menus")
	}
}
