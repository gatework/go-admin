package version

import (
	"github.com/glebarez/sqlite"
	adminmodels "go-admin/app/admin/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"path/filepath"
	"testing"
)

func TestNetworkDeliverySeedsSchemaPermissionsAndDependencies(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "delivery.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	if e = db.AutoMigrate(&adminmodels.SysMenu{}, &adminmodels.SysApi{}, &adminmodels.SysRole{}, &adminmodels.SysAppCasbinGrant{}); e != nil {
		t.Fatal(e)
	}
	if e = db.Exec(`CREATE TABLE casbin_rule (id INTEGER PRIMARY KEY AUTOINCREMENT, ptype TEXT,v0 TEXT,v1 TEXT,v2 TEXT,v3 TEXT,v4 TEXT,v5 TEXT)`).Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Create(&adminmodels.SysRole{RoleName: "Admin", RoleKey: "admin"}).Error; e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2; i++ {
		if e = seedNetworkDelivery(db); e != nil {
			t.Fatal(e)
		}
	}
	for _, table := range []string{"sys_delivery_order", "sys_delivery_target", "sys_delivery_connection", "sys_delivery_receipt", "sys_delivery_device_lease", "sys_delivery_audit"} {
		if !db.Migrator().HasTable(table) {
			t.Fatal(table)
		}
	}
	var page adminmodels.SysMenu
	if e = db.Where("app_code = ? AND seed_code = ?", "network", "delivery-manage").First(&page).Error; e != nil {
		t.Fatal(e)
	}
	if page.MenuName != "NetworkDeliveryManage" {
		t.Fatal(page.MenuName)
	}
	for _, action := range []string{"list", "query", "add", "edit", "execute", "cancel", "retry", "connection"} {
		var m adminmodels.SysMenu
		if e = db.Preload("SysApi").Where("permission = ?", "network:delivery:"+action).First(&m).Error; e != nil {
			t.Fatal(action, e)
		}
		if len(m.SysApi) == 0 {
			t.Fatal(action)
		}
		if action == "edit" || action == "add" || action == "connection" {
			paths := map[string]bool{}
			for _, a := range m.SysApi {
				paths[a.Action+" "+a.Path] = true
			}
			if !paths["GET /api/v1/network-delivery/options"] || !paths["GET /api/v1/network-delivery/devices"] {
				t.Fatal("missing read dependencies", action)
			}
		}
	}
}
