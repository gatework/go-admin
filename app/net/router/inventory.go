package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/net/handler"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/middleware"
)

// RegisterInventory mounts authenticated business operations on the host router.
func RegisterInventory(engine *gin.Engine, auth *jwt.GinJWTMiddleware) {
	registerDeviceRouter(engine.Group("/api/v1"), auth)
	registerDeviceGroupRouter(engine.Group("/api/v1"), auth)
	registerCircuitRouter(engine.Group("/api/v1"), auth)
}

func registerCircuitRouter(v1 *gin.RouterGroup, auth *jwt.GinJWTMiddleware) {
	h := handler.Circuit{}
	circuits := v1.Group("/network-circuit").Use(auth.MiddlewareFunc()).Use(circuitRoutePermission()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	circuits.GET("", h.List)
	circuits.GET("/options", h.Options)
	circuits.GET("/export", h.Export)
	circuits.GET("/zabbix/hosts", h.Hosts)
	circuits.GET("/zabbix/items", h.Items)
	circuits.POST("", h.Create)
	circuits.GET("/:id", h.Get)
	circuits.PUT("/:id", h.Update)
	circuits.DELETE("/:id", h.Delete)
	circuits.GET("/:id/traffic", h.Traffic)
}
func registerDeviceRouter(v1 *gin.RouterGroup, auth *jwt.GinJWTMiddleware) {
	h := handler.Device{}
	devices := v1.Group("/network-device").Use(auth.MiddlewareFunc()).Use(deviceRoutePermission()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	devices.GET("", h.List)
	devices.GET("/options", h.Options)
	devices.GET("/sync-status", h.SyncStatus)
	devices.GET("/export", h.Export)
	devices.POST("/sync", h.Sync)
	devices.PUT("/assign-group", h.AssignGroup)
	devices.GET("/:id", h.Get)
	devices.PUT("/:id", h.Update)
}
func registerDeviceGroupRouter(v1 *gin.RouterGroup, auth *jwt.GinJWTMiddleware) {
	h := handler.DeviceGroup{}
	groups := v1.Group("/network-device-group").Use(auth.MiddlewareFunc()).Use(deviceRoutePermission()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	groups.GET("", h.Tree)
	groups.POST("", h.Create)
	groups.PUT("/:id", h.Update)
	groups.DELETE("/:id", h.Delete)
}
