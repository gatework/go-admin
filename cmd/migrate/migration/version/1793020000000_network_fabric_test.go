package version

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	admin "github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/net/models/fabric"
)

func TestFabricMigrationPreservesRulesAndSeedsScopedCapabilities(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "fabric.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&admin.SysMenu{}, &admin.SysAPI{}, &admin.SysRole{}, &admin.SysAppCasbinGrant{}); err != nil {
		t.Fatal(err)
	}
	createCasbinRule(t, db)
	if err = db.Create(&admin.SysRole{RoleName: "Admin", RoleKey: "admin"}).Error; err != nil {
		t.Fatal(err)
	}
	if err = seedNetworkFabric(db); err != nil {
		t.Fatal(err)
	}
	var rules fabric.RuleSet
	if err = db.First(&rules, 1).Error; err != nil {
		t.Fatal(err)
	}
	if len(rules.Rules) != 6 {
		t.Fatalf("initial regions: %#v", rules)
	}
	rules.Version = 2
	rules.Rules[0].Pattern = "(?i)^CUSTOM-"
	if err = db.Save(&rules).Error; err != nil {
		t.Fatal(err)
	}
	if err = seedNetworkFabric(db); err != nil {
		t.Fatal(err)
	}
	var saved fabric.RuleSet
	if err = db.First(&saved, 1).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Version != 2 || saved.Rules[0].Pattern != "(?i)^CUSTOM-" {
		t.Fatal("seed overwrote edited rules")
	}
	var count int64
	db.Model(&fabric.Fabric{}).Count(&count)
	if count != 6 {
		t.Fatalf("duplicate fabrics: %d", count)
	}
	var menu admin.SysMenu
	if err = db.Where("app_code = ? AND seed_code = ?", "network", "fabric-manage").First(&menu).Error; err != nil {
		t.Fatal(err)
	}
	if menu.MenuName != "NetworkFabricManage" || menu.Component != "/network/fabric/index" {
		t.Fatalf("menu: %#v", menu)
	}
	for _, action := range []string{"list", "query", "edit", "collect", "preview", "execute"} {
		var button admin.SysMenu
		if err = db.Preload("SysAPI").Where("permission = ?", "network:fabric:"+action).First(&button).Error; err != nil {
			t.Fatal(action, err)
		}
		if len(button.SysAPI) == 0 {
			t.Fatal("no APIs", action)
		}
		candidateAccess := false
		for _, api := range button.SysAPI {
			if api.Path == "/api/v1/network-fabrics/:id/change-candidates" && api.Action == "POST" {
				candidateAccess = true
			}
			if action == "list" || action == "query" {
				if api.Action != "GET" {
					t.Fatalf("read permission grants write: %s %s", action, api.Path)
				}
			}
		}
		if candidateAccess != (action == "preview") {
			t.Fatalf("candidate checking must belong to preview capability: %s %v", action, candidateAccess)
		}
	}
}
