package version

import (
	contractmodels "github.com/go-admin-team/go-admin-core/v2/sdk/contract/models"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/seed"
	_ "go-admin/app/admin/service"
	"go-admin/cmd/migrate/migration"
	common "go-admin/common/models"
	"gorm.io/gorm"
	"runtime"
)

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
		return seed.SeedMenus(tx, "network", []seed.MenuSpec{
			{Code: "management", Kind: contractmodels.Directory, Title: "网络管理", Path: "/network", Component: "Layout", Icon: "branch", Sort: 30},
			{Code: "diagnostics", Parent: "management", Kind: contractmodels.Menu, Title: "网络检查", Path: "/network/diagnostics", Component: "/network/diagnostics/index", Icon: "code", Sort: 2, ApiCodes: []string{"diagnostics-server", "diagnostics-get"}},
			{Code: "diagnostics-run", Parent: "diagnostics", Kind: contractmodels.Button, Title: "执行网络检查", Permission: "network:diagnostics:run", Sort: 1, ApiCodes: []string{"diagnostics-start", "diagnostics-stop"}},
		}, []seed.ApiSpec{
			{Code: "diagnostics-server", Title: "网络检查服务器", Path: "/api/v1/network-diagnostics/server", Method: "GET", Handle: "go-admin/app/diagnostics/apis.Diagnostics.Server-fm"},
			{Code: "diagnostics-start", Title: "开始网络检查", Path: "/api/v1/network-diagnostics/jobs", Method: "POST", Handle: "go-admin/app/diagnostics/apis.Diagnostics.Start-fm"},
			{Code: "diagnostics-get", Title: "网络检查输出", Path: "/api/v1/network-diagnostics/jobs/:id", Method: "GET", Handle: "go-admin/app/diagnostics/apis.Diagnostics.Get-fm"},
			{Code: "diagnostics-stop", Title: "停止网络检查", Path: "/api/v1/network-diagnostics/jobs/:id", Method: "DELETE", Handle: "go-admin/app/diagnostics/apis.Diagnostics.Stop-fm"},
		})
	})
}
