package router

import (
	"github.com/gin-gonic/gin"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"github.com/go-admin-team/go-admin-core/v2/response"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"go-admin/app/firewall"
	"go-admin/app/firewall/apis"
	"go-admin/common/actions"
	"go-admin/common/middleware"
)

func InitRouter() {
	engine, ok := sdk.Runtime.GetEngine().(*gin.Engine)
	if ok {
		InitBusinessRouter(engine, middleware.GetAuthMiddleware())
	}
}
func InitBusinessRouter(engine *gin.Engine, auth *jwt.GinJWTMiddleware) {
	r := engine.Group("/api/v1/network-firewall").Use(auth.MiddlewareFunc()).Use(routePermission()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	handlers := (apis.Firewall{}).Handlers()
	for _, route := range firewall.Routes {
		handler := handlers[route.Handler]
		if handler == nil {
			panic("firewall route has no handler: " + route.Handler)
		}
		r.Handle(route.Method, route.Path, handler)
	}
}

// Match registered route templates; a parameter grant must not become a different capability.
func routePermission() gin.HandlerFunc {
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
		if role == "admin" {
			c.Next()
			return
		}
		enforcer := sdk.Runtime.GetCasbinByTenant(c.Request.Host)
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
