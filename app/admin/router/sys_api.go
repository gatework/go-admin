package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"

	"github.com/gatework/motor/app/admin/handler"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/middleware"
)

//nolint:gochecknoinits // The host seals startup registries after these module declarations are loaded.
func init() {
	routerCheckRole = append(routerCheckRole, registerSysAPIRouter)
}

// registerSysAPIRouter
func registerSysAPIRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	h := handler.Route{}
	// PermissionAction is not optional here: all three handlers below read the
	// data permission out of the context, and without it they read the zero
	// value - an unset scope, which Permission now fails closed on.
	r := v1.Group("/sys-api").Use(authMiddleware.MiddlewareFunc()).Use(middleware.AuthCheckRole()).Use(actions.PermissionAction())
	{
		r.GET("", h.List)
		r.GET("/:id", h.Get)
		r.PUT("/:id", h.Update)
	}
}
