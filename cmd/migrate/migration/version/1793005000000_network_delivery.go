package version

import (
	contractmodels "github.com/go-admin-team/go-admin-core/v2/sdk/contract/models"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/seed"
	_ "go-admin/app/admin/service"
	"go-admin/app/delivery/models"
	"go-admin/cmd/migrate/migration"
	common "go-admin/common/models"
	"gorm.io/gorm"
	"runtime"
)

func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), _1793005000000NetworkDelivery)
}
func _1793005000000NetworkDelivery(db *gorm.DB, version string) error {
	if e := seedNetworkDelivery(db); e != nil {
		return e
	}
	return db.Create(&common.Migration{Version: version}).Error
}
func seedNetworkDelivery(db *gorm.DB) error {
	if e := db.AutoMigrate(&models.Connection{}, &models.Order{}, &models.Target{}, &models.DeviceLease{}, &models.Receipt{}, &models.Audit{}); e != nil {
		return e
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if e := advanceNetworkSeedSequences(tx); e != nil {
			return e
		}
		menus := []seed.MenuSpec{{Code: "management", Kind: contractmodels.Directory, Title: "网络管理", Path: "/network", Component: "Layout", Icon: "branch", Sort: 30}, {Code: "delivery-manage", Parent: "management", Kind: contractmodels.Menu, Title: "配置下发", Path: "/network/delivery", Component: "/network/delivery/index", Icon: "send", Sort: 4, ApiCodes: []string{"delivery-list", "delivery-options"}}}
		type operation struct {
			code, title, path, method, handler string
			dependencies                       []string
		}
		ops := []operation{
			{"list", "查询下发工单", "/orders", "GET", "Page", []string{"delivery-options"}},
			{"query", "查看下发工单", "/orders/:id", "GET", "Get", []string{"delivery-target", "delivery-options"}},
			{"add", "新建下发工单", "/orders", "POST", "Create", []string{"delivery-options", "delivery-devices", "delivery-connection-get", "delivery-query", "delivery-target"}},
			{"edit", "编辑下发工单", "/orders/:id", "PUT", "Edit", []string{"delivery-options", "delivery-devices", "delivery-connection-get", "delivery-query", "delivery-target"}},
			{"execute", "立即执行", "/orders/:id/execute", "POST", "Execute", []string{"delivery-options", "delivery-query", "delivery-target"}},
			{"cancel", "取消下发", "/orders/:id/cancel", "POST", "Cancel", []string{"delivery-options", "delivery-query", "delivery-target"}},
			{"retry", "重试安全失败", "/orders/:id/retry", "POST", "Retry", []string{"delivery-options", "delivery-query", "delivery-target"}},
			{"connection", "配置设备登录", "/devices/:id/connection", "PUT", "SetConnection", []string{"delivery-options", "delivery-devices", "delivery-connection-get"}},
		}
		apis := []seed.ApiSpec{{Code: "delivery-options", Title: "下发选项", Path: "/api/v1/network-delivery/options", Method: "GET", Handle: "go-admin/app/delivery/apis.Delivery.Options-fm"}, {Code: "delivery-devices", Title: "下发设备", Path: "/api/v1/network-delivery/devices", Method: "GET", Handle: "go-admin/app/delivery/apis.Delivery.Devices-fm"}, {Code: "delivery-connection-get", Title: "设备登录配置", Path: "/api/v1/network-delivery/devices/:id/connection", Method: "GET", Handle: "go-admin/app/delivery/apis.Delivery.Connection-fm"}, {Code: "delivery-target", Title: "下发目标回显", Path: "/api/v1/network-delivery/orders/:id/targets/:targetId", Method: "GET", Handle: "go-admin/app/delivery/apis.Delivery.Target-fm"}}
		for i, o := range ops {
			code := "delivery-" + o.code
			menus = append(menus, seed.MenuSpec{Code: code, Parent: "delivery-manage", Kind: contractmodels.Button, Title: o.title, Permission: "network:delivery:" + o.code, Sort: i + 1, ApiCodes: append([]string{code}, o.dependencies...)})
			apis = append(apis, seed.ApiSpec{Code: code, Title: o.title, Path: "/api/v1/network-delivery" + o.path, Method: o.method, Handle: "go-admin/app/delivery/apis.Delivery." + o.handler + "-fm"})
		}
		return seed.SeedMenus(tx, "network", menus, apis)
	})
}
