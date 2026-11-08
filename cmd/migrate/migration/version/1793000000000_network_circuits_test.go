package version

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	adminmodels "go-admin/app/admin/models"
	common "go-admin/common/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNetworkCircuitMigrationSeedsScopedActionsAndBackfillsRoleDependencies(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "circuits.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&adminmodels.SysMenu{}, &adminmodels.SysApi{}, &adminmodels.SysRole{}, &adminmodels.SysAppCasbinGrant{}, &common.Migration{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TABLE casbin_rule (id INTEGER PRIMARY KEY AUTOINCREMENT, ptype TEXT, v0 TEXT, v1 TEXT, v2 TEXT, v3 TEXT, v4 TEXT, v5 TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	role := adminmodels.SysRole{RoleName: "Administrator", RoleKey: "admin"}
	if err = db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err = seedNetworkInventory(db); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = seedNetworkCircuits(db); err != nil {
			t.Fatal(err)
		}
	}
	if !db.Migrator().HasTable("sys_network_circuit") {
		t.Fatal("missing circuit table")
	}
	var menus []adminmodels.SysMenu
	if err = db.Preload("SysApi").Where("app_code = ? AND seed_code LIKE ?", "network", "circuit-%").Find(&menus).Error; err != nil {
		t.Fatal(err)
	}
	byCode := map[string]adminmodels.SysMenu{}
	for _, m := range menus {
		if m.SeedCode != nil {
			byCode[*m.SeedCode] = m
		}
	}
	page := byCode["circuit-manage"]
	if page.MenuName != "NetworkCircuitManage" || page.Path != "/network/circuit" || page.Component != "/network/circuit/index" || len(menus) != 8 {
		t.Fatalf("incorrect menu: %+v count=%d", page, len(menus))
	}
	for _, operation := range []string{"list", "query", "add", "edit", "remove", "export", "traffic"} {
		m := byCode["circuit-"+operation]
		if m.Permission != "network:circuit:"+operation || m.MenuType != "F" || m.ParentId != page.MenuId {
			t.Fatalf("missing action: %+v", m)
		}
	}
	for _, action := range []string{"add", "edit"} {
		m := byCode["circuit-"+action]
		apis := map[string]bool{}
		for _, a := range m.SysApi {
			apis[a.Action+" "+a.Path] = true
		}
		for _, path := range []string{"/api/v1/network-circuit/zabbix/hosts", "/api/v1/network-circuit/zabbix/items", "/api/v1/network-circuit/options", "/api/v1/network-device"} {
			if !apis["GET "+path] {
				t.Errorf("%s missing %s", action, path)
			}
		}
		if action == "edit" && !apis["GET /api/v1/network-circuit/:id"] {
			t.Error("editor cannot load current version")
		}
	}
	editor := adminmodels.SysRole{RoleName: "Circuit editor", RoleKey: "circuit-editor"}
	if err = db.Create(&editor).Error; err != nil {
		t.Fatal(err)
	}
	edit := byCode["circuit-edit"]
	if err = db.Exec("INSERT INTO sys_role_menu(role_id,menu_id) VALUES (?,?)", editor.RoleId, edit.MenuId).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = seedNetworkCircuits(db); err != nil {
			t.Fatal(err)
		}
	}
	for _, endpoint := range edit.SysApi {
		var count int64
		if err = db.Table("casbin_rule").Where("v0 = ? AND v1 = ? AND v2 = ?", editor.RoleKey, endpoint.Path, endpoint.Action).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("dependency missing/duplicated: %s %s %d %v", endpoint.Action, endpoint.Path, count, err)
		}
	}
	var leaked int64
	db.Table("casbin_rule").Where("v0 = ? AND v1 IN ?", editor.RoleKey, []string{"/api/v1/network-circuit/export", "/api/v1/network-circuit/:id/traffic"}).Count(&leaked)
	if leaked != 0 {
		t.Fatal("edit grants export or traffic")
	}
	var column struct {
		NotNull int `gorm:"column:notnull"`
		Type    string
	}
	if err = db.Raw(`SELECT "notnull",type FROM pragma_table_info('sys_network_circuit') WHERE name = 'deleted_at'`).Scan(&column).Error; err != nil || column.NotNull != 1 || column.Type != "INTEGER" {
		t.Fatalf("wrong marker: %+v %v", column, err)
	}
}
