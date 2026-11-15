package router

import (
	"github.com/gatework/motor/app/admin/handler"
	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
	"github.com/gatework/motor/common/actions"
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
	// Configuration is system-wide, so its CRUD is not data-scoped.
	r := v1.Group("/config").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole()).Use(actions.Unscoped)
	{
		r.GET("", actions.Index[models.SysConfig, dto.ConfigSearch]())
		r.GET("/:id", actions.View[models.SysConfig, dto.ConfigByID]())
		r.POST("", actions.Create[models.SysConfig, dto.ConfigControl]())
		r.PUT("/:id", actions.Update[models.SysConfig, dto.ConfigControl]())
		r.DELETE("", actions.Delete[models.SysConfig, dto.ConfigByID]())
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
