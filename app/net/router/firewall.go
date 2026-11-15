package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/net/handler"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/middleware"
)

// RegisterFirewall mounts firewall routes with authentication and data permissions.
func RegisterFirewall(engine *gin.Engine, auth *jwt.GinJWTMiddleware) {
	r := engine.Group("/api/v1/network-firewall").Use(auth.MiddlewareFunc()).Use(firewallRoutePermission()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	handlers := handler.Firewall{}.Handlers()
	for _, route := range FirewallRoutes {
		handler := handlers[route.Handler]
		if handler == nil {
			panic("firewall route has no handler: " + route.Handler)
		}
		r.Handle(route.Method, route.Path, handler)
	}
}
