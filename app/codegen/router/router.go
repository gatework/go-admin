package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	log "github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"

	"github.com/gatework/motor/common/middleware"
)

// routerCheckRole collects the generator's authenticated route registrations.
var routerCheckRole = make([]func(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware), 0)

// InitRouter mounts the generator's routes on the host engine.
func InitRouter() {
	r, ok := sdk.Runtime.Engine().(*gin.Engine)
	if !ok || r == nil {
		log.NewHelper(log.Default()).Error("router: Gin engine is required")
		return
	}
	authMiddleware := middleware.GetAuthMiddleware()
	v1 := r.Group("/api/v1")
	for _, register := range routerCheckRole {
		register(v1, authMiddleware)
	}
}
