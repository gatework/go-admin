package version

import (
	"runtime"

	contractmodels "github.com/go-admin-team/go-admin-core/v2/sdk/contract/models"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/seed"
	adminmodels "go-admin/app/admin/models"
	_ "go-admin/app/admin/service"
	"go-admin/app/network/grouping"
	networkmodels "go-admin/app/network/models"
	"go-admin/cmd/migrate/migration"
	common "go-admin/common/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), _1793010000000DeviceGroups)
}
func _1793010000000DeviceGroups(db *gorm.DB, version string) error {
	if err := seedDeviceGroups(db); err != nil {
		return err
	}
	return db.Create(&common.Migration{Version: version}).Error
}

// DDL may auto-commit on MySQL. Empty source markers survive a crash between
// schema creation and the transactional backfill; completed modes are untouched.
func seedDeviceGroups(db *gorm.DB) error {
	if err := db.AutoMigrate(&networkmodels.DeviceGroup{}, &networkmodels.DeviceGroupLock{}, &networkmodels.Device{}); err != nil {
		return err
	}
	if err := grouping.SeedAndBackfill(db); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := advanceNetworkSeedSequences(tx); err != nil {
			return err
		}
		if err := seed.SeedMenus(tx, "network", []seed.MenuSpec{
			{Code: "management", Kind: contractmodels.Directory, Title: "网络管理", Path: "/network", Component: "Layout", Icon: "branch", Sort: 30},
			{Code: "device-manage", Parent: "management", Kind: contractmodels.Menu, Title: "设备管理", Path: "/network/device", Component: "/network/device/index", Icon: "desktop", Sort: 1, ApiCodes: []string{"list", "options", "sync-status", "group-tree"}},
			{Code: "device-list", Parent: "device-manage", Kind: contractmodels.Button, Title: "查询台账", Permission: "network:device:list", Sort: 1, ApiCodes: []string{"list", "group-tree"}},
			{Code: "device-query", Parent: "device-manage", Kind: contractmodels.Button, Title: "查看设备", Permission: "network:device:query", Sort: 2, ApiCodes: []string{"query", "group-tree"}},
			{Code: "device-edit", Parent: "device-manage", Kind: contractmodels.Button, Title: "编辑台账", Permission: "network:device:edit", Sort: 3, ApiCodes: []string{"edit", "query", "group-tree", "assign-group"}},
			{Code: "device-group-add", Parent: "device-manage", Kind: contractmodels.Button, Title: "新增分组", Permission: "network:device-group:add", Sort: 6, ApiCodes: []string{"group-add", "group-tree"}},
			{Code: "device-group-edit", Parent: "device-manage", Kind: contractmodels.Button, Title: "编辑分组", Permission: "network:device-group:edit", Sort: 7, ApiCodes: []string{"group-edit", "group-tree"}},
			{Code: "device-group-remove", Parent: "device-manage", Kind: contractmodels.Button, Title: "删除分组", Permission: "network:device-group:remove", Sort: 8, ApiCodes: []string{"group-remove", "group-tree"}},
		}, []seed.ApiSpec{
			{Code: "list", Title: "设备台账列表", Path: "/api/v1/network-device", Method: "GET", Handle: "go-admin/app/network/apis.Device.GetPage-fm"},
			{Code: "query", Title: "设备台账详情", Path: "/api/v1/network-device/:id", Method: "GET", Handle: "go-admin/app/network/apis.Device.Get-fm"},
			{Code: "edit", Title: "编辑人工台账", Path: "/api/v1/network-device/:id", Method: "PUT", Handle: "go-admin/app/network/apis.Device.Update-fm"},
			{Code: "options", Title: "设备筛选选项", Path: "/api/v1/network-device/options", Method: "GET", Handle: "go-admin/app/network/apis.Device.Options-fm"},
			{Code: "sync-status", Title: "台账同步状态", Path: "/api/v1/network-device/sync-status", Method: "GET", Handle: "go-admin/app/network/apis.Device.SyncStatus-fm"},
			{Code: "group-tree", Title: "设备分组树", Path: "/api/v1/network-device-group", Method: "GET", Handle: "go-admin/app/network/apis.DeviceGroup.Tree-fm"},
			{Code: "group-add", Title: "新增设备分组", Path: "/api/v1/network-device-group", Method: "POST", Handle: "go-admin/app/network/apis.DeviceGroup.Create-fm"},
			{Code: "group-edit", Title: "编辑设备分组", Path: "/api/v1/network-device-group/:id", Method: "PUT", Handle: "go-admin/app/network/apis.DeviceGroup.Update-fm"},
			{Code: "group-remove", Title: "删除设备分组", Path: "/api/v1/network-device-group/:id", Method: "DELETE", Handle: "go-admin/app/network/apis.DeviceGroup.Delete-fm"},
			{Code: "assign-group", Title: "调整设备分组", Path: "/api/v1/network-device/assign-group", Method: "PUT", Handle: "go-admin/app/network/apis.Device.AssignGroup-fm"},
		}); err != nil {
			return err
		}
		return bindDeviceGroupRoleDependencies(tx)
	})
}

func bindDeviceGroupRoleDependencies(tx *gorm.DB) error {
	var bindings []struct{ RoleKey, Path, Action string }
	err := tx.Model(&adminmodels.SysRole{}).Distinct("sys_role.role_key, sys_api.path, sys_api.action").
		Joins("JOIN sys_role_menu ON sys_role_menu.role_id = sys_role.role_id").
		Joins("JOIN sys_menu ON sys_menu.menu_id = sys_role_menu.menu_id AND sys_menu.deleted_at = 0").
		Joins("JOIN sys_menu_api_rule ON sys_menu_api_rule.sys_menu_menu_id = sys_menu.menu_id").
		Joins("JOIN sys_api ON sys_api.id = sys_menu_api_rule.sys_api_id AND sys_api.deleted_at = 0").
		Where("sys_menu.app_code = ? AND sys_menu.seed_code IN ?", "network", []string{"device-manage", "device-list", "device-query", "device-edit", "device-group-add", "device-group-edit", "device-group-remove"}).Scan(&bindings).Error
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		result := tx.Exec(`INSERT INTO casbin_rule(ptype,v0,v1,v2,v3,v4,v5) SELECT 'p',?,?,?,'','','' WHERE NOT EXISTS (SELECT 1 FROM casbin_rule WHERE ptype='p' AND v0=? AND v1=? AND v2=?)`, binding.RoleKey, binding.Path, binding.Action, binding.RoleKey, binding.Path, binding.Action)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		grant := adminmodels.SysAppCasbinGrant{AppCode: "network", Ptype: "p", V0: binding.RoleKey, V1: binding.Path, V2: binding.Action}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&grant).Error; err != nil {
			return err
		}
	}
	return nil
}
