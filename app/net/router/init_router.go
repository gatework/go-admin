package router

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor/app/net/service/diagnostics"
	"github.com/gatework/motor/app/net/service/netconfig"
	"github.com/gatework/motor/common/middleware"
)

// InitRouter mounts the network routes, each with the configuration it
// needs, and registers diagnostics cleanup.
func InitRouter(cfg netconfig.Config) {
	engine, ok := sdk.Runtime.Engine().(*gin.Engine)
	if !ok || engine == nil {
		panic("network router: host engine must be initialized as a Gin engine")
	}
	auth := middleware.GetAuthMiddleware()
	manager := diagnostics.NewManager()
	sdk.Runtime.SetShutdown(func(context.Context) { manager.Close() })

	access := cfg.Delivery.Access
	RegisterInventory(engine, auth, cfg.Netdisco, cfg.Zabbix)
	RegisterFabric(engine, auth, access, cfg.Delivery.Enabled)
	RegisterWireless(engine, auth, access)
	RegisterFirewall(engine, auth, access)
	RegisterDelivery(engine, auth, cfg.Delivery)
	RegisterDiagnostics(engine, auth, manager)
}
