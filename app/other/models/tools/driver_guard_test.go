package tools

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"gorm.io/gorm"
)

func TestCodegenModelsRefuseUnsupportedOrMissingConnections(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	previous := config.DatabaseConfig.Driver
	config.DatabaseConfig.Driver = "mysql" // A global setting cannot enable another connection's dialect.
	t.Cleanup(func() { config.DatabaseConfig.Driver = previous })
	calls := map[string]func(*gorm.DB) error{
		"tables page":  func(db *gorm.DB) error { _, _, err := new(DBTables).GetPage(db, 10, 1); return err },
		"table":        func(db *gorm.DB) error { _, err := (&DBTables{TableName: "x"}).Get(db); return err },
		"columns page": func(db *gorm.DB) error { _, _, err := (&DBColumns{TableName: "x"}).GetPage(db, 10, 1); return err },
		"columns":      func(db *gorm.DB) error { _, err := (&DBColumns{TableName: "x"}).GetList(db); return err },
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(nil); err == nil || !strings.Contains(err.Error(), "connection") {
				t.Fatalf("missing connection: %v", err)
			}
			if err := call(db); err == nil || !strings.Contains(err.Error(), "unsupported driver") {
				t.Fatalf("unsupported driver: %v", err)
			}
		})
	}
}
