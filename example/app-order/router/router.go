// Package router wires app-order's four routes onto a host's gin engine.
package router

import (
	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/contract/actions"
	coreruntime "github.com/gatework/motor-core/sdk/runtime"

	"github.com/gatework/motor/example/app-order/handler"
)

// RegisterRouter mounts application routes using the host authentication and role chain.
func RegisterRouter(v1 *gin.RouterGroup, authMiddleware *jwt.GinJWTMiddleware) {
	roleCheck, ok := sdk.Runtime.GetHandlerFunc(coreruntime.RoleCheck)
	if !ok {
		// A host that has not wired up RoleCheck has not wired up Casbin
		// authorization at all. Registering these routes without it would
		// silently serve every order to every authenticated caller
		// regardless of role - fail loud at startup instead, the same way
		// PermissionAction fails loud (Abort, not c.Next) when its own
		// database lookup errors. See contract/actions.PermissionAction's
		// doc comment for the same reasoning applied to data-scope instead
		// of role.
		panic("app-order: host has not registered core's " + coreruntime.RoleCheck +
			" middleware (sdk.Runtime.SetMiddleware); refusing to mount unauthorized order routes")
	}

	e := handler.Order{}
	r := v1.Group("/order").
		Use(authMiddleware.MiddlewareFunc()).
		Use(roleCheck)
	{
		// actions.PermissionAction is imported directly from core - a plain
		// function, not something fetched through sdk.Runtime - because
		// unlike RoleCheck's Casbin policy tables (host-owned; see
		// contract/actions's package doc), the data-scope machinery it
		// installs has no host-specific state at all.
		r.GET("", actions.PermissionAction(func() bool { return config.Current().Settings.Application.EnableDP }), e.List)
		r.GET("/:id", actions.PermissionAction(func() bool { return config.Current().Settings.Application.EnableDP }), e.Get)
		r.POST("", e.Create)
		r.PUT("/:id/pay", actions.PermissionAction(func() bool { return config.Current().Settings.Application.EnableDP }), e.Pay)
	}
}
