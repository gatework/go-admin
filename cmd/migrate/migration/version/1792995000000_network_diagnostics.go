package version

import (
	"runtime"

	"gorm.io/gorm"

	contractmodels "github.com/gatework/motor-core/sdk/contract/models"
	"github.com/gatework/motor-core/sdk/contract/seed"

	// Registers the host menu seeder before migrations run.
	_ "github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/cmd/migrate/migration"
	common "github.com/gatework/motor/common/models"
)

//nolint:gochecknoinits // Frozen migration registration must complete before the host seals its startup registry.
func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), _1792995000000NetworkDiagnostics)
}
func _1792995000000NetworkDiagnostics(db *gorm.DB, version string) error {
	if err := seedNetworkDiagnostics(db); err != nil {
		return err
	}
	return db.Create(&common.Migration{Version: version}).Error
}
func seedNetworkDiagnostics(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := advanceNetworkSeedSequences(tx); err != nil {
			return err
		}
		return seed.Menus(tx, "network", []seed.MenuSpec{
			{Code: "management", Kind: contractmodels.Directory, Title: "网络管理", Path: "/network", Component: "Layout", Icon: "branch", Sort: 30},
			{Code: "diagnostics", Parent: "management", Kind: contractmodels.Menu, Title: "网络检查", Path: "/network/diagnostics", Component: "/network/diagnostics/index", Icon: "code", Sort: 2, APICodes: []string{"diagnostics-server", "diagnostics-get"}},
			{Code: "diagnostics-run", Parent: "diagnostics", Kind: contractmodels.Button, Title: "执行网络检查", Permission: "network:diagnostics:run", Sort: 1, APICodes: []string{"diagnostics-start", "diagnostics-stop"}},
		}, []seed.APISpec{
			{Code: "diagnostics-server", Title: "网络检查服务器", Path: "/api/v1/network-diagnostics/server", Method: "GET", Handle: "go-admin/app/diagnostics/apis.Diagnostics.Server-fm"},
			{Code: "diagnostics-start", Title: "开始网络检查", Path: "/api/v1/network-diagnostics/jobs", Method: "POST", Handle: "go-admin/app/diagnostics/apis.Diagnostics.Start-fm"},
			{Code: "diagnostics-get", Title: "网络检查输出", Path: "/api/v1/network-diagnostics/jobs/:id", Method: "GET", Handle: "go-admin/app/diagnostics/apis.Diagnostics.Get-fm"},
			{Code: "diagnostics-stop", Title: "停止网络检查", Path: "/api/v1/network-diagnostics/jobs/:id", Method: "DELETE", Handle: "go-admin/app/diagnostics/apis.Diagnostics.Stop-fm"},
		})
	})
}
