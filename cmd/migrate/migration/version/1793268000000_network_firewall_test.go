package version

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	admin "go-admin/app/admin/models"
	"go-admin/app/firewall"
	"go-admin/app/firewall/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestFirewallMigrationCreatesOfflineCapabilitiesIdempotently(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "firewall.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.AutoMigrate(&admin.SysMenu{}, &admin.SysApi{}, &admin.SysRole{}, &admin.SysAppCasbinGrant{}); e != nil {
		t.Fatal(e)
	}
	if e = db.Exec(`CREATE TABLE casbin_rule (id INTEGER PRIMARY KEY AUTOINCREMENT, ptype TEXT,v0 TEXT,v1 TEXT,v2 TEXT,v3 TEXT,v4 TEXT,v5 TEXT)`).Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Create(&admin.SysRole{RoleName: "Admin", RoleKey: "admin"}).Error; e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e = seedNetworkFirewall(db); e != nil {
			t.Fatal(e)
		}
	}
	for _, m := range models.Schema() {
		if !db.Migrator().HasTable(m) {
			t.Fatalf("missing %T", m)
		}
	}
	for _, route := range firewall.Routes {
		var count int64
		if e = db.Model(&admin.SysApi{}).Where("path = ? AND action = ?", "/api/v1/network-firewall"+route.Path, route.Method).Count(&count).Error; e != nil || count != 1 {
			t.Fatalf("route not seeded once: %+v count=%d %v", route, count, e)
		}
	}
	var menu admin.SysMenu
	if e = db.Where("permission = ?", "network:firewall:list").Preload("SysApi").First(&menu).Error; e != nil {
		t.Fatal(e)
	}
	for _, a := range menu.SysApi {
		if a.Action != "GET" {
			t.Fatalf("list capability grants mutation: %+v", a)
		}
	}
	for _, name := range []string{"NetworkFirewallObjects", "NetworkFirewallQuery", "NetworkFirewallDesign"} {
		var count int64
		db.Model(&admin.SysMenu{}).Where("menu_name = ?", name).Count(&count)
		if count != 1 {
			t.Fatalf("menu name mismatch %s: %d", name, count)
		}
	}
	var count int64
	db.Model(&models.Domain{}).Where("code = ?", "default").Count(&count)
	if count != 1 {
		t.Fatal("default domain duplicated", count)
	}
}
