package version

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	admin "github.com/gatework/motor/app/admin/models"
	models "github.com/gatework/motor/app/net/models/firewall"
)

func TestFirewallCollectionMigrationAddsSeparateReadCapabilities(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "collection.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&admin.SysMenu{}, &admin.SysAPI{}, &admin.SysRole{}, &admin.SysAppCasbinGrant{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TABLE casbin_rule (id INTEGER PRIMARY KEY AUTOINCREMENT, ptype TEXT,v0 TEXT,v1 TEXT,v2 TEXT,v3 TEXT,v4 TEXT,v5 TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	if err = seedNetworkFirewall(db); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = seedFirewallCollection(db); err != nil {
			t.Fatal(err)
		}
	}
	for _, model := range []any{&models.CollectionBatch{}, &models.CollectionTarget{}} {
		if !db.Migrator().HasTable(model) {
			t.Fatalf("missing %T", model)
		}
	}
	for _, capability := range []string{"collect", "connection", "list"} {
		var menu admin.SysMenu
		if err = db.Where("permission = ?", "network:firewall:"+capability).Preload("SysAPI").First(&menu).Error; err != nil {
			t.Fatal(err)
		}
		found := false
		for _, api := range menu.SysAPI {
			if capability == "list" && api.Action != "GET" {
				t.Fatal("reader can mutate")
			}
			if capability == "collect" && api.Action == "POST" && api.Path == "/api/v1/network-firewall/collection-batches" {
				found = true
			}
			if capability == "connection" && api.Action == "PUT" && api.Path == "/api/v1/network-firewall/devices/:id/connection" {
				found = true
			}
		}
		if capability != "list" && !found {
			t.Fatalf("missing %s grant", capability)
		}
	}
}
