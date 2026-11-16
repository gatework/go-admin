package version

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	admin "github.com/gatework/motor/app/admin/models"
	models "github.com/gatework/motor/app/net/models/firewall"
	firewall "github.com/gatework/motor/app/net/router"
)

func TestFirewallMigrationCreatesOfflineCapabilitiesIdempotently(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "firewall.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.AutoMigrate(&admin.SysMenu{}, &admin.SysAPI{}, &admin.SysRole{}, &admin.SysAppCasbinGrant{}); e != nil {
		t.Fatal(e)
	}
	createCasbinRule(t, db)
	if e = db.Create(&admin.SysRole{RoleName: "Admin", RoleKey: "admin"}).Error; e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e = seedNetworkFirewall(db); e != nil {
			t.Fatal(e)
		}
	}
	for _, m := range models.Schema() {
		if !db.Migrator().HasTable(m) {
			t.Fatalf("missing %T", m)
		}
	}
	for _, route := range firewall.FirewallRoutes {
		var count int64
		if e = db.Model(&admin.SysAPI{}).Where("path = ? AND action = ?", "/api/v1/network-firewall"+route.Path, route.Method).Count(&count).Error; e != nil || count != 1 {
			t.Fatalf("route not seeded once: %+v count=%d %v", route, count, e)
		}
	}
	var menu admin.SysMenu
	if e = db.Where("permission = ?", "network:firewall:list").Preload("SysAPI").First(&menu).Error; e != nil {
		t.Fatal(e)
	}
	for _, a := range menu.SysAPI {
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
