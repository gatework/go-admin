package version

import (
	"runtime"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	contractmodels "github.com/gatework/motor-core/sdk/contract/models"
	"github.com/gatework/motor-core/sdk/contract/seed"

	// Registers the host menu seeder before migrations run.
	_ "github.com/gatework/motor/app/admin/service"
	models "github.com/gatework/motor/app/net/models/firewall"
	firewall "github.com/gatework/motor/app/net/router"
	"github.com/gatework/motor/cmd/migrate/migration"
	common "github.com/gatework/motor/common/models"
)

//nolint:gochecknoinits // Frozen migration registration must complete before the host seals its startup registry.
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
		apis := []seed.APISpec{}
		byCapability := map[string][]string{}
		for _, r := range firewall.FirewallRoutes {
			code := "firewall-" + r.Code
			apis = append(apis, seed.APISpec{Code: code, Title: "Firewall " + r.Code, Path: "/api/v1/network-firewall" + r.Path, Method: r.Method, Handle: "go-admin/app/firewall/apis.Firewall." + r.Handler + "-fm"})
			byCapability[r.Capability] = append(byCapability[r.Capability], code)
		}
		apis = append(apis, seed.APISpec{Code: "firewall-inventory", Title: "防火墙设备选择", Path: "/api/v1/network-device", Method: "GET", Handle: "go-admin/app/network/apis.Device.GetPage-fm"}, seed.APISpec{Code: "firewall-groups", Title: "防火墙设备分组", Path: "/api/v1/network-device-group", Method: "GET", Handle: "go-admin/app/network/apis.DeviceGroup.Tree-fm"})
		read := append(append([]string{}, byCapability["list"]...), "firewall-inventory", "firewall-groups")
		menus := []seed.MenuSpec{
			{Code: "management", Kind: contractmodels.Directory, Title: "网络管理", Path: "/network", Component: "Layout", Icon: "branch", Sort: 30},
			{Code: "firewall-objects", Parent: "management", Kind: contractmodels.Menu, Title: "防火墙对象", Path: "/network/firewall", Component: "/network/firewall/index", Icon: "safe", Sort: 7, APICodes: read},
			{Code: "firewall-query", Parent: "management", Kind: contractmodels.Menu, Title: "策略查询", Path: "/network/firewall/query", Component: "/network/firewall/query", Icon: "search", Sort: 8, APICodes: read},
			{Code: "firewall-design", Parent: "management", Kind: contractmodels.Menu, Title: "策略设计", Path: "/network/firewall/design", Component: "/network/firewall/design", Icon: "edit", Sort: 9, APICodes: read},
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
			menus = append(menus, seed.MenuSpec{Code: "firewall-action-" + action, Parent: parent, Kind: contractmodels.Button, Title: map[string]string{"list": "查看防火墙对象", "query": "查询防火墙策略", "import": "导入防火墙配置", "edit": "维护保护范围", "design": "维护策略设计"}[action], Permission: "network:firewall:" + action, Sort: i + 1, APICodes: codes})
		}
		return seed.Menus(tx, "network", menus, apis)
	})
}
