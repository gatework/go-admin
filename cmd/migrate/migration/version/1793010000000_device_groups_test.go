package version

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	adminmodels "github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/net/models/grouping"
	models "github.com/gatework/motor/app/net/models/inventory"
)

func deviceGroupMigrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "groups.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&adminmodels.SysMenu{}, &adminmodels.SysAPI{}, &adminmodels.SysRole{}, &adminmodels.SysAppCasbinGrant{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TABLE casbin_rule (id INTEGER PRIMARY KEY AUTOINCREMENT,ptype TEXT,v0 TEXT,v1 TEXT,v2 TEXT,v3 TEXT,v4 TEXT,v5 TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	if err = seedNetworkInventory(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestDeviceGroupsMigrationPreservesLegacyMetadataAndResumesBackfill(t *testing.T) {
	db := deviceGroupMigrationDB(t)
	legacyLabel := "  " + strings.Repeat("区", 126)
	rows := []models.Device{
		{IP: "192.0.2.1", Name: "SZX9-CSW1", SourceTags: []string{}, LegacyGroup: legacyLabel, DeviceType: "router", DeviceRole: "other", Owner: "Alice", Tags: []string{"important"}, Notes: "keep", Version: 4},
		{IP: "192.0.2.2", Name: "SHA-SW01", SourceTags: []string{}, Tags: []string{}, Version: 1},
		{IP: "192.0.2.3", Serial: "CKG-ASW1", SourceTags: []string{}, Tags: []string{}, Version: 1},
		{IP: "192.0.2.4", Name: "ordinary", Serial: "SZX9-FW1", SourceTags: []string{}, Tags: []string{}, Version: 1},
	}
	for i := range rows {
		rows[i].SetCreateBy(7)
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Simulate a process stopping after DDL. Empty markers must survive retries.
	if err := db.AutoMigrate(&models.DeviceGroup{}, &models.DeviceGroupLock{}, &models.Device{}); err != nil {
		t.Fatal(err)
	}
	if err := seedDeviceGroups(db); err != nil {
		t.Fatal(err)
	}
	var after []models.Device
	db.Order("id").Find(&after)
	if after[0].GroupSource != "manual" || after[0].ClassificationSource != "manual" || after[0].DeviceType != "router" || after[0].DeviceRole != "other" || after[0].Owner != "Alice" || after[0].Notes != "keep" || after[0].CreateBy != 7 || after[0].LegacyGroup != legacyLabel || after[0].Version != 5 {
		t.Fatalf("legacy changed: %+v", after[0])
	}
	var legacy models.DeviceGroup
	if err := db.First(&legacy, *after[0].GroupID).Error; err != nil || legacy.Name != legacyLabel || !strings.HasPrefix(legacy.Code, "legacy.") {
		t.Fatalf("legacy label lost: %+v %v", legacy, err)
	}
	for i, expect := range []struct{ code, role, kind string }{{"site.shanghai", "access_switch", "switch"}, {"site.chongqing", "aggregate_switch", "switch"}, {"site.other", "other", "other"}} {
		row := after[i+1]
		var group models.DeviceGroup
		db.First(&group, *row.GroupID)
		if group.Code != expect.code || row.DeviceRole != expect.role || row.DeviceType != expect.kind || row.ClassificationSource != "auto" || row.GroupSource != "auto" || row.Version != 2 {
			t.Fatalf("backfill: %+v %+v", row, group)
		}
	}
	var group models.DeviceGroup
	db.Where("code = ?", "site.shanghai").First(&group)
	db.Model(&group).Updates(map[string]any{"name": "自定义区域", "description": "keep description", "enabled": false, "parent_id": legacy.ID, "position": 44})
	db.Model(&models.Device{}).Where("id = ?", after[1].ID).Updates(map[string]any{"group_id": nil, "group_source": "manual", "classification_source": "manual", "device_type": "firewall", "device_role": "firewall"})
	if err := seedDeviceGroups(db); err != nil {
		t.Fatal(err)
	}
	var repeated models.Device
	db.First(&repeated, after[1].ID)
	db.First(&group, group.ID)
	if repeated.GroupID != nil || repeated.GroupSource != "manual" || repeated.DeviceType != "firewall" || repeated.Version != after[1].Version || group.Name != "自定义区域" || group.Enabled || group.ParentID == nil || *group.ParentID != legacy.ID || group.Description != "keep description" || group.Position != 44 {
		t.Fatalf("retry reset edits: %+v %+v", repeated, group)
	}
	var count int64
	db.Model(&models.DeviceGroup{}).Count(&count)
	if count != 8 {
		t.Fatalf("duplicate groups: %d", count)
	}
}

