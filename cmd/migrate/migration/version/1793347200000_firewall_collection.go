package version

import (
	contractmodels "github.com/go-admin-team/go-admin-core/v2/sdk/contract/models"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/seed"
	"go-admin/app/firewall"
	"go-admin/app/firewall/models"
	"go-admin/cmd/migrate/migration"
	common "go-admin/common/models"
	"gorm.io/gorm"
	"runtime"
)

func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), _1793347200000FirewallCollection)
}
func _1793347200000FirewallCollection(db *gorm.DB, version string) error {
	if err := seedFirewallCollection(db); err != nil {
		return err
	}
	return db.Create(&common.Migration{Version: version}).Error
}
func seedFirewallCollection(db *gorm.DB) error {
	if err := models.Migrate(db); err != nil {
		return err
	}
	// Refresh read bindings for existing installations without rewriting their
	// executed migration or changing manually assigned role memberships.
	if err := seedNetworkFirewall(db); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := advanceNetworkSeedSequences(tx); err != nil {
			return err
		}
		apis := []seed.ApiSpec{}
		codes := map[string][]string{}
		for _, r := range firewall.Routes {
			code := "firewall-" + r.Code
			apis = append(apis, seed.ApiSpec{Code: code, Title: "Firewall " + r.Code, Path: "/api/v1/network-firewall" + r.Path, Method: r.Method, Handle: "go-admin/app/firewall/apis.Firewall." + r.Handler + "-fm"})
			codes[r.Capability] = append(codes[r.Capability], code)
		}
		menus := []seed.MenuSpec{
			{Code: "management", Kind: contractmodels.Directory, Title: "网络管理", Path: "/network", Component: "Layout", Icon: "branch", Sort: 30},
			{Code: "firewall-objects", Parent: "management", Kind: contractmodels.Menu, Title: "防火墙对象", Path: "/network/firewall", Component: "/network/firewall/index", Icon: "safe", Sort: 7, ApiCodes: codes["list"]},
		}
		for n, capability := range []string{"collect", "connection"} {
			menus = append(menus, seed.MenuSpec{Code: "firewall-action-" + capability, Parent: "firewall-objects", Kind: contractmodels.Button, Title: map[string]string{"collect": "登录测试与配置采集", "connection": "维护防火墙连接"}[capability], Permission: "network:firewall:" + capability, Sort: 6 + n, ApiCodes: append(append([]string{}, codes["list"]...), codes[capability]...)})
		}
		return seed.SeedMenus(tx, "network", menus, apis)
	})
}
