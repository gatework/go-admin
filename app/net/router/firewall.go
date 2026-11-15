package router

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor/common/account"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/response"
	"github.com/gatework/motor-core/sdk"
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

// Match registered route templates; a parameter grant must not become a different capability.
func firewallRoutePermission() gin.HandlerFunc {
	return func(c *gin.Context) {
		deny := func() { response.Error(c, 403, nil, "firewall.permission_denied"); c.Abort() }
		raw, exists := c.Get(jwt.JwtPayloadKey)
		claims, ok := raw.(jwt.MapClaims)
		if !exists || !ok {
			deny()
			return
		}
		role, ok := claims["rolekey"].(string)
		if !ok || role == "" {
			deny()
			return
		}
		if account.IsAdmin(role) {
			c.Next()
			return
		}
		enforcer := sdk.Runtime.Casbin(c.Request.Host)
		if enforcer == nil {
			deny()
			return
		}
		allowed, e := enforcer.EnforceWithMatcher(`r.sub == p.sub && (r.obj == p.obj || keyMatch(r.obj, p.obj)) && (r.act == p.act || p.act == "*")`, role, c.FullPath(), c.Request.Method)
		if e != nil || !allowed {
			deny()
			return
		}
		c.Next()
	}
}
