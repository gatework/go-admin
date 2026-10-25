package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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

func TestOptions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("OPTIONS request", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodOptions, "/", nil)

		Options(c)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("Access-Control-Allow-Origin = %q", got)
		}
		if got := w.Header().Get("Access-Control-Allow-Methods"); got != "GET,POST,PUT,PATCH,DELETE,OPTIONS" {
			t.Errorf("Access-Control-Allow-Methods = %q", got)
		}
		if got := w.Header().Get("Access-Control-Allow-Headers"); got != "authorization, origin, content-type, accept" {
			t.Errorf("Access-Control-Allow-Headers = %q", got)
		}
		if got := w.Header().Get("Allow"); got != "HEAD,GET,POST,PUT,PATCH,DELETE,OPTIONS" {
			t.Errorf("Allow = %q", got)
		}
		if got := w.Header().Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		if !c.IsAborted() {
			t.Error("expected the request to be aborted")
		}
		if w.Code != http.StatusOK {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
		}
	})

	t.Run("non-OPTIONS request", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		Options(c)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
		}
		if c.IsAborted() {
			t.Error("expected the request not to be aborted")
		}
	})
}

func TestSecure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("without TLS", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

		Secure(c)

		if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("Access-Control-Allow-Origin = %q", got)
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
			c.Request = httptest.NewRequest("GET", tc.path, nil)
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
