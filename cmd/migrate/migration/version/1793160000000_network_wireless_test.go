package version

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	admin "github.com/gatework/motor/app/admin/models"
	models "github.com/gatework/motor/app/net/models/wireless"
)

func TestWirelessMigrationIsIdempotentAndReadCapabilitiesCannotWrite(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "wireless.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.AutoMigrate(&admin.SysMenu{}, &admin.SysAPI{}, &admin.SysRole{}, &admin.SysAppCasbinGrant{}); e != nil {
		t.Fatal(e)
	}
	if e = db.Exec(`CREATE TABLE casbin_rule (id INTEGER PRIMARY KEY AUTOINCREMENT, ptype TEXT,v0 TEXT,v1 TEXT,v2 TEXT,v3 TEXT,v4 TEXT,v5 TEXT)`).Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Create(&admin.SysRole{RoleName: "Admin", RoleKey: "admin"}).Error; e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e = seedNetworkWireless(db); e != nil {
			t.Fatal(e)
		}
	}
	for _, m := range []any{&models.Resource{}, &models.Dimension{}, &models.Collection{}, &models.NeighborState{}, &models.Audit{}} {
		if !db.Migrator().HasTable(m) {
			t.Fatalf("missing %T", m)
		}
	}
	var menus []admin.SysMenu
	if e = db.Where("permission LIKE ?", "network:wireless:%").Preload("SysAPI").Find(&menus).Error; e != nil {
		t.Fatal(e)
	}
	if len(menus) != 6 {
		t.Fatal(len(menus))
	}
	for _, m := range menus {
		for _, a := range m.SysAPI {
			if (m.Permission == "network:wireless:list" || m.Permission == "network:wireless:query") && a.Action != "GET" {
				t.Fatal("read grants write", m.Permission, a.Path)
			}
		}
	}
	var menu admin.SysMenu
	db.Where("seed_code = ?", "wireless-manage").First(&menu)
	if menu.MenuName != "NetworkWirelessManage" {
		t.Fatal(menu.MenuName)
	}
}
