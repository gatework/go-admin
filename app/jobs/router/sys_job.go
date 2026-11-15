package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/jobs/handler"
	models2 "github.com/gatework/motor/app/jobs/models"
	dto2 "github.com/gatework/motor/app/jobs/service/dto"
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
		r.GET("", actions.PermissionAction(), actions.Index[models2.SysJob, dto2.SysJobSearch]())
		// The detail answers SysJobItem, whose entryId the edit form reads;
		// the model itself marshals it as entry_id.
		r.GET("/:id", actions.PermissionAction(), actions.ViewAs[models2.SysJob, dto2.SysJobByID, dto2.SysJobItem]())
		r.POST("", handler.ReconcileWrites, actions.Create[models2.SysJob, dto2.SysJobControl]())
		r.PUT("", actions.PermissionAction(), handler.ReconcileWrites, actions.Update[models2.SysJob, dto2.SysJobControl]())
		r.DELETE("", actions.PermissionAction(), handler.ReconcileWrites, actions.Delete[models2.SysJob, dto2.SysJobByID]())
	}
	sysJob := handler.Job{}

	r2 := v1.Group("/job").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	{
		r2.GET("/remove/:id", sysJob.Stop)
		r2.GET("/start/:id", sysJob.Start)
	}
}
