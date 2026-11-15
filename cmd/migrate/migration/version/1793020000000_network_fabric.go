package version

import (
	"runtime"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	contractmodels "github.com/gatework/motor-core/sdk/contract/models"
	"github.com/gatework/motor-core/sdk/contract/seed"

	// Registers the host menu seeder before migrations run.
	_ "github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/net/models/delivery"
	models "github.com/gatework/motor/app/net/models/fabric"
	"github.com/gatework/motor/cmd/migrate/migration"
	common "github.com/gatework/motor/common/models"
)

//nolint:gochecknoinits // Frozen migration registration must complete before the host seals its startup registry.
func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), _1793020000000NetworkFabric)
}
func _1793020000000NetworkFabric(db *gorm.DB, version string) error {
	if err := seedNetworkFabric(db); err != nil {
		return err
	}
	return db.Create(&common.Migration{Version: version}).Error
}

func seedNetworkFabric(db *gorm.DB) error {
	if err := db.AutoMigrate(&models.Fabric{}, &models.RuleSet{}, &models.Membership{}, &models.Observation{}, &models.Assessment{}, &models.Collection{}, &models.Change{}, &delivery.Order{}, &delivery.Target{}); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := advanceNetworkSeedSequences(tx); err != nil {
			return err
		}
		regions := []struct{ code, name string }{{"SZX9", "传音大厦"}, {"SZX10", "深圳云科技"}, {"SZX8", "深圳百旺信"}, {"SZX11", "深圳传意园"}, {"SHA", "上海职场"}, {"CKG", "重庆职场"}}
		rules := []models.Rule{}
		for _, region := range regions {
			row := models.Fabric{Code: region.code, Name: region.name, Version: 1, RulesVersion: 1, RuntimeMinutes: 5, ConfigMinutes: 30}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
			if err := tx.Where("code = ?", region.code).First(&row).Error; err != nil {
				return err
			}
			rules = append(rules, models.Rule{FabricID: row.ID, Pattern: "(?i)(?:^|[-_.])" + region.code + "(?:[-_.]|$)", Enabled: true})
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.RuleSet{ID: 1, Version: 1, Rules: rules}).Error; err != nil {
			return err
		}
		return seedFabricPermissions(tx)
	})
}

func seedFabricPermissions(tx *gorm.DB) error {
	type route struct{ code, path, method, handler string }
	routes := []route{
		{"list", "", "GET", "Page"}, {"query", "/:id", "GET", "Get"},
		{"rules", "/rules", "GET", "Rules"}, {"rules-preview", "/rules/preview", "POST", "PreviewRules"}, {"rules-save", "/rules", "PUT", "SaveRules"},
		{"members", "/members", "GET", "Members"}, {"member-save", "/members/:deviceId", "PUT", "SetMember"},
		{"settings", "/:id/settings", "PUT", "Settings"}, {"endpoints", "/endpoints", "GET", "Endpoints"},
		{"nodes", "/:id/nodes", "GET", "Nodes"}, {"segments", "/:id/segments", "GET", "Segments"}, {"vpc-groups", "/:id/vpc-groups", "GET", "VPCGroups"},
		{"findings", "/:id/findings", "GET", "Findings"}, {"assessments", "/:id/assessments", "GET", "Assessments"},
		{"collections", "/:id/collections", "GET", "Collections"}, {"collect", "/:id/collections", "POST", "CreateCollection"}, {"collection", "/collections/:collectionId", "GET", "Collection"},
		{"changes", "/:id/changes", "GET", "Changes"}, {"preview", "/:id/changes", "POST", "PreviewChange"}, {"change", "/changes/:changeId", "GET", "Change"},
		{"candidates", "/:id/change-candidates", "POST", "ChangeCandidates"},
		{"execute", "/changes/:changeId/execute", "POST", "ExecuteChange"}, {"cancel", "/changes/:changeId/cancel", "POST", "CancelChange"},
	}
	apis := make([]seed.APISpec, 0, len(routes))
	for _, r := range routes {
		apis = append(apis, seed.APISpec{Code: "fabric-" + r.code, Title: "Fabric " + r.code, Path: "/api/v1/network-fabrics" + r.path, Method: r.method, Handle: "go-admin/app/fabric/apis.Fabric." + r.handler + "-fm"})
	}
	menus := []seed.MenuSpec{
		{Code: "management", Kind: contractmodels.Directory, Title: "网络管理", Path: "/network", Component: "Layout", Icon: "branch", Sort: 30},
		{Code: "fabric-manage", Parent: "management", Kind: contractmodels.Menu, Title: "Fabric 管理", Path: "/network/fabric", Component: "/network/fabric/index", Icon: "branch", Sort: 5, APICodes: []string{"fabric-list", "fabric-endpoints"}},
	}
	read := []string{"fabric-list", "fabric-query", "fabric-nodes", "fabric-segments", "fabric-vpc-groups", "fabric-findings", "fabric-assessments", "fabric-endpoints", "fabric-collections", "fabric-collection", "fabric-changes", "fabric-change"}
	operations := []struct {
		action, title string
		apis          []string
	}{
		{"list", "查询 Fabric", []string{"fabric-list", "fabric-endpoints"}},
		{"query", "查看 Fabric", read},
		{"edit", "维护归属与采集设置", append(append([]string{}, read...), "fabric-rules", "fabric-rules-preview", "fabric-rules-save", "fabric-members", "fabric-member-save", "fabric-settings")},
		{"collect", "采集 Fabric", append(append([]string{}, read...), "fabric-collect")},
		{"preview", "预览 Fabric 变更", append(append([]string{}, read...), "fabric-preview", "fabric-candidates")},
		{"execute", "执行 Fabric 变更", append(append([]string{}, read...), "fabric-execute", "fabric-cancel")},
	}
	for i, o := range operations {
		menus = append(menus, seed.MenuSpec{Code: "fabric-" + o.action, Parent: "fabric-manage", Kind: contractmodels.Button, Title: o.title, Permission: "network:fabric:" + o.action, Sort: i + 1, APICodes: o.apis})
	}
	return seed.Menus(tx, "network", menus, apis)
}
