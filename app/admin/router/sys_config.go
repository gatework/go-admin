package router

import (
	"github.com/gatework/motor/app/admin/handler"
	"github.com/gatework/motor/common/middleware"

	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
)

//nolint:gochecknoinits // The host seals startup registries after these module declarations are loaded.
func init() {
	routerCheckRole = append(routerCheckRole, registerSysConfigRouter)
}

// 需认证的路由代码
func registerSysConfigRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	h := handler.Config{}
	r := v1.Group("/config").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	{
		r.GET("", h.List)
		r.GET("/:id", h.Get)
		r.POST("", h.Create)
		r.PUT("/:id", h.Update)
		r.DELETE("", h.Delete)
	}

	r1 := v1.Group("/configKey").Use(authMiddleware.MiddlewareFunc())
	{
		r1.GET("/:configKey", h.GetByKey)
	}

	r2 := v1.Group("/app-config")
	{
		r2.GET("", h.AppConfig)
	}

	r3 := v1.Group("/set-config").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	{
		r3.PUT("", h.UpdateSettings)
		r3.GET("", h.Settings)
	}

}
