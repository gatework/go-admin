package version

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func configTable(t *testing.T, rows string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.Close() })
	for _, sql := range []string{"CREATE TABLE sys_config (id INTEGER PRIMARY KEY, config_key TEXT, deleted_at INTEGER NOT NULL DEFAULT 0)", rows} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestConfigKeyIndexRefusesActiveDuplicatesAndAllowsDeletedOnes(t *testing.T) {
	db := configTable(t, "INSERT INTO sys_config (id, config_key, deleted_at) VALUES (1, 'a', 0), (2, 'a', 0), (3, 'b', 0)")
	err := addConfigKeyIndex(db)
	if err == nil || !strings.Contains(err.Error(), "(a)") {
		t.Fatalf("duplicates not named: %v", err)
	}
	if db.Migrator().HasIndex("sys_config", configKeyIndex) {
		t.Fatal("index created over duplicates")
	}

	db = configTable(t, "INSERT INTO sys_config (id, config_key, deleted_at) VALUES (1, 'a', 0), (2, 'a', 1700000000000)")
	for range 2 {
		if err := addConfigKeyIndex(db); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec("INSERT INTO sys_config (id, config_key, deleted_at) VALUES (3, 'a', 0)").Error; err == nil {
		t.Fatal("second active row with the same key accepted")
	}
	if err := db.Exec("INSERT INTO sys_config (id, config_key, deleted_at) VALUES (4, 'a', 1800000000000)").Error; err != nil {
		t.Fatalf("a deleted row blocked by the index: %v", err)
	}
}
