package version

import (
	"github.com/glebarez/sqlite"
	adminmodels "go-admin/app/admin/models"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
)

func TestNetworkDiagnosticsMenuIsIdempotentAndGrantsBothModes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "diagnostics.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&adminmodels.SysMenu{}, &adminmodels.SysApi{}, &adminmodels.SysRole{}, &adminmodels.SysAppCasbinGrant{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE casbin_rule (id INTEGER PRIMARY KEY AUTOINCREMENT,ptype TEXT,v0 TEXT,v1 TEXT,v2 TEXT,v3 TEXT,v4 TEXT,v5 TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&adminmodels.SysRole{RoleName: "Admin", RoleKey: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := seedNetworkDiagnostics(db); err != nil {
			t.Fatal(err)
		}
	}
	var page adminmodels.SysMenu
	if err := db.Preload("SysApi").Where("menu_name = ?", "NetworkDiagnostics").First(&page).Error; err != nil {
		t.Fatal(err)
	}
	if page.Path != "/network/diagnostics" || page.Component != "/network/diagnostics/index" {
		t.Fatal(page)
	}
	var count int64
	db.Model(&adminmodels.SysApi{}).Where("path LIKE ?", "/api/v1/network-diagnostics%").Count(&count)
	if count != 4 {
		t.Fatalf("APIs = %d", count)
	}
	db.Table("casbin_rule").Where("v0 = ? AND v1 LIKE ?", "admin", "/api/v1/network-diagnostics%").Count(&count)
	if count != 4 {
		t.Fatalf("grants = %d", count)
	}
}
