package router

import (
	"strings"

	"github.com/gin-gonic/gin"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"github.com/go-admin-team/go-admin-core/v2/response"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
)

// Match the registered route rather than parameter expansion, which could
// otherwise let a /:id grant authorize /assign-group, /export or /options.
const wirelessRouteMatcher = `r.sub == p.sub && (r.obj == p.obj || keyMatch(r.obj, p.obj)) && (r.act == p.act || p.act == "*")`

func routePermission() gin.HandlerFunc {
	return func(c *gin.Context) {
		deny := func() {
			response.Error(c, 403, nil, "对不起，您没有该接口访问权限，请联系管理员")
			c.Abort()
		}
		route := c.FullPath()
		valid := false
		for _, prefix := range []string{"/api/v1/network-wireless"} {
			if route == prefix || strings.HasPrefix(route, prefix+"/") {
				valid = true
				break
			}
		}
		if !valid {
			deny()
			return
		}
		data, exists := c.Get(jwt.JwtPayloadKey)
		claims, ok := data.(jwt.MapClaims)
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
		allowed, err := enforcer.EnforceWithMatcher(wirelessRouteMatcher, role, route, c.Request.Method)
		if err != nil {
			response.Error(c, 500, nil, "Wireless 权限校验失败，请稍后重试")
			c.Abort()
			return
		}
		if !allowed {
			deny()
			return
		}
		c.Next()
	}
}
