package version

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	adminmodels "github.com/gatework/motor/app/admin/models"
)

func TestNetworkDiagnosticsMenuIsIdempotentAndGrantsBothModes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "diagnostics.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&adminmodels.SysMenu{}, &adminmodels.SysAPI{}, &adminmodels.SysRole{}, &adminmodels.SysAppCasbinGrant{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE casbin_rule (id INTEGER PRIMARY KEY AUTOINCREMENT,ptype TEXT,v0 TEXT,v1 TEXT,v2 TEXT,v3 TEXT,v4 TEXT,v5 TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&adminmodels.SysRole{RoleName: "Admin", RoleKey: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := seedNetworkDiagnostics(db); err != nil {
			t.Fatal(err)
		}
	}
	var page adminmodels.SysMenu
	if err := db.Preload("SysAPI").Where("menu_name = ?", "NetworkDiagnostics").First(&page).Error; err != nil {
		t.Fatal(err)
	}
	if page.Path != "/network/diagnostics" || page.Component != "/network/diagnostics/index" {
		t.Fatal(page)
	}
	var count int64
	db.Model(&adminmodels.SysAPI{}).Where("path LIKE ?", "/api/v1/network-diagnostics%").Count(&count)
	if count != 4 {
		t.Fatalf("APIs = %d", count)
	}
	db.Table("casbin_rule").Where("v0 = ? AND v1 LIKE ?", "admin", "/api/v1/network-diagnostics%").Count(&count)
	if count != 4 {
		t.Fatalf("grants = %d", count)
	}
}
