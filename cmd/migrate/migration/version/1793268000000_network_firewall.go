package version

import (
	"runtime"

	contractmodels "github.com/go-admin-team/go-admin-core/v2/sdk/contract/models"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/seed"
	_ "go-admin/app/admin/service"
	"go-admin/app/firewall"
	"go-admin/app/firewall/models"
	"go-admin/cmd/migrate/migration"
	common "go-admin/common/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), _1793268000000NetworkFirewall)
}
func _1793268000000NetworkFirewall(db *gorm.DB, version string) error {
	if e := seedNetworkFirewall(db); e != nil {
		return e
	}
	return db.Create(&common.Migration{Version: version}).Error
}
func seedNetworkFirewall(db *gorm.DB) error {
	if e := models.Migrate(db); e != nil {
		return e
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if e := advanceNetworkSeedSequences(tx); e != nil {
			return e
		}
		if e := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "code"}}, DoNothing: true}).Create(&models.Domain{Code: "default", Name: "默认网络域", Version: 1}).Error; e != nil {
			return e
		}
		apis := []seed.ApiSpec{}
		byCapability := map[string][]string{}
		for _, r := range firewall.Routes {
			code := "firewall-" + r.Code
			apis = append(apis, seed.ApiSpec{Code: code, Title: "Firewall " + r.Code, Path: "/api/v1/network-firewall" + r.Path, Method: r.Method, Handle: "go-admin/app/firewall/apis.Firewall." + r.Handler + "-fm"})
			byCapability[r.Capability] = append(byCapability[r.Capability], code)
		}
		apis = append(apis, seed.ApiSpec{Code: "firewall-inventory", Title: "防火墙设备选择", Path: "/api/v1/network-device", Method: "GET", Handle: "go-admin/app/network/apis.Device.GetPage-fm"}, seed.ApiSpec{Code: "firewall-groups", Title: "防火墙设备分组", Path: "/api/v1/network-device-group", Method: "GET", Handle: "go-admin/app/network/apis.DeviceGroup.Tree-fm"})
		read := append(append([]string{}, byCapability["list"]...), "firewall-inventory", "firewall-groups")
		menus := []seed.MenuSpec{
			{Code: "management", Kind: contractmodels.Directory, Title: "网络管理", Path: "/network", Component: "Layout", Icon: "branch", Sort: 30},
			{Code: "firewall-objects", Parent: "management", Kind: contractmodels.Menu, Title: "防火墙对象", Path: "/network/firewall", Component: "/network/firewall/index", Icon: "safe", Sort: 7, ApiCodes: read},
			{Code: "firewall-query", Parent: "management", Kind: contractmodels.Menu, Title: "策略查询", Path: "/network/firewall/query", Component: "/network/firewall/query", Icon: "search", Sort: 8, ApiCodes: read},
			{Code: "firewall-design", Parent: "management", Kind: contractmodels.Menu, Title: "策略设计", Path: "/network/firewall/design", Component: "/network/firewall/design", Icon: "edit", Sort: 9, ApiCodes: read},
		}
		for i, action := range []string{"list", "query", "import", "edit", "design"} {
			codes := append([]string{}, read...)
			if action != "list" {
				codes = append(codes, byCapability[action]...)
			}
			parent := "firewall-objects"
			if action == "query" {
				parent = "firewall-query"
			}
			if action == "design" {
				parent = "firewall-design"
				codes = append(codes, byCapability["query"]...)
			}
			menus = append(menus, seed.MenuSpec{Code: "firewall-action-" + action, Parent: parent, Kind: contractmodels.Button, Title: map[string]string{"list": "查看防火墙对象", "query": "查询防火墙策略", "import": "导入防火墙配置", "edit": "维护保护范围", "design": "维护策略设计"}[action], Permission: "network:firewall:" + action, Sort: i + 1, ApiCodes: codes})
		}
		return seed.SeedMenus(tx, "network", menus, apis)
	})
}
