package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/admin/handler"
	"github.com/gatework/motor/app/admin/service"
)

//nolint:gochecknoinits // The host seals startup registries after these module declarations are loaded.
func init() { routerCheckRole = append(routerCheckRole, registerAccountRouter) }
func registerAccountRouter(v1 *gin.RouterGroup, auth *jwt.GinJWTMiddleware) {
	public := v1.Group("/account")
	public.GET("/settings", handler.AccountPublic("settings", service.AccountSettings))
	public.POST("/registration/email", handler.AccountPublic("registration-email", service.SendRegistrationEmail))
	public.POST("/register", handler.AccountPublic("register", service.Register))
	public.POST("/password/forgot", handler.AccountPublic("forgot", service.SendRecoveryEmail))
	public.POST("/password/reset", handler.AccountPublic("reset", service.ResetPassword))
	self := public.Group("/mfa").Use(auth.MiddlewareFunc())
	self.GET("", handler.AccountMFA("status", service.MFAStatus))
	self.POST("/setup", handler.AccountMFA("setup", service.SetupMFA))
	self.POST("/cancel", handler.AccountMFA("cancel", service.CancelMFASetup))
	self.POST("/enable", handler.AccountMFA("enable", service.EnableMFA))
	self.POST("/disable", handler.AccountMFA("disable", service.DisableMFA))
	self.POST("/regenerate", handler.AccountMFA("regenerate", service.RegenerateRecoveryCodes))
	policy := public.Group("/registration-policy").Use(auth.MiddlewareFunc())
	policy.GET("", handler.AccountPolicy("policy-get", service.AccountSettings))
	policy.PUT("", handler.AccountPolicy("policy-set", service.SetRegistrationPolicy))
}
