package database

import (
	"guanlan/internal/models"
	sqldb "guanlan/pkg/database"
	"path/filepath"
	"testing"
)

func TestDatasourceGroupTreeMigration(t *testing.T) {
	db, err := openDatabaseDSN(filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	statements := []string{
		"CREATE TABLE datasource_groups (id integer PRIMARY KEY AUTOINCREMENT, name text NOT NULL, description text, created_at datetime, updated_at datetime)",
		"CREATE UNIQUE INDEX idx_datasource_groups_name ON datasource_groups(name)",
		"INSERT INTO datasource_groups(id,name,description) VALUES(7,'existing','keep')",
		"CREATE TABLE datasource_group_members (id integer PRIMARY KEY, group_id integer, datasource_id integer)",
		"INSERT INTO datasource_group_members VALUES(1,7,42)",
		"CREATE TABLE alert_rule_datasource_groups (id integer PRIMARY KEY, rule_id integer, datasource_group_id integer)",
		"INSERT INTO alert_rule_datasource_groups VALUES(1,9,7)",
	}
	for _, sql := range statements {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := sqldb.RecordSchemaVersion(db, "2026102201"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := prepareDatabase(db); err != nil {
			t.Fatal(err)
		}
	}
	var group models.DatasourceGroup
	if err := db.First(&group, 7).Error; err != nil || group.ParentID != 0 || group.Name != "existing" || group.Description != "keep" {
		t.Fatalf("lost group: %+v %v", group, err)
	}
	var member models.DatasourceGroupMember
	db.First(&member)
	var binding models.AlertRuleDatasourceGroup
	db.First(&binding)
	if member.GroupID != 7 || member.DatasourceID != 42 || binding.RuleID != 9 || binding.DatasourceGroupID != 7 {
		t.Fatal("lost membership or rule selection")
	}
	if err := db.Create(&models.DatasourceGroup{Name: "existing", ParentID: 7}).Error; err != nil {
		t.Fatal("same name under another parent:", err)
	}
	if err := db.Create(&models.DatasourceGroup{Name: "existing", ParentID: 7}).Error; err == nil {
		t.Fatal("duplicate siblings accepted")
	}
	if err := sqldb.RequireSchemaVersion(db, CurrentSchemaVersion); err != nil {
		t.Fatal(err)
	}
}
