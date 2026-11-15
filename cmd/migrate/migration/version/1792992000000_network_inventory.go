package version

import (
	"fmt"
	"runtime"

	"gorm.io/gorm"

	contractmodels "github.com/gatework/motor-core/sdk/contract/models"
	"github.com/gatework/motor-core/sdk/contract/seed"

	// Registers the host menu seeder before migrations run.
	_ "github.com/gatework/motor/app/admin/service"
	networkmodels "github.com/gatework/motor/app/net/models/inventory" // Register the host's idempotent menu seeder.

	"github.com/gatework/motor/cmd/migrate/migration"
	common "github.com/gatework/motor/common/models"
)

//nolint:gochecknoinits // Frozen migration registration must complete before the host seals its startup registry.
func init() {
	_, fileName, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(fileName), _1792992000000NetworkInventory)
}

func _1792992000000NetworkInventory(db *gorm.DB, version string) error {
	if err := seedNetworkInventory(db); err != nil {
		return err
	}
	return db.Create(&common.Migration{Version: version}).Error
}

// Separate schema creation from the transaction: MySQL DDL commits implicitly.
// Both steps tolerate retries without resetting records or manual menu settings.
func seedNetworkInventory(db *gorm.DB) error {
	if err := db.AutoMigrate(&networkmodels.Device{}, &networkmodels.SyncState{}); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := advanceNetworkSeedSequences(tx); err != nil {
			return err
		}
		return seed.Menus(tx, "network", []seed.MenuSpec{
			{Code: "management", Kind: contractmodels.Directory, Title: "网络管理", Path: "/network", Component: "Layout", Icon: "branch", Sort: 30},
			{Code: "device-manage", Parent: "management", Kind: contractmodels.Menu, Title: "设备管理", Path: "/network/device", Component: "/network/device/index", Icon: "desktop", Sort: 1, APICodes: []string{"list", "options", "sync-status"}},
			{Code: "device-list", Parent: "device-manage", Kind: contractmodels.Button, Title: "查询台账", Permission: "network:device:list", Sort: 1, APICodes: []string{"list"}},
			{Code: "device-query", Parent: "device-manage", Kind: contractmodels.Button, Title: "查看设备", Permission: "network:device:query", Sort: 2, APICodes: []string{"query"}},
			{Code: "device-edit", Parent: "device-manage", Kind: contractmodels.Button, Title: "编辑台账", Permission: "network:device:edit", Sort: 3, APICodes: []string{"edit"}},
			{Code: "device-sync", Parent: "device-manage", Kind: contractmodels.Button, Title: "同步台账", Permission: "network:device:sync", Sort: 4, APICodes: []string{"sync"}},
			{Code: "device-export", Parent: "device-manage", Kind: contractmodels.Button, Title: "导出台账", Permission: "network:device:export", Sort: 5, APICodes: []string{"export"}},
		}, []seed.APISpec{
			{Code: "list", Title: "设备台账列表", Path: "/api/v1/network-device", Method: "GET", Handle: "go-admin/app/network/apis.Device.GetPage-fm"},
			{Code: "query", Title: "设备台账详情", Path: "/api/v1/network-device/:id", Method: "GET", Handle: "go-admin/app/network/apis.Device.Get-fm"},
			{Code: "edit", Title: "编辑人工台账", Path: "/api/v1/network-device/:id", Method: "PUT", Handle: "go-admin/app/network/apis.Device.Update-fm"},
			{Code: "options", Title: "设备筛选选项", Path: "/api/v1/network-device/options", Method: "GET", Handle: "go-admin/app/network/apis.Device.Options-fm"},
			{Code: "sync-status", Title: "台账同步状态", Path: "/api/v1/network-device/sync-status", Method: "GET", Handle: "go-admin/app/network/apis.Device.SyncStatus-fm"},
			{Code: "sync", Title: "从 Netdisco 同步台账", Path: "/api/v1/network-device/sync", Method: "POST", Handle: "go-admin/app/network/apis.Device.Sync-fm"},
			{Code: "export", Title: "导出设备台账", Path: "/api/v1/network-device/export", Method: "GET", Handle: "go-admin/app/network/apis.Device.Export-fm"},
		})
	})
}

// Legacy PostgreSQL seeds insert explicit IDs without advancing their sequences.
// Keep new menu/API allocation above both stored IDs and already allocated IDs.
func advanceNetworkSeedSequences(tx *gorm.DB) error {
	if tx.Name() != "postgres" {
		return nil
	}
	if err := tx.Exec("LOCK TABLE sys_api, sys_menu IN SHARE ROW EXCLUSIVE MODE").Error; err != nil {
		return err
	}
	for _, key := range [][2]string{{"sys_api", "id"}, {"sys_menu", "menu_id"}} {
		query := fmt.Sprintf(`SELECT setval(pg_get_serial_sequence(?, ?)::regclass,
			GREATEST(COALESCE((SELECT MAX(%s) FROM %s), 1),
			COALESCE(pg_sequence_last_value(pg_get_serial_sequence(?, ?)::regclass), 1)), true)`, key[1], key[0])
		if err := tx.Exec(query, key[0], key[1], key[0], key[1]).Error; err != nil {
			return err
		}
	}
	return nil
}
