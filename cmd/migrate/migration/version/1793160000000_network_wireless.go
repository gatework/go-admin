package version

import (
	"runtime"

	"gorm.io/gorm"

	contractmodels "github.com/gatework/motor-core/sdk/contract/models"
	"github.com/gatework/motor-core/sdk/contract/seed"

	// Registers the host menu seeder before migrations run.
	_ "github.com/gatework/motor/app/admin/service"
	models "github.com/gatework/motor/app/net/models/wireless"

	"github.com/gatework/motor/cmd/migrate/migration"
	common "github.com/gatework/motor/common/models"
)

//nolint:gochecknoinits // Frozen migration registration must complete before the host seals its startup registry.
func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), _1793160000000NetworkWireless)
}
func _1793160000000NetworkWireless(db *gorm.DB, version string) error {
	if e := seedNetworkWireless(db); e != nil {
		return e
	}
	return db.Create(&common.Migration{Version: version}).Error
}
func seedNetworkWireless(db *gorm.DB) error {
	if e := db.AutoMigrate(&models.Dimension{}, &models.Resource{}, &models.NeighborState{}, &models.Collection{}, &models.Audit{}); e != nil {
		return e
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if e := advanceNetworkSeedSequences(tx); e != nil {
			return e
		}
		type route struct{ code, path, method, handler string }
		routes := []route{
			{"controllers", "/controllers", "GET", "Controllers"}, {"groups", "/groups", "GET", "Groups"}, {"switches", "/switches", "GET", "Switches"}, {"clients", "/clients", "GET", "Clients"},
			{"controller", "/controllers/:controllerId", "GET", "Controller"}, {"analysis", "/controllers/:controllerId/analysis", "GET", "Analysis"},
			{"aps", "/controllers/:controllerId/access-points", "GET", "AccessPoints"}, {"ap", "/controllers/:controllerId/access-points/:id", "GET", "APDetail"},
			{"radios", "/controllers/:controllerId/radios", "GET", "Radios"}, {"bsses", "/controllers/:controllerId/bsses", "GET", "BSSes"}, {"client", "/controllers/:controllerId/clients/:id", "GET", "Client"},
			{"client-resources", "/controllers/:controllerId/clients", "GET", "ClientResources"},
			{"collections", "/controllers/:controllerId/collections", "GET", "Collections"}, {"collection", "/controllers/:controllerId/collections/:id", "GET", "Collection"},
			{"collect", "/controllers/:controllerId/collections", "POST", "Collect"}, {"management", "/controllers/:controllerId/access-points/:id/management", "PUT", "Management"}, {"neighbor", "/controllers/:controllerId/access-points/:id/neighbor", "PUT", "Associate"},
			{"export", "/controllers/:controllerId/exports/:kind", "GET", "Export"}, {"preview-import", "/imports/preview", "POST", "PreviewImport"}, {"commit-import", "/imports/commit", "POST", "CommitImport"},
		}
		apis := []seed.APISpec{}
		read := []string{}
		for _, r := range routes {
			apis = append(apis, seed.APISpec{Code: "wireless-" + r.code, Title: "Wireless " + r.code, Path: "/api/v1/network-wireless" + r.path, Method: r.method, Handle: "go-admin/app/wireless/apis.Wireless." + r.handler + "-fm"})
			if r.method == "GET" && r.code != "export" {
				read = append(read, "wireless-"+r.code)
			}
		}
		menus := []seed.MenuSpec{{Code: "management", Kind: contractmodels.Directory, Title: "网络管理", Path: "/network", Component: "Layout", Icon: "branch", Sort: 30}, {Code: "wireless-manage", Parent: "management", Kind: contractmodels.Menu, Title: "无线管理", Path: "/network/wireless", Component: "/network/wireless/index", Icon: "wifi", Sort: 6, APICodes: []string{"wireless-controllers", "wireless-groups"}}}
		for i, action := range []string{"list", "query", "collect", "edit", "import", "export"} {
			codes := append([]string{}, read...)
			switch action {
			case "list":
				codes = []string{"wireless-controllers", "wireless-groups"}
			case "collect":
				codes = append(codes, "wireless-collect")
			case "edit":
				codes = append(codes, "wireless-management", "wireless-neighbor")
			case "import":
				codes = append(codes, "wireless-preview-import", "wireless-commit-import")
			case "export":
				codes = append(codes, "wireless-export")
			}
			menus = append(menus, seed.MenuSpec{Code: "wireless-" + action, Parent: "wireless-manage", Kind: contractmodels.Button, Title: "Wireless " + action, Permission: "network:wireless:" + action, Sort: i + 1, APICodes: codes})
		}
		return seed.Menus(tx, "network", menus, apis)
	})
}
