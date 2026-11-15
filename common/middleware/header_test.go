package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor/config"
)

func TestNoCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	NoCache(c)

	if got := w.Header().Get("Cache-Control"); got != "no-cache, no-store, max-age=0, must-revalidate" {
		t.Errorf("Cache-Control = %q", got)
	}
	if got := w.Header().Get("Expires"); got != "Thu, 01 Jan 1970 00:00:00 GMT" {
		t.Errorf("Expires = %q", got)
	}
	if got := w.Header().Get("Last-Modified"); got == "" {
		t.Error("Last-Modified should not be empty")
	} else if _, err := time.Parse(http.TimeFormat, got); err != nil {
		t.Errorf("Last-Modified = %q is not a valid HTTP time: %v", got, err)
	}
}

func TestCORSAnswersOnlyListedOrigins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.Ext()
	t.Cleanup(func() { config.SetExt(previous) })
	config.SetExt(config.Extend{CORS: config.CORS{Origins: []string{"http://localhost:9527"}}})
	engine := gin.New()
	engine.Use(CORS)
	engine.GET("/api", func(c *gin.Context) { c.Status(http.StatusOK) })
	serve := func(method, origin string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "/api", nil)
		if origin != "" {
			request.Header.Set("Origin", origin)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response
	}

	preflight := serve(http.MethodOptions, "http://localhost:9527")
	if preflight.Code != http.StatusNoContent || preflight.Header().Get("Access-Control-Allow-Origin") != "http://localhost:9527" ||
		!strings.Contains(preflight.Header().Get("Access-Control-Allow-Headers"), "authorization") {
		t.Fatalf("listed preflight: %d %v", preflight.Code, preflight.Header())
	}
	if got := serve(http.MethodGet, "http://localhost:9527").Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:9527" {
		t.Fatalf("listed origin not granted: %q", got)
	}
	for _, response := range []*httptest.ResponseRecorder{serve(http.MethodGet, "https://evil.example"), serve(http.MethodOptions, "https://evil.example"), serve(http.MethodGet, "")} {
		if got := response.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("unlisted origin granted %q", got)
		}
		if !strings.Contains(response.Header().Get("Vary"), "Origin") {
			t.Fatal("response varies by origin without saying so")
		}
	}
}

func TestSecure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("without TLS", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		Secure(c)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want none", got)
		}
		if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Errorf("X-Content-Type-Options = %q", got)
		}
		if got := w.Header().Get("X-XSS-Protection"); got != "1; mode=block" {
			t.Errorf("X-XSS-Protection = %q", got)
		}
		if got := w.Header().Get("Strict-Transport-Security"); got != "" {
			t.Errorf("Strict-Transport-Security = %q, want empty without TLS", got)
		}
	})

	t.Run("with TLS", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Request.TLS = &tls.ConnectionState{}

		Secure(c)

		if got := w.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
			t.Errorf("Strict-Transport-Security = %q", got)
		}
	})
}

func TestUploadHeadersDoNotDisableBundledApplications(t *testing.T) {
	for _, tc := range []struct {
		path                string
		sandbox, attachment bool
	}{
		{"/static/uploadfile/example.html", true, true},
		{"/static/UPLOADFILE/example.html", true, true},
		{"/static/UploadFile/example.svg", true, true},
		{"/static/UPLOADFILE/EXAMPLE.PNG", true, false},
		{"/static/uploadfile/example.svg", true, true},
		{"/static/uploadfile/example.png", true, false},
		{"/static/form-generator/index.html", false, false},
		{"/static/form-generator/js/index.js", false, false},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, tc.path, nil)
			Secure(c)
			if (w.Header().Get("Content-Security-Policy") != "") != tc.sandbox {
				t.Errorf("sandbox header for %s: %v", tc.path, w.Header())
			}
			if (w.Header().Get("Content-Disposition") != "") != tc.attachment {
				t.Errorf("attachment header for %s: %v", tc.path, w.Header())
			}
		})
	}
}
