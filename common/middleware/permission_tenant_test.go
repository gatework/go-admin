package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/sdk"
)

// A request for a host with no authorization enforcer (an unknown tenant, or one
// still being set up) used to dereference a nil enforcer and panic. It is an
// unavailable tenant, not a crash.
func TestPermissionChecksAnswerUnavailableForATenantWithoutAnEnforcer(t *testing.T) {
	const host = "no-enforcer.invalid"
	if sdk.Runtime.Casbin(host) != nil {
		t.Skip("a wildcard enforcer is installed in this process")
	}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(jwtauth.JwtPayloadKey, jwtauth.MapClaims{"identity": float64(7), "rolekey": "operator"})
		c.Next()
	})
	engine.GET("/checked", AuthCheckRole(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	engine.GET("/enforced", func(c *gin.Context) {
		if _, err := EnforceRoleFor(c, "/api/v1/sys-user", http.MethodPut); err == nil {
			t.Error("EnforceRoleFor answered without an enforcer")
		}
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/checked", nil)
	request.Host = host
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	var body struct{ Code int }
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", response.Body.String(), err)
	}
	if body.Code != http.StatusServiceUnavailable {
		t.Fatalf("code %d, body %s", body.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/enforced", nil)
	request.Host = host
	engine.ServeHTTP(httptest.NewRecorder(), request)
}
