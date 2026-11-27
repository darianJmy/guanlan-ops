package database

import (
	"path/filepath"
	"testing"

	"guanlan/internal/models"
	sqldb "guanlan/pkg/database"
)

func TestServiceDashboardUpgradeFromPreviousSchema(t *testing.T) {
	db, err := openDatabaseDSN(filepath.Join(t.TempDir(), "previous.db"))
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	if err := db.AutoMigrate(&models.User{}, &models.Role{}, &models.Permission{}, &models.Menu{}, &models.Dashboard{}); err != nil {
		t.Fatal(err)
	}
	admin := models.Role{Name: "Administrator", Code: "admin", Status: 1}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	viewer := models.Role{Name: "Viewer", Code: "viewer", Status: 1}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "existing-admin", Password: "existing-fixture-value", Status: 1, Roles: []models.Role{admin}}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := sqldb.RecordSchemaVersion(db, previousSchemaVersion); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if err := prepareDatabase(db); err != nil {
			t.Fatal(err)
		}
	}
	if err := sqldb.RequireSchemaVersion(db, CurrentSchemaVersion); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable(&models.ServicePortalResource{}) {
		t.Fatal("upgraded database lacks service table")
	}
	var loaded models.User
	if err := db.Preload("Roles.Permissions").Preload("Roles.Menus").First(&loaded, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if user.Password != loaded.Password || len(loaded.Roles) != 1 || len(loaded.Roles[0].Permissions) != 2 || len(loaded.Roles[0].Menus) != 1 {
		t.Fatal("unexpected user, menu, or permissions after upgrade")
	}
	if count := db.Model(&viewer).Association("Permissions").Count(); count != 0 {
		t.Fatalf("viewer received %d service permissions", count)
	}
}

func TestServiceDashboardMenuMergePreservesServiceOnlyRoles(t *testing.T) {
	db, err := openDatabaseDSN(filepath.Join(t.TempDir(), "service-menu.db"))
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	if err := createCurrentSchema(db); err != nil {
		t.Fatal(err)
	}
	if err := seedInitialData(db); err != nil {
		t.Fatal(err)
	}

	legacy := models.Menu{Path: "/dashboards/services", Name: "Services", Component: "monitor/ServiceDashboardManage"}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	var permission models.Permission
	if err := db.Where("code = ?", "service-dashboard:list").First(&permission).Error; err != nil {
		t.Fatal(err)
	}
	viewer := models.Role{
		Name: "Services viewer", Code: "services-viewer",
		Menus: []models.Menu{legacy}, Permissions: []models.Permission{permission},
	}
	if err := db.Create(&viewer).Error; err != nil {
		t.Fatal(err)
	}
	service := models.ServicePortalResource{ID: "demo", Name: "Demo", Type: "http_direct", TargetURL: "https://app.example.test/"}
	if err := db.Create(&service).Error; err != nil {
		t.Fatal(err)
	}
	if err := sqldb.RecordSchemaVersion(db, serviceDashboardMenuVersion); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if err := prepareDatabase(db); err != nil {
			t.Fatal(err)
		}
	}
	var role models.Role
	if err := db.Preload("Menus").Preload("Permissions").First(&role, viewer.ID).Error; err != nil {
		t.Fatal(err)
	}
	if len(role.Menus) != 1 || role.Menus[0].Path != "/dashboards" {
		t.Fatalf("service viewer received unexpected menus: %+v", role.Menus)
	}
	if len(role.Permissions) != 1 || role.Permissions[0].Code != permission.Code {
		t.Fatalf("service viewer permissions changed: %+v", role.Permissions)
	}
	var legacyCount int64
	if err := db.Model(&models.Menu{}).Where("id = ?", legacy.ID).Count(&legacyCount).Error; err != nil || legacyCount != 0 {
		t.Fatalf("legacy menu still present: %d, %v", legacyCount, err)
	}
	if err := db.First(&models.ServicePortalResource{}, "id = ?", service.ID).Error; err != nil {
		t.Fatalf("service data did not survive upgrade: %v", err)
	}
}
