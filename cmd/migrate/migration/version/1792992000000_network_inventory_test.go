package version

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	adminmodels "github.com/gatework/motor/app/admin/models"
	common "github.com/gatework/motor/common/models"
)

func TestNetworkInventoryAdvancesLegacySequencesOnPostgres(t *testing.T) {
	db := postgresDB(t)
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	schema := fmt.Sprintf("network_inventory_sequences_%d", time.Now().UnixNano())
	if err = tx.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	if err = tx.Exec("SET LOCAL search_path TO " + schema).Error; err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"CREATE TABLE sys_api (id bigserial PRIMARY KEY)",
		"CREATE TABLE sys_menu (menu_id bigserial PRIMARY KEY)",
		"INSERT INTO sys_api(id) VALUES(9005)",
		"INSERT INTO sys_menu(menu_id) VALUES(9004)",
	} {
		if err = tx.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = advanceNetworkSeedSequences(tx); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err = tx.Raw("INSERT INTO sys_api DEFAULT VALUES RETURNING id").Scan(&id).Error; err != nil || id != 9006 {
		t.Fatalf("sys_api next id=%d err=%v", id, err)
	}
	if err = tx.Raw("INSERT INTO sys_menu DEFAULT VALUES RETURNING menu_id").Scan(&id).Error; err != nil || id != 9005 {
		t.Fatalf("sys_menu next id=%d err=%v", id, err)
	}
	if err = tx.Exec("SELECT setval(pg_get_serial_sequence('sys_api','id')::regclass, 9500, true)").Error; err != nil {
		t.Fatal(err)
	}
	if err = advanceNetworkSeedSequences(tx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Raw("INSERT INTO sys_api DEFAULT VALUES RETURNING id").Scan(&id).Error; err != nil || id != 9501 {
		t.Fatalf("sequence moved backwards: id=%d err=%v", id, err)
	}
}

func TestNetworkInventoryMigrationSeedsHierarchyAndScopedActions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "network.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&adminmodels.SysMenu{}, &adminmodels.SysAPI{}, &adminmodels.SysRole{}, &adminmodels.SysAppCasbinGrant{}, &common.Migration{}); err != nil {
		t.Fatal(err)
	}
	createCasbinRule(t, db)
	role := adminmodels.SysRole{RoleName: "Administrator", RoleKey: "admin"}
	if err = db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if err = seedNetworkInventory(db); err != nil {
			t.Fatal(err)
		}
	}
	for _, table := range []string{"sys_network_device", "sys_network_device_sync_state"} {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("missing table %s", table)
		}
	}
	var menus []adminmodels.SysMenu
	if err = db.Preload("SysAPI").Where("app_code = ?", "network").Find(&menus).Error; err != nil {
		t.Fatal(err)
	}
	byName := map[string]adminmodels.SysMenu{}
	for _, m := range menus {
		byName[m.MenuName] = m
	}
	if len(menus) != 7 {
		t.Fatalf("menus = %d, want 7", len(menus))
	}
	dir, page := byName["NetworkManagement"], byName["NetworkDeviceManage"]
	if dir.Title != "网络管理" || dir.MenuType != "M" || dir.Path != "/network" || page.ParentID != dir.MenuID || page.Title != "设备管理" || page.Component != "/network/device/index" || page.Path != "/network/device" {
		t.Fatalf("invalid menu hierarchy: dir=%+v page=%+v", dir, page)
	}
	pageAPIs := map[string]bool{}
	for _, a := range page.SysAPI {
		pageAPIs[a.Action+" "+a.Path] = true
	}
	if len(pageAPIs) != 3 || !pageAPIs["GET /api/v1/network-device"] || !pageAPIs["GET /api/v1/network-device/options"] || !pageAPIs["GET /api/v1/network-device/sync-status"] {
		t.Fatalf("page APIs = %v", pageAPIs)
	}
	for permission, endpoint := range map[string]string{
		"network:device:list":   "GET /api/v1/network-device",
		"network:device:query":  "GET /api/v1/network-device/:id",
		"network:device:edit":   "PUT /api/v1/network-device/:id",
		"network:device:sync":   "POST /api/v1/network-device/sync",
		"network:device:export": "GET /api/v1/network-device/export",
	} {
		found := false
		for _, m := range menus {
			if m.Permission != permission {
				continue
			}
			if m.MenuType != "F" || m.ParentID != page.MenuID || len(m.SysAPI) != 1 || m.SysAPI[0].Action+" "+m.SysAPI[0].Path != endpoint {
				t.Fatalf("incorrect action binding: %+v", m)
			}
			found = true
		}
		if !found {
			t.Errorf("missing permission %s", permission)
		}
	}
	for table, expected := range map[string]int64{"sys_api": 7, "sys_role_menu": 7, "casbin_rule": 7, "sys_app_casbin_grant": 7} {
		var count int64
		if err = db.Table(table).Count(&count).Error; err != nil || count != expected {
			t.Errorf("%s count=%d want=%d err=%v", table, count, expected, err)
		}
	}
	// New inventory tables are born with the integer soft-delete marker;
	// they must not depend on rerunning the historical conversion migration.
	var column struct {
		NotNull int `gorm:"column:notnull"`
		Type    string
	}
	if err = db.Raw(`SELECT "notnull", type FROM pragma_table_info('sys_network_device') WHERE name = 'deleted_at'`).Scan(&column).Error; err != nil || column.NotNull != 1 || column.Type != "INTEGER" {
		t.Fatalf("deleted_at shape = %+v, err=%v", column, err)
	}

	// An edit-only role still needs GET to load the current version. The
	// correction grants that dependency without granting sync/export or
	// changing the independently visible query action.
	editor := adminmodels.SysRole{RoleName: "Inventory editor", RoleKey: "inventory-editor"}
	if err = db.Create(&editor).Error; err != nil {
		t.Fatal(err)
	}
	edit := byName["NetworkDeviceEdit"]
	if err = db.Exec("INSERT INTO sys_role_menu (role_id, menu_id) VALUES (?, ?)", editor.RoleID, edit.MenuID).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = db.Transaction(bindNetworkInventoryEditRead); err != nil {
			t.Fatal(err)
		}
	}
	var readBack adminmodels.SysMenu
	if err = db.Preload("SysAPI").First(&readBack, edit.MenuID).Error; err != nil {
		t.Fatal(err)
	}
	methods := map[string]bool{}
	for _, endpoint := range readBack.SysAPI {
		methods[endpoint.Action+" "+endpoint.Path] = true
	}
	if len(methods) != 2 || !methods["GET /api/v1/network-device/:id"] || !methods["PUT /api/v1/network-device/:id"] {
		t.Fatalf("edit dependencies = %v", methods)
	}
	var granted int64
	if err = db.Table("casbin_rule").Where("v0 = ?", editor.RoleKey).Count(&granted).Error; err != nil || granted != 1 {
		t.Fatalf("editor dependency grants = %d, err=%v", granted, err)
	}
	if err = db.Table("casbin_rule").Where("v0 = ? AND v1 = ? AND v2 = 'GET'", editor.RoleKey, "/api/v1/network-device/:id").Count(&granted).Error; err != nil || granted != 1 {
		t.Fatalf("missing editor read grant: count=%d err=%v", granted, err)
	}
}
