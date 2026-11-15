package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/admin/handler"
)

//nolint:gochecknoinits // The host seals startup registries after these module declarations are loaded.
func init() { routerCheckRole = append(routerCheckRole, registerAccountRouter) }
func registerAccountRouter(v1 *gin.RouterGroup, auth *jwt.GinJWTMiddleware) {
	public := v1.Group("/account")
	public.GET("/settings", handler.AccountPublic("settings"))
	public.POST("/registration/email", handler.AccountPublic("registration-email"))
	public.POST("/register", handler.AccountPublic("register"))
	public.POST("/password/forgot", handler.AccountPublic("forgot"))
	public.POST("/password/reset", handler.AccountPublic("reset"))
	self := public.Group("/mfa").Use(auth.MiddlewareFunc())
	self.GET("", handler.AccountMFA("status"))
	for _, op := range []string{"setup", "cancel", "enable", "disable", "regenerate"} {
		self.POST("/"+op, handler.AccountMFA(op))
	}
	policy := public.Group("/registration-policy").Use(auth.MiddlewareFunc())
	policy.GET("", handler.AccountPolicy("policy-get"))
	policy.PUT("", handler.AccountPolicy("policy-set"))
}
