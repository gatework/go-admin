package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/admin/handler"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/middleware"
)

//nolint:gochecknoinits // The host seals startup registries after these module declarations are loaded.
func init() {
	routerCheckRole = append(routerCheckRole, registerSysUserRouter)
}

// 需认证的路由代码
func registerSysUserRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	h := handler.User{}
	r := v1.Group("/sys-user").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	{
		r.GET("", h.List)
		r.GET("/:id", h.Get)
		r.POST("", h.Create)
		r.PUT("", h.Update)
		r.DELETE("", h.Delete)
	}

	user := v1.Group("/user").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	{
		user.GET("/profile", h.Profile)
		user.POST("/avatar", h.UpdateAvatar)
		user.PUT("/pwd/set", h.ChangePassword)
		user.PUT("/pwd/reset", h.ResetPassword)
		user.PUT("/status", h.UpdateStatus)
	}
	v1auth := v1.Group("").Use(authMiddleware.MiddlewareFunc())
	{
		v1auth.GET("/getinfo", h.Current)
	}
}
