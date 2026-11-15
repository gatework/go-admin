package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRecoverAnswersPanicsThatCustomErrorRethrows(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(Recover, CustomError)
	engine.GET("/value", func(*gin.Context) { panic(struct{ reason string }{"not a CustomError protocol value"}) })
	engine.GET("/nil", func(c *gin.Context) {
		values := map[string][]int{}
		c.Status(values[c.Query("missing")][0]) // a runtime.Error: index out of range
	})
	engine.GET("/written", func(c *gin.Context) {
		c.String(http.StatusAccepted, "partial")
		panic("after write")
	})
	engine.GET("/abort", func(*gin.Context) { panic(http.ErrAbortHandler) })

	for _, path := range []string{"/value", "/nil"} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), `"code":500`) {
			t.Fatalf("%s: status %d body %s", path, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "runtime error") || strings.Contains(response.Body.String(), "CustomError protocol") {
			t.Fatalf("%s leaked panic detail: %s", path, response.Body.String())
		}
	}

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/written", nil))
	if response.Code != http.StatusAccepted || response.Body.String() != "partial" {
		t.Fatalf("recovery rewrote a started response: %d %q", response.Code, response.Body.String())
	}

	defer func() {
		if recover() == nil {
			t.Fatal("http.ErrAbortHandler must keep propagating to net/http")
		}
	}()
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/abort", nil))
}
