package database

import (
	sqldb "guanlan/pkg/database"
	"path/filepath"
	"testing"
)

func TestPreviousSchemaRequiresRecreation(t *testing.T) {
	db, err := openDatabaseDSN(filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatal(err)
	}
	pool, _ := db.DB()
	defer pool.Close()
	if err := db.Exec("CREATE TABLE datasource_groups (id integer PRIMARY KEY, name text)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO datasource_groups VALUES (7, 'existing')").Error; err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"2026102201", "2026102601"} {
		if err := sqldb.RecordSchemaVersion(db, version); err != nil {
			t.Fatal(err)
		}
		if err := prepareDatabase(db); err == nil {
			t.Fatal("old development schema was silently accepted")
		}
		var name string
		if err := db.Raw("SELECT name FROM datasource_groups WHERE id = 7").Scan(&name).Error; err != nil || name != "existing" {
			t.Fatalf("existing data changed: %q %v", name, err)
		}
	}
}
