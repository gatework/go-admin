package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/middleware"
)

//nolint:gochecknoinits // The host seals startup registries after these module declarations are loaded.
func init() {
	routerCheckRole = append(routerCheckRole, registerLoginLogRouter)
}

// registerLoginLogRouter mounts the audit log on the generic actions: read and delete only.
// Access is decided by route permission; the log is not data-scoped.
func registerLoginLogRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	r := v1.Group("/sys-login-log").
		Use(authMiddleware.MiddlewareFunc()).
		Use(middleware.AuthCheckRole()).
		Use(actions.Unscoped)
	{
		r.GET("", actions.Index[models.SysLoginLog, dto.LoginLogSearch]())
		r.GET("/:id", actions.View[models.SysLoginLog, dto.LoginLogByID]())
		r.DELETE("", actions.Delete[models.SysLoginLog, dto.LoginLogByID]())
	}
}
