package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/response"
)

func TestErrorStatusPutsTheEnvelopeStatusOnTheWire(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(ErrorStatus)
	engine.GET("/ok", func(c *gin.Context) { response.OK(c, "x", "") })
	engine.GET("/missing", func(c *gin.Context) { response.Error(c, http.StatusNotFound, nil, "not found") })
	engine.GET("/broken", func(c *gin.Context) { response.Error(c, http.StatusInternalServerError, nil, "") })
	engine.GET("/legacy", func(c *gin.Context) { response.Error(c, -1, nil, "not an HTTP status") })
	engine.GET("/explicit", func(c *gin.Context) {
		c.Set(statusKey, http.StatusConflict)
		c.String(http.StatusAccepted, "own status")
	})
	for path, want := range map[string]int{
		"/ok":       http.StatusOK,
		"/missing":  http.StatusNotFound,
		"/broken":   http.StatusInternalServerError,
		"/legacy":   http.StatusOK,
		"/explicit": http.StatusAccepted,
	} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != want {
			t.Errorf("%s: status %d, want %d", path, response.Code, want)
		}
	}
}
