package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/net/handler"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/middleware"
)

// RegisterDelivery installs the module's routes and permission middleware.
func RegisterDelivery(engine *gin.Engine, auth *jwt.GinJWTMiddleware) {
	a := handler.Delivery{}
	r := engine.Group("/api/v1/network-delivery").Use(auth.MiddlewareFunc()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	r.GET("/options", a.Options)
	r.GET("/devices", a.Devices)
	r.GET("/devices/:id/connection", a.Connection)
	r.PUT("/devices/:id/connection", a.SetConnection)
	r.GET("/orders", a.List)
	r.POST("/orders", a.Create)
	r.GET("/orders/:id", a.Get)
	r.PUT("/orders/:id", a.Edit)
	r.POST("/orders/:id/execute", a.Execute)
	r.POST("/orders/:id/cancel", a.Cancel)
	r.POST("/orders/:id/retry", a.Retry)
	r.GET("/orders/:id/targets/:targetId", a.Target)
}
