package router

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/response"
	"github.com/gatework/motor-core/sdk"

	"github.com/gatework/motor/common/account"
)

// routeMatcher matches the registered route template rather than the expanded
// request path: with expansion, a grant on /:id would also authorize sibling
// routes such as /export or /options. Direct role equality and deliberately
// granted '*' methods are preserved.
const routeMatcher = `r.sub == p.sub && (r.obj == p.obj || keyMatch(r.obj, p.obj)) && (r.act == p.act || p.act == "*")`

// routeRule is one capability's route permission: the route prefixes it may
// guard and the messages its callers expect.
type routeRule struct {
	prefixes       []string
	deniedMessage  string
	failureMessage string // empty: an enforcer error is answered as a denial
}

const defaultDeniedMessage = "对不起，您没有该接口访问权限，请联系管理员"

// routePermission checks the caller's role against Casbin by registered route.
// A route outside the rule's prefixes, a missing or empty role, or a tenant
// without an enforcer is denied; an enforcer error answers 500 with the rule's
// failure message, or is denied when the rule has none.
func routePermission(rule routeRule) gin.HandlerFunc {
	return func(c *gin.Context) {
		deny := func() {
			response.Error(c, http.StatusForbidden, nil, rule.deniedMessage)
			c.Abort()
		}
		route := c.FullPath()
		if !underAnyPrefix(route, rule.prefixes) {
			deny()
			return
		}
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
		allowed, err := enforcer.EnforceWithMatcher(routeMatcher, role, route, c.Request.Method)
		if err != nil && rule.failureMessage != "" {
			response.Error(c, http.StatusInternalServerError, nil, rule.failureMessage)
			c.Abort()
			return
		}
		if err != nil || !allowed {
			deny()
			return
		}
		c.Next()
	}
}

func underAnyPrefix(route string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if route == prefix || strings.HasPrefix(route, prefix+"/") {
			return true
		}
	}
	return false
}

func wirelessRoutePermission() gin.HandlerFunc {
	return routePermission(routeRule{[]string{"/api/v1/network-wireless"}, defaultDeniedMessage, "Wireless 权限校验失败，请稍后重试"})
}

func fabricRoutePermission() gin.HandlerFunc {
	return routePermission(routeRule{[]string{"/api/v1/network-fabrics"}, defaultDeniedMessage, "Fabric 权限校验失败，请稍后重试"})
}

func deviceRoutePermission() gin.HandlerFunc {
	return routePermission(routeRule{[]string{"/api/v1/network-device", "/api/v1/network-device-group"}, defaultDeniedMessage, "设备权限校验失败，请稍后重试"})
}

func circuitRoutePermission() gin.HandlerFunc {
	return routePermission(routeRule{[]string{"/api/v1/network-circuit"}, defaultDeniedMessage, "专线权限校验失败，请稍后重试"})
}

func firewallRoutePermission() gin.HandlerFunc {
	return routePermission(routeRule{[]string{"/api/v1/network-firewall"}, "firewall.permission_denied", ""})
}
