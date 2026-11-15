package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor/app/net/handler"
	service "github.com/gatework/motor/app/net/service/diagnostics"
	"github.com/gatework/motor/common/middleware"
)

// RegisterDiagnostics mounts the module handlers with authentication and authorization.
func RegisterDiagnostics(engine *gin.Engine, auth *jwt.GinJWTMiddleware, manager *service.Manager) {
	h := handler.Diagnostics{Manager: manager}
	routes := engine.Group("/api/v1/network-diagnostics").Use(auth.MiddlewareFunc()).Use(middleware.AuthCheckRole())
	routes.GET("/server", h.Server)
	routes.POST("/jobs", h.Start)
	routes.GET("/jobs/:id", h.Get)
	routes.DELETE("/jobs/:id", h.Stop)
}
