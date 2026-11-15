package router

import (
	"context"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor/app/net/service/diagnostics"
	"github.com/gatework/motor/common/middleware"
)

// InitRouter mounts the network routes and registers diagnostics cleanup.
func InitRouter() {
	engine, ok := sdk.Runtime.Engine().(*gin.Engine)
	if !ok || engine == nil {
		panic("network router: host engine must be initialized as a Gin engine")
	}
	auth := middleware.GetAuthMiddleware()
	manager := diagnostics.NewManager()
	sdk.Runtime.SetShutdown(func(context.Context) { manager.Close() })

	RegisterInventory(engine, auth)
	RegisterFabric(engine, auth)
	RegisterWireless(engine, auth)
	RegisterFirewall(engine, auth)
	RegisterDelivery(engine, auth)
	RegisterDiagnostics(engine, auth, manager)
}
