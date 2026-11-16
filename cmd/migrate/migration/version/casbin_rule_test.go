package version

import (
	"testing"

	gormadapter "github.com/casbin/gorm-adapter/v3"
	"gorm.io/gorm"
)

// createCasbinRule creates casbin_rule as the host's Casbin adapter does at
// startup, before any migration runs; no migration creates it.
func createCasbinRule(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(&gormadapter.CasbinRule{}); err != nil {
		t.Fatalf("create casbin_rule: %v", err)
	}
}
