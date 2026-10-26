package router

import (
	"github.com/gin-gonic/gin"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"go-admin/app/admin/apis"
)

func init() { routerCheckRole = append(routerCheckRole, registerAccountRouter) }
func registerAccountRouter(v1 *gin.RouterGroup, auth *jwt.GinJWTMiddleware) {
	public := v1.Group("/account")
	public.GET("/settings", apis.AccountPublic("settings"))
	public.POST("/registration/email", apis.AccountPublic("registration-email"))
	public.POST("/register", apis.AccountPublic("register"))
	public.POST("/password/forgot", apis.AccountPublic("forgot"))
	public.POST("/password/reset", apis.AccountPublic("reset"))
	self := public.Group("/mfa").Use(auth.MiddlewareFunc())
	self.GET("", apis.AccountMFA("status"))
	for _, op := range []string{"setup", "cancel", "enable", "disable", "regenerate"} {
		self.POST("/"+op, apis.AccountMFA(op))
	}
	policy := public.Group("/registration-policy").Use(auth.MiddlewareFunc())
	policy.GET("", apis.AccountPolicy("policy-get"))
	policy.PUT("", apis.AccountPolicy("policy-set"))
}
