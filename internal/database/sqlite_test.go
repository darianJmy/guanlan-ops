package database

import (
	"context"
	"errors"
	"guanlan/internal/config"
	"guanlan/internal/models"
	sqldb "guanlan/pkg/database"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func freshSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousConfig := DB, config.AppConfig
	config.AppConfig.DBDSN = filepath.Join(t.TempDir(), "nested", "aiops.db")
	t.Cleanup(func() { DB, config.AppConfig = previousDB, previousConfig })
	db, cleanup, err := New(config.AppConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	return db
}

func TestSQLiteInitializationAndReopen(t *testing.T) {
	db := freshSQLite(t)
	var mcpMenus int64
	if err := db.Model(&models.Menu{}).Where("path = ? OR component = ?", "/settings/mcp", "system/MCPServiceManage").Count(&mcpMenus).Error; err != nil {
		t.Fatal(err)
	}
	if mcpMenus != 0 {
		t.Fatal("fresh database must not create the retired MCP service page")
	}
	var user models.User
	if err := db.Preload("Roles.Permissions").Preload("Roles.Menus").Where("username = ?", "admin").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte("admin123")); err != nil {
		t.Fatal(err)
	}
	if len(user.Roles) == 0 || len(user.Roles[0].Permissions) == 0 || len(user.Roles[0].Menus) == 0 {
		t.Fatal("administrator associations were not initialized")
	}
	reopened, cleanup, err := New(config.AppConfig)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, setting := range []struct {
		query string
		want  int
	}{
		{"PRAGMA foreign_keys", 1}, {"PRAGMA busy_timeout", 5000},
	} {
		var got int
		if err := reopened.Raw(setting.query).Scan(&got).Error; err != nil || got != setting.want {
			t.Fatalf("%s = %d, err %v", setting.query, got, err)
		}
	}
	var mode string
	if err := reopened.Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil || mode != "wal" {
		t.Fatalf("journal mode = %q, err %v", mode, err)
	}
	orphan := models.InspectionItem{GroupID: 999999, Name: "orphan", Type: "http", Target: "example"}
	if err := reopened.Create(&orphan).Error; err == nil {
		t.Fatal("foreign key must reject missing group")
	}
	// Connection settings must also apply after database/sql reopens a connection.
	pool, _ := reopened.DB()
	pool.SetMaxIdleConns(0)
	var enabled int
	if err := reopened.Raw("PRAGMA foreign_keys").Scan(&enabled).Error; err != nil || enabled != 1 {
		t.Fatalf("foreign keys lost on reconnect: %d %v", enabled, err)
	}
}

func TestSQLiteRuntimeInitializesOnce(t *testing.T) {
	previous := DB
	t.Cleanup(func() { DB = previous })
	cfg := config.Config{DBDSN: filepath.Join(t.TempDir(), "data", "aiops.db")}
	db, cleanup, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var user models.User
	if err := db.Preload("Roles.Permissions").Preload("Roles.Menus").Where("username = ?", "admin").First(&user).Error; err != nil {
		cleanup()
		t.Fatal(err)
	}
	if len(user.Roles) != 1 || len(user.Roles[0].Permissions) == 0 || len(user.Roles[0].Menus) == 0 {
		cleanup()
		t.Fatal("initial administrator associations missing")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte("admin123")); err != nil {
		cleanup()
		t.Fatal(err)
	}
	if err := db.Model(&user).Updates(map[string]interface{}{"nickname": "custom", "password": "changed-password"}).Error; err != nil {
		cleanup()
		t.Fatal(err)
	}
	if err := db.Model(&models.Menu{}).Where("id = ?", 1).Update("sort", 1234).Error; err != nil {
		cleanup()
		t.Fatal(err)
	}
	var counts = make(map[string]int64)
	for _, table := range []string{"users", "roles", "permissions", "menus", "schema_migrations", "inspection_rule_groups"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			cleanup()
			t.Fatal(err)
		}
		counts[table] = count
	}
	if counts["inspection_rule_groups"] != 0 {
		t.Fatal("fresh database created a default probe rule group")
	}
	cleanup()
	db, cleanup, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	var restored models.User
	if err := db.First(&restored, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if restored.Nickname != "custom" || restored.Password != "changed-password" {
		t.Fatal("restart reset administrator data")
	}
	var menu models.Menu
	if err := db.First(&menu, 1).Error; err != nil || menu.Sort != 1234 {
		t.Fatalf("restart reset menu: %v %+v", err, menu)
	}
	for table, before := range counts {
		var after int64
		if err := db.Table(table).Count(&after).Error; err != nil || after != before {
			t.Fatalf("restart changed %s count: %d -> %d, %v", table, before, after, err)
		}
	}
}

