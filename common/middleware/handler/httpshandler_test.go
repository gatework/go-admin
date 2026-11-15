package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTLSHandlerStopsTheChainWhenItRedirects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	reached := false
	engine.GET("/api", TLSHandler(), func(c *gin.Context) {
		reached = true
		c.String(http.StatusOK, "handled")
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://motor.example/api", nil))
	if response.Code < 300 || response.Code >= 400 {
		t.Fatalf("plain request not redirected: %d", response.Code)
	}
	if reached {
		t.Fatal("handler ran after the redirect was written")
	}
}
