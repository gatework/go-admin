package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/runtime"
	"github.com/gatework/motor/common/actions"
)

// These alias core's own constants (see sdk/runtime.GetHandlerFunc's contract
// doc, section 9) rather than redeclaring the same three strings, so a typo
// here can no longer split registration and lookup into two different keys
// that both happen to compile.
const (
	JwtTokenCheck   = runtime.JwtTokenCheck
	RoleCheck       = runtime.RoleCheck
	PermissionCheck = runtime.PermissionCheck
)

// InitMiddlewareWithDatabase lets the host exempt process-level probes from
// tenant database selection without coupling common to application routes.
func InitMiddlewareWithDatabase(r *gin.Engine, database gin.HandlerFunc) error {
	r.Use(DemoEnv())
	// CORS preflight is answered before the tenant database is resolved: a
	// preflight carries no credentials and needs no database, and answering it
	// after the lookup turned every preflight to an unknown host into a 503.
	r.Use(CORS)
	// 数据库链接
	r.Use(database)
	// 日志处理
	r.Use(AccessLog())
	// NoCache is a middleware function that appends headers
	r.Use(NoCache)
	// Secure is a middleware function that appends security
	r.Use(Secure)

	// Build the shared JWT middleware instance here, before any module
	// registers routes (initRouter runs ahead of runStartupHooks, which is
	// what invokes each module's InitRouter - see cmd/api/server.go). Doing
	// it once here, instead of once per module via AuthInit, is what makes
	// GetAuthMiddleware and sdk.Runtime.GetHandlerFunc(JwtTokenCheck) both
	// resolve to a single, meaningful instance instead of "whichever module
	// happened to initialize last".
	//
	// SetMiddleware must be given a bound closure (authMiddleware.MiddlewareFunc()),
	// not the unbound method expression (*jwt.GinJWTMiddleware).MiddlewareFunc:
	// the latter has no receiver bound to it, so GetHandlerFunc's type
	// assertion to gin.HandlerFunc always fails for it.
	var err error
	authMiddleware, err = AuthInit()
	if err != nil {
		// A process with no JWT middleware must not start serving requests.
		return fmt.Errorf("jwt middleware: %w", err)
	}
	sdk.Runtime.SetMiddleware(JwtTokenCheck, authMiddleware.MiddlewareFunc())
	sdk.Runtime.SetMiddleware(RoleCheck, AuthCheckRole())
	sdk.Runtime.SetMiddleware(PermissionCheck, actions.PermissionAction())
	return nil
}
