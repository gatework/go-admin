package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/jobs/handler"
	"github.com/gatework/motor/app/jobs/models"
	"github.com/gatework/motor/app/jobs/service/dto"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/middleware"
)

//nolint:gochecknoinits // Host startup collects router registrations before serving requests.
func init() {
	routerCheckRole = append(routerCheckRole, registerSysJobRouter)
}

// 需认证的路由代码
func registerSysJobRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {

	r := v1.Group("/sysjob").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	{
		r.GET("", actions.PermissionAction(), actions.Index[models.SysJob, dto.JobSearch]())
		r.GET("/:id", actions.PermissionAction(), actions.View[models.SysJob, dto.JobByID]())
		r.POST("", handler.ReconcileWrites, actions.Create[models.SysJob, dto.JobControl]())
		r.PUT("", actions.PermissionAction(), handler.ReconcileWrites, actions.Update[models.SysJob, dto.JobControl]())
		r.DELETE("", actions.PermissionAction(), handler.ReconcileWrites, actions.Delete[models.SysJob, dto.JobByID]())
	}
	sysJob := handler.Job{}

	r2 := v1.Group("/job").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	{
		// POST: both change state. They answered GET, which caches, prefetches
		// and crosses sites as a plain link.
		r2.POST("/remove/:id", sysJob.Stop)
		r2.POST("/start/:id", sysJob.Start)
	}
}
