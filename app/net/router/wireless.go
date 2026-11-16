package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/net/handler"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/middleware"
)

// RegisterWireless mounts authenticated business operations on the host router.
func RegisterWireless(engine *gin.Engine, auth *jwt.GinJWTMiddleware, access deviceaccess.Config) {
	a := handler.Wireless{Access: access}
	r := engine.Group("/api/v1/network-wireless").Use(auth.MiddlewareFunc()).Use(wirelessRoutePermission()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	r.GET("/controllers", a.Controllers)
	r.GET("/groups", a.Groups)
	r.GET("/switches", a.Switches)
	r.GET("/clients", a.Clients)
	r.POST("/imports/preview", a.PreviewImport)
	r.POST("/imports/commit", a.CommitImport)
	r.GET("/controllers/:controllerId", a.Controller)
	r.GET("/controllers/:controllerId/analysis", a.Analysis)
	r.GET("/controllers/:controllerId/access-points", a.AccessPoints)
	r.GET("/controllers/:controllerId/radios", a.Radios)
	r.GET("/controllers/:controllerId/bsses", a.BSSes)
	r.GET("/controllers/:controllerId/clients", a.ClientResources)
	r.GET("/controllers/:controllerId/access-points/:id", a.APDetail)
	r.GET("/controllers/:controllerId/clients/:id", a.Client)
	r.PUT("/controllers/:controllerId/access-points/:id/management", a.Management)
	r.PUT("/controllers/:controllerId/access-points/:id/neighbor", a.Associate)
	r.GET("/controllers/:controllerId/collections", a.Collections)
	r.POST("/controllers/:controllerId/collections", a.Collect)
	r.GET("/controllers/:controllerId/collections/:id", a.Collection)
	r.GET("/controllers/:controllerId/exports/:kind", a.Export)
}
