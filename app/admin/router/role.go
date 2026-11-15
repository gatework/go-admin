package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"

	"github.com/gatework/motor/app/admin/handler"
	"github.com/gatework/motor/common/middleware"
)

//nolint:gochecknoinits // The host seals startup registries after these module declarations are loaded.
func init() {
	routerCheckRole = append(routerCheckRole, registerSysRoleRouter)
}

// 需认证的路由代码
func registerSysRoleRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	h := handler.Role{}
	r := v1.Group("/role").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	{
		r.GET("", h.List)
		r.GET("/:id", h.Get)
		r.POST("", h.Create)
		r.PUT("/:id", h.Update)
		r.DELETE("", h.Delete)
	}
	r1 := v1.Group("").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	{
		r1.PUT("/role-status", h.UpdateStatus)
		r1.PUT("/roledatascope", h.UpdateDataScope)
	}
}
