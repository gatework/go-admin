package router

import (
	"context"
	"github.com/gin-gonic/gin"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	log "github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"go-admin/app/diagnostics/apis"
	"go-admin/app/diagnostics/service"
	"go-admin/common/middleware"
)

func InitRouter() {
	engine, ok := sdk.Runtime.GetEngine().(*gin.Engine)
	if !ok {
		log.Fatal("network diagnostics requires Gin")
		return
	}
	manager := service.NewManager()
	sdk.Runtime.SetShutdown(func(context.Context) { manager.Close() })
	Register(engine, middleware.GetAuthMiddleware(), manager)
}
func Register(engine *gin.Engine, auth *jwt.GinJWTMiddleware, manager *service.Manager) {
	api := apis.Diagnostics{Manager: manager}
	routes := engine.Group("/api/v1/network-diagnostics").Use(auth.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	routes.GET("/server", api.Server)
	routes.POST("/jobs", api.Start)
	routes.GET("/jobs/:id", api.Get)
	routes.DELETE("/jobs/:id", api.Stop)
}
