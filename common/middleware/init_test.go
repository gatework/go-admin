package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gatework/motor/internal/testconfig"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/runtime"
)

// freshRuntime hands the test its own Runtime and puts the old one back, the
// same pattern cmd/api/server_test.go uses: sdk.Runtime is a process-wide
// singleton, and a test that registers into it would otherwise leak state
// into every other test in the binary.
func freshRuntime(t *testing.T) {
	t.Helper()
	previous := sdk.Runtime
	t.Cleanup(func() { sdk.Runtime = previous })
	sdk.Runtime = runtime.New()
}

// TestInitMiddlewareRegistersUsableJwtHandlerFunc is the reverse proof for
// hoisting the JWT instance's construction into InitMiddleware:
// sdk.Runtime.GetHandlerFunc(JwtTokenCheck) must hand back ok=true and a
// non-nil gin.HandlerFunc, not just something GetMiddleware can return as an
// untyped interface{}.
//
// Before this change, InitMiddleware registered the unbound method
// expression (*jwt.GinJWTMiddleware).MiddlewareFunc under this key - a value
// with no receiver bound to it, which is not a gin.HandlerFunc no matter how
// a caller asserts its type. Reverting the registration below to that
// expression makes GetHandlerFunc report ok=false; it does not fail to
// compile, because (*jwt.GinJWTMiddleware).MiddlewareFunc has a well-formed,
// unrelated method-expression type that SetMiddleware's interface{} param
// happily accepts.
func TestInitMiddlewareRegistersUsableJwtHandlerFunc(t *testing.T) {
	freshRuntime(t)

	testconfig.Apply(t, func(s *config.Config) { s.JWT.Secret = "test-secret-key" })

	gin.SetMode(gin.TestMode)
	if err := InitMiddlewareWithDatabase(gin.New(), WithContextDB); err != nil {
		t.Fatal(err)
	}

	h, ok := sdk.Runtime.GetHandlerFunc(JwtTokenCheck)
	if !ok {
		t.Fatal("GetHandlerFunc(JwtTokenCheck) reported ok=false after InitMiddleware ran")
	}
	if h == nil {
		t.Fatal("GetHandlerFunc(JwtTokenCheck) reported ok=true but returned a nil handler")
	}
}

// TestInitMiddlewareBuildsOneSharedJwtInstance locks down the fix for the
// four-instances problem: GetAuthMiddleware must return the very instance
// InitMiddleware built and handed to sdk.Runtime, not a lookalike built
// separately by whichever caller asks first.
func TestInitMiddlewareBuildsOneSharedJwtInstance(t *testing.T) {
	freshRuntime(t)

	testconfig.Apply(t, func(s *config.Config) { s.JWT.Secret = "test-secret-key" })

	gin.SetMode(gin.TestMode)
	if err := InitMiddlewareWithDatabase(gin.New(), WithContextDB); err != nil {
		t.Fatal(err)
	}

	shared := GetAuthMiddleware()
	if shared == nil {
		t.Fatal("GetAuthMiddleware returned nil after InitMiddleware ran")
	}
	if shared != authMiddleware {
		t.Error("GetAuthMiddleware did not return the package-level instance InitMiddleware built")
	}
}

// A missing JWT secret is a configuration error: it is returned to the host,
// which reports it and exits through its own deferred cleanup. It used to end
// the process from inside the library with log.Fatal.
func TestInitMiddlewareReturnsAJWTConfigurationError(t *testing.T) {
	freshRuntime(t)
	testconfig.Apply(t, func(s *config.Config) { s.JWT.Secret = "" })
	gin.SetMode(gin.TestMode)
	if err := InitMiddlewareWithDatabase(gin.New(), WithContextDB); err == nil {
		t.Fatal("JWT setup without a secret reported success")
	}
}

func TestProductionRefusesAShortJWTSecret(t *testing.T) {
	if err := checkSecret("prod", "motor"); err == nil {
		t.Fatal("five-byte production secret accepted")
	}
	if err := checkSecret("prod", strings.Repeat("k", minProductionSecret)); err != nil {
		t.Fatalf("adequate production secret refused: %v", err)
	}
	if err := checkSecret("dev", "motor"); err != nil {
		t.Fatalf("development secret refused: %v", err)
	}
}

// Tokens carried in a URL end up in access and proxy logs. Only a websocket
// upgrade may present one there, and it is moved into the header and removed
// from the URL before the request is logged or authenticated.
func TestQueryTokensAreAcceptedOnlyForWebsocketUpgrades(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	var authorization, rawQuery string
	engine.GET("/ws", WebsocketToken, func(c *gin.Context) {
		authorization, rawQuery = c.GetHeader("Authorization"), c.Request.URL.RawQuery
	})
	request := httptest.NewRequest(http.MethodGet, "/ws?token=abc&channel=1", nil)
	request.Header.Set("Upgrade", "websocket")
	engine.ServeHTTP(httptest.NewRecorder(), request)
	if authorization != "Bearer abc" || strings.Contains(rawQuery, "abc") {
		t.Fatalf("upgrade: authorization %q, query %q", authorization, rawQuery)
	}
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ws?token=abc", nil))
	if authorization != "" || strings.Contains(rawQuery, "abc") {
		t.Fatalf("plain request: authorization %q, query %q", authorization, rawQuery)
	}
}
