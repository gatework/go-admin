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
	routerCheckRole = append(routerCheckRole, registerOperationLogRouter)
}

// registerOperationLogRouter mounts the audit log on the generic actions: read and delete only.
// Access is decided by route permission; the log is not data-scoped.
func registerOperationLogRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	r := v1.Group("/sys-opera-log").
		Use(authMiddleware.MiddlewareFunc()).
		Use(middleware.AuthCheckRole()).
		Use(actions.Unscoped)
	{
		r.GET("", actions.Index[models.SysOperaLog, dto.OperationLogSearch]())
		r.GET("/:id", actions.View[models.SysOperaLog, dto.OperationLogByID]())
		r.DELETE("", actions.Delete[models.SysOperaLog, dto.OperationLogByID]())
	}
}
