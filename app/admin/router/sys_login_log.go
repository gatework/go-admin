package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/admin/handler"
	"github.com/gatework/motor/common/middleware"
)

//nolint:gochecknoinits // The host seals startup registries after these module declarations are loaded.
func init() {
	routerCheckRole = append(routerCheckRole, registerSysLoginLogRouter)
}

// 需认证的路由代码
func registerSysLoginLogRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	h := handler.LoginLog{}

	r := v1.Group("/sys-login-log").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	{
		r.GET("", h.List)
		r.GET("/:id", h.Get)
		r.DELETE("", h.Delete)
	}
}
