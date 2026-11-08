package version

import (
	"runtime"

	contractmodels "github.com/go-admin-team/go-admin-core/v2/sdk/contract/models"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/seed"
	adminmodels "go-admin/app/admin/models"
	_ "go-admin/app/admin/service"
	networkmodels "go-admin/app/network/models"
	"go-admin/cmd/migrate/migration"
	common "go-admin/common/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), _1793000000000NetworkCircuits)
}
func _1793000000000NetworkCircuits(db *gorm.DB, version string) error {
	if err := seedNetworkCircuits(db); err != nil {
		return err
	}
	return db.Create(&common.Migration{Version: version}).Error
}
func seedNetworkCircuits(db *gorm.DB) error {
	if err := db.AutoMigrate(&networkmodels.Circuit{}); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := advanceNetworkSeedSequences(tx); err != nil {
			return err
		}
		if err := seed.SeedMenus(tx, "network", []seed.MenuSpec{
			{Code: "management", Kind: contractmodels.Directory, Title: "网络管理", Path: "/network", Component: "Layout", Icon: "branch", Sort: 30},
			{Code: "circuit-manage", Parent: "management", Kind: contractmodels.Menu, Title: "专线管理", Path: "/network/circuit", Component: "/network/circuit/index", Icon: "swap", Sort: 3, ApiCodes: []string{"circuit-list", "circuit-options"}},
			{Code: "circuit-list", Parent: "circuit-manage", Kind: contractmodels.Button, Title: "查询专线", Permission: "network:circuit:list", Sort: 1, ApiCodes: []string{"circuit-list", "circuit-options"}},
			{Code: "circuit-query", Parent: "circuit-manage", Kind: contractmodels.Button, Title: "查看专线", Permission: "network:circuit:query", Sort: 2, ApiCodes: []string{"circuit-query"}},
			{Code: "circuit-add", Parent: "circuit-manage", Kind: contractmodels.Button, Title: "新增专线", Permission: "network:circuit:add", Sort: 3, ApiCodes: []string{"circuit-add", "circuit-options", "circuit-hosts", "circuit-items", "device-list"}},
			{Code: "circuit-edit", Parent: "circuit-manage", Kind: contractmodels.Button, Title: "编辑专线", Permission: "network:circuit:edit", Sort: 4, ApiCodes: []string{"circuit-edit", "circuit-query", "circuit-options", "circuit-hosts", "circuit-items", "device-list"}},
			{Code: "circuit-remove", Parent: "circuit-manage", Kind: contractmodels.Button, Title: "删除规划中专线", Permission: "network:circuit:remove", Sort: 5, ApiCodes: []string{"circuit-remove"}},
			{Code: "circuit-export", Parent: "circuit-manage", Kind: contractmodels.Button, Title: "导出专线", Permission: "network:circuit:export", Sort: 6, ApiCodes: []string{"circuit-export"}},
			{Code: "circuit-traffic", Parent: "circuit-manage", Kind: contractmodels.Button, Title: "查看流量", Permission: "network:circuit:traffic", Sort: 7, ApiCodes: []string{"circuit-traffic"}},
		}, []seed.ApiSpec{
			{Code: "circuit-list", Title: "专线台账列表", Path: "/api/v1/network-circuit", Method: "GET", Handle: "go-admin/app/network/apis.Circuit.GetPage-fm"},
			{Code: "circuit-query", Title: "专线详情", Path: "/api/v1/network-circuit/:id", Method: "GET", Handle: "go-admin/app/network/apis.Circuit.Get-fm"},
			{Code: "circuit-add", Title: "新增专线", Path: "/api/v1/network-circuit", Method: "POST", Handle: "go-admin/app/network/apis.Circuit.Create-fm"},
			{Code: "circuit-edit", Title: "编辑专线", Path: "/api/v1/network-circuit/:id", Method: "PUT", Handle: "go-admin/app/network/apis.Circuit.Update-fm"},
			{Code: "circuit-remove", Title: "删除规划中专线", Path: "/api/v1/network-circuit/:id", Method: "DELETE", Handle: "go-admin/app/network/apis.Circuit.Delete-fm"},
			{Code: "circuit-options", Title: "专线筛选选项", Path: "/api/v1/network-circuit/options", Method: "GET", Handle: "go-admin/app/network/apis.Circuit.Options-fm"},
			{Code: "circuit-export", Title: "导出专线台账", Path: "/api/v1/network-circuit/export", Method: "GET", Handle: "go-admin/app/network/apis.Circuit.Export-fm"},
			{Code: "circuit-hosts", Title: "搜索 Zabbix 主机", Path: "/api/v1/network-circuit/zabbix/hosts", Method: "GET", Handle: "go-admin/app/network/apis.Circuit.Hosts-fm"},
			{Code: "circuit-items", Title: "查询 Zabbix 速率监控项", Path: "/api/v1/network-circuit/zabbix/items", Method: "GET", Handle: "go-admin/app/network/apis.Circuit.Items-fm"},
			{Code: "circuit-traffic", Title: "专线流量", Path: "/api/v1/network-circuit/:id/traffic", Method: "GET", Handle: "go-admin/app/network/apis.Circuit.Traffic-fm"},
			{Code: "device-list", Title: "设备台账列表", Path: "/api/v1/network-device", Method: "GET", Handle: "go-admin/app/network/apis.Device.GetPage-fm"},
		}); err != nil {
			return err
		}
		return backfillCircuitRolePolicies(tx)
	})
}

// Reconcile every already assigned role, not only admin, after adding API
// dependencies. The ledger records only policies created by this migration.
func backfillCircuitRolePolicies(tx *gorm.DB) error {
	var bindings []struct{ RoleKey, Path, Action string }
	err := tx.Model(&adminmodels.SysRole{}).Distinct("sys_role.role_key, sys_api.path, sys_api.action").
		Joins("JOIN sys_role_menu ON sys_role_menu.role_id = sys_role.role_id").
		Joins("JOIN sys_menu ON sys_menu.menu_id = sys_role_menu.menu_id AND sys_menu.deleted_at = 0").
		Joins("JOIN sys_menu_api_rule ON sys_menu_api_rule.sys_menu_menu_id = sys_menu.menu_id").
		Joins("JOIN sys_api ON sys_api.id = sys_menu_api_rule.sys_api_id AND sys_api.deleted_at = 0").
		Where("sys_menu.app_code = ? AND sys_menu.seed_code LIKE ?", "network", "circuit-%").Scan(&bindings).Error
	if err != nil {
		return err
	}
	for _, b := range bindings {
		result := tx.Exec(`INSERT INTO casbin_rule (ptype,v0,v1,v2,v3,v4,v5) SELECT 'p',?,?,?,'','','' WHERE NOT EXISTS (SELECT 1 FROM casbin_rule WHERE ptype='p' AND v0=? AND v1=? AND v2=?)`, b.RoleKey, b.Path, b.Action, b.RoleKey, b.Path, b.Action)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}
		grant := adminmodels.SysAppCasbinGrant{AppCode: "network", Ptype: "p", V0: b.RoleKey, V1: b.Path, V2: b.Action}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&grant).Error; err != nil {
			return err
		}
	}
	return nil
}
