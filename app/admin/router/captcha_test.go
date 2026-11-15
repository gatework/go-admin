package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk/config"

	"github.com/gatework/motor/common/middleware"
	"github.com/gatework/motor/internal/testconfig"
)

// The login form asks for a captcha before anyone is signed in.
func TestCaptchaIsServedBeforeSignIn(t *testing.T) {
	testconfig.Apply(t, func(s *config.Config) { s.JWT.Secret = "captcha-test-secret-of-adequate-length" })
	mw, err := middleware.AuthInit()
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	_ = InitSysRouter(r, mw)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/captcha", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("captcha answered %d %s", w.Code, w.Body.String())
	}
}