func TestSQLiteInitializationRollsBackAndRetries(t *testing.T) {
	db, err := openDatabaseDSN(filepath.Join(t.TempDir(), "failed.db"))
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	failure := errors.New("injected menu seed failure")
	if err := db.Callback().Create().Before("gorm:create").Register("test:fail_seed", func(tx *gorm.DB) {
		if tx.Statement.Table == "menus" {
			tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := prepareDatabase(db); !errors.Is(err, failure) {
		t.Fatalf("expected seed failure, got %v", err)
	}
	var tables int64
	if err := db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'").Scan(&tables).Error; err != nil || tables != 0 {
		t.Fatalf("partial initialization remains: %d %v", tables, err)
	}
	if err := db.Callback().Create().Remove("test:fail_seed"); err != nil {
		t.Fatal(err)
	}
	if err := prepareDatabase(db); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if err := sqldb.RequireSchemaVersion(db, CurrentSchemaVersion); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteConcurrentFirstStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.db")
	var connections []*gorm.DB
	for i := 0; i < 2; i++ {
		db, err := openDatabaseDSN(path)
		if err != nil {
			t.Fatal(err)
		}
		pool, _ := db.DB()
		t.Cleanup(func() { _ = pool.Close() })
		connections = append(connections, db)
	}
	results := make(chan error, len(connections))
	var wg sync.WaitGroup
	for _, db := range connections {
		wg.Go(func() { results <- prepareDatabase(db) })
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	var users int64
	if err := connections[0].Model(&models.User{}).Count(&users).Error; err != nil || users != 1 {
		t.Fatalf("duplicate/missing admin: %d %v", users, err)
	}
}

func TestSQLiteRuntimePreservesUnrecognizedDatabase(t *testing.T) {
	cfg := config.Config{DBDSN: filepath.Join(t.TempDir(), "existing.db")}
	db, err := openDatabaseDSN(cfg.DBDSN)
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	if err := db.Exec("CREATE TABLE custom_data (value TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO custom_data VALUES ('keep')").Error; err != nil {
		t.Fatal(err)
	}
	previous := DB
	_, cleanup, err := New(cfg)
	if err == nil || !strings.Contains(err.Error(), "no schema version") || cleanup != nil {
		t.Fatalf("unexpected result: %v", err)
	}
	if DB != previous {
		t.Fatal("failed initialization published DB")
	}
	var value string
	if err := db.Raw("SELECT value FROM custom_data").Scan(&value).Error; err != nil || value != "keep" {
		t.Fatalf("existing data changed: %s %v", value, err)
	}
}

func TestSQLiteURIAndMemory(t *testing.T) {
	for _, dsn := range []string{":memory:", "file::memory:?cache=shared", "file:" + filepath.ToSlash(filepath.Join(t.TempDir(), "nested", "uri.db"))} {
		t.Run(dsn, func(t *testing.T) {
			db, err := openDatabaseDSN(dsn)
			if err != nil {
				t.Fatal(err)
			}
			pool, _ := db.DB()
			defer pool.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := db.WithContext(ctx).AutoMigrate(&testSchemaVersion{}); err != nil {
				t.Fatal(err)
			}
			if err := db.WithContext(ctx).Create(&testSchemaVersion{Version: "test"}).Error; err != nil {
				t.Fatal(err)
			}
		})
	}
}

type testSchemaVersion struct {
	Version string `gorm:"primaryKey"`
}
