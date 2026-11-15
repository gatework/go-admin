package router

import (
	"strings"

	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/response"
	"github.com/gatework/motor-core/sdk"
)

// The shared model treats a :parameter as any path segment. Circuit detail
// and static operations share a prefix, so that would let a detail grant
// authorize export/options too. Match the registered route identity instead,
// preserving direct role equality and deliberately granted '*' paths/methods.
const circuitRouteMatcher = `r.sub == p.sub && (r.obj == p.obj || keyMatch(r.obj, p.obj)) && (r.act == p.act || p.act == "*")`

func circuitRoutePermission() gin.HandlerFunc {
	return func(c *gin.Context) {
		deny := func() {
			response.Error(c, 403, nil, "对不起，您没有该接口访问权限，请联系管理员")
			c.Abort()
		}
		route := c.FullPath()
		if route != "/api/v1/network-circuit" && !strings.HasPrefix(route, "/api/v1/network-circuit/") {
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
		enforcer := sdk.Runtime.Casbin(c.Request.Host)
		if enforcer == nil {
			deny()
			return
		}
		allowed, err := enforcer.EnforceWithMatcher(circuitRouteMatcher, role, route, c.Request.Method)
		if err != nil {
			response.Error(c, 500, nil, "专线权限校验失败，请稍后重试")
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
