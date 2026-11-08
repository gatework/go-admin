package router

import (
	"github.com/gin-gonic/gin"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	log "github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"go-admin/app/network/apis"
	"go-admin/common/actions"
	"go-admin/common/middleware"
)

func InitRouter() {
	engine, ok := sdk.Runtime.GetEngine().(*gin.Engine)
	if !ok {
		log.Fatal("network inventory requires the Gin engine")
		return
	}
	InitBusinessRouter(engine, middleware.GetAuthMiddleware())
}
func InitBusinessRouter(engine *gin.Engine, auth *jwt.GinJWTMiddleware) {
	registerDeviceRouter(engine.Group("/api/v1"), auth)
	registerDeviceGroupRouter(engine.Group("/api/v1"), auth)
	registerCircuitRouter(engine.Group("/api/v1"), auth)
}

func registerCircuitRouter(v1 *gin.RouterGroup, auth *jwt.GinJWTMiddleware) {
	api := apis.Circuit{}
	circuits := v1.Group("/network-circuit").Use(auth.MiddlewareFunc()).Use(circuitRoutePermission()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	circuits.GET("", api.GetPage)
	circuits.GET("/options", api.Options)
	circuits.GET("/export", api.Export)
	circuits.GET("/zabbix/hosts", api.Hosts)
	circuits.GET("/zabbix/items", api.Items)
	circuits.POST("", api.Create)
	circuits.GET("/:id", api.Get)
	circuits.PUT("/:id", api.Update)
	circuits.DELETE("/:id", api.Delete)
	circuits.GET("/:id/traffic", api.Traffic)
}
func registerDeviceRouter(v1 *gin.RouterGroup, auth *jwt.GinJWTMiddleware) {
	api := apis.Device{}
	devices := v1.Group("/network-device").Use(auth.MiddlewareFunc()).Use(deviceRoutePermission()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	devices.GET("", api.GetPage)
	devices.GET("/options", api.Options)
	devices.GET("/sync-status", api.SyncStatus)
	devices.GET("/export", api.Export)
	devices.POST("/sync", api.Sync)
	devices.PUT("/assign-group", api.AssignGroup)
	devices.GET("/:id", api.Get)
	devices.PUT("/:id", api.Update)
}
func registerDeviceGroupRouter(v1 *gin.RouterGroup, auth *jwt.GinJWTMiddleware) {
	api := apis.DeviceGroup{}
	groups := v1.Group("/network-device-group").Use(auth.MiddlewareFunc()).Use(deviceRoutePermission()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	groups.GET("", api.Tree)
	groups.POST("", api.Create)
	groups.PUT("/:id", api.Update)
	groups.DELETE("/:id", api.Delete)
}
