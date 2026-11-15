package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor/config"
)

// The metrics endpoint used to answer anyone who could reach the port. It is
// served only to a scraper presenting the configured token, and not at all
// when none is configured.
func TestMetricsRequireTheConfiguredScrapeToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.Ext()
	t.Cleanup(func() { config.SetExt(previous) })
	engine := gin.New()
	RegisterMonitorRouter(engine.Group("/api/v1"))
	scrape := func(authorization string) int {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		return response.Code
	}

	config.SetExt(config.Extend{})
	if code := scrape("Bearer anything"); code != http.StatusNotFound {
		t.Fatalf("metrics served with no token configured: %d", code)
	}

	configured := config.Extend{Metrics: config.Metrics{Token: "scrape-token"}}
	config.SetExt(configured)
	for _, wrong := range []string{"", "Bearer wrong", "scrape-token", "Basic scrape-token"} {
		if code := scrape(wrong); code != http.StatusUnauthorized {
			t.Fatalf("metrics served for %q: %d", wrong, code)
		}
	}
	if code := scrape("Bearer scrape-token"); code != http.StatusOK {
		t.Fatalf("configured scraper refused: %d", code)
	}
}
