package router

import (
	"context"
	"github.com/gin-gonic/gin"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	base "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"go-admin/app/delivery/apis"
	"go-admin/app/delivery/service"
	"go-admin/common/actions"
	"go-admin/common/middleware"
	"gorm.io/gorm"
)

func InitRouter() {
	engine, ok := sdk.Runtime.GetEngine().(*gin.Engine)
	if !ok {
		return
	}
	Register(engine, middleware.GetAuthMiddleware())
	seen := map[*gorm.DB]bool{}
	for _, db := range sdk.Runtime.GetAllDb() {
		if db == nil || seen[db] {
			continue
		}
		seen[db] = true
		worker := service.NewWorker(&service.Workflow{Service: base.Service{Orm: db}, Config: service.FromEnvironment()}, nil)
		worker.Start()
		sdk.Runtime.SetShutdown(func(ctx context.Context) { worker.Close(ctx) })
	}
}
func Register(engine *gin.Engine, auth *jwt.GinJWTMiddleware) {
	a := apis.Delivery{}
	r := engine.Group("/api/v1/network-delivery").Use(auth.MiddlewareFunc()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	r.GET("/options", a.Options)
	r.GET("/devices", a.Devices)
	r.GET("/devices/:id/connection", a.Connection)
	r.PUT("/devices/:id/connection", a.SetConnection)
	r.GET("/orders", a.Page)
	r.POST("/orders", a.Create)
	r.GET("/orders/:id", a.Get)
	r.PUT("/orders/:id", a.Edit)
	r.POST("/orders/:id/execute", a.Execute)
	r.POST("/orders/:id/cancel", a.Cancel)
	r.POST("/orders/:id/retry", a.Retry)
	r.GET("/orders/:id/targets/:targetId", a.Target)
}