func TestDeviceGroupsMigrationBindsOnlyNeededRoleDependencies(t *testing.T) {
	db := deviceGroupMigrationDB(t)
	for _, item := range []struct{ role, menu string }{{"list-only", "device-list"}, {"query-only", "device-query"}, {"edit-only", "device-edit"}, {"export-only", "device-export"}} {
		role := adminmodels.SysRole{RoleKey: item.role, RoleName: item.role}
		db.Create(&role)
		var menu adminmodels.SysMenu
		db.Where("app_code = 'network' AND seed_code = ?", item.menu).First(&menu)
		if err := db.Exec("INSERT INTO sys_role_menu(role_id,menu_id) VALUES (?,?)", role.RoleID, menu.MenuID).Error; err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := seedDeviceGroups(db); err != nil {
			t.Fatal(err)
		}
	}
	for _, role := range []string{"list-only", "query-only", "edit-only", "export-only"} {
		var count int64
		db.Table("casbin_rule").Where("v0 = ? AND v1 = ? AND v2 = 'GET'", role, "/api/v1/network-device-group").Count(&count)
		want := int64(1)
		if role == "export-only" {
			want = 0
		}
		if count != want {
			t.Errorf("tree grant %s=%d want %d", role, count, want)
		}
		db.Table("casbin_rule").Where("v0 = ? AND v1 = ? AND v2 = 'PUT'", role, "/api/v1/network-device/assign-group").Count(&count)
		want = 0
		if role == "edit-only" {
			want = 1
		}
		if count != want {
			t.Errorf("batch grant %s=%d", role, count)
		}
		db.Table("casbin_rule").Where("v0 = ? AND v1 = ? AND v2 = 'POST'", role, "/api/v1/network-device-group").Count(&count)
		if count != 0 {
			t.Errorf("unrelated group create grant %s", role)
		}
	}
	for _, permission := range []string{"network:device-group:add", "network:device-group:edit", "network:device-group:remove"} {
		var count int64
		db.Model(&adminmodels.SysMenu{}).Where("permission = ?", permission).Count(&count)
		if count != 1 {
			t.Errorf("permission %s=%d", permission, count)
		}
	}
}

func TestDeviceGroupsLegacyStandardRegionKeepsStableCodeAfterRename(t *testing.T) {
	db := deviceGroupMigrationDB(t)
	row := models.Device{IP: "192.0.2.1", Name: "SZX9-CSW1", SourceTags: []string{}, LegacyGroup: "传音大厦", Tags: []string{}, Version: 1}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := seedDeviceGroups(db); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&row, row.ID).Error; err != nil {
		t.Fatal(err)
	}
	var group models.DeviceGroup
	if err := db.First(&group, *row.GroupID).Error; err != nil {
		t.Fatal(err)
	}
	if group.Code != "site.transsion-tower" || row.GroupSource != "manual" {
		t.Fatalf("legacy standard group lost stable identity: %+v %+v", group, row)
	}
	if err := db.Model(&group).Update("name", "大厦园区").Error; err != nil {
		t.Fatal(err)
	}
	if err := seedDeviceGroups(db); err != nil {
		t.Fatal(err)
	}
	groups, err := grouping.Load(db)
	if err != nil {
		t.Fatal(err)
	}
	resolved := groups.Resolve("SZX9-SW1", "", nil)
	if resolved == nil || *resolved != group.ID {
		t.Fatalf("renamed standard target was lost: %v", resolved)
	}
}
