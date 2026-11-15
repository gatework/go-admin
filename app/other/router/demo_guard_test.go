package router

import (
	"testing"

	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/sdk/config"

	"github.com/gatework/motor/internal/testconfig"
)

// registeredRoutes builds the generator's routes on an engine of its own and
// reports the patterns they were registered under.
//
// The mode has to be given rather than inherited, because it now decides what
// gets registered: a test that leaves it at the zero value would be asking
// about a mode no deployment runs in, and would pass whether or not the gate
// works.
//
// It is put back before this returns, not at the end of the test. t.Cleanup
// would leave the mode set for everything the caller does afterwards, so a
// caller that went on to assert something mode-dependent would be reading a
// value this helper left behind rather than one it chose. A caller that does
// want the mode set has to set it, which is visible where it happens.
//
// The JWT middleware is a zero value. MiddlewareFunc only closes over the
// receiver and is never called here - no request is served, the engine is
// asked what it has - so nothing dereferences it.
func registeredRoutes(t *testing.T, mode string) map[string]bool {
	t.Helper()
	gin.SetMode(gin.TestMode)

	restore := testconfig.Swap(t, func(settings *config.Config) {
		settings.
			Application.Mode = mode
	})

	defer restore()
	r := gin.New()
	v1 := r.Group("/api/v1")
	sysNoCheckRoleRouter(v1, &jwt.GinJWTMiddleware{})
	registerDBRouter(v1, &jwt.GinJWTMiddleware{})

	out := map[string]bool{}
	for _, route := range r.Routes() {
		out[route.Path] = true
	}
	return out
}
