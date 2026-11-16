package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	service "github.com/gatework/motor/app/net/service/diagnostics"
)

// Routes are registered once and served concurrently, so a handler must not keep
// per-request state (the gin context, the tenant database) on a value that every
// request shares. Run with -race: a shared write is reported as a data race.
func serveConcurrently(t *testing.T, engine *gin.Engine, method, path, body string) {
	t.Helper()
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			req := httptest.NewRequest(method, path, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			engine.ServeHTTP(httptest.NewRecorder(), req)
		})
	}
	wg.Wait()
}

func identified(engine *gin.Engine) {
	engine.Use(func(c *gin.Context) {
		c.Set(jwt.JwtPayloadKey, jwt.MapClaims{"identity": float64(7), "rolekey": "operator"})
		c.Next()
	})
}

func TestFirewallHandlersKeepNoStateBetweenRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	identified(engine)
	handlers := Firewall{}.Handlers()
	engine.POST("/normalize", handlers["Normalize"])
	engine.GET("/domains", handlers["Domains"])
	serveConcurrently(t, engine, http.MethodPost, "/normalize", `{}`)
	serveConcurrently(t, engine, http.MethodGet, "/domains", "")
}

func TestDiagnosticsHandlersKeepNoStateBetweenRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := service.NewManager()
	defer manager.Close()
	h := Diagnostics{Manager: manager}
	engine := gin.New()
	diagnosticsDB(t, engine)
	identified(engine)
	engine.GET("/server", h.Server)
	engine.GET("/jobs/:id", h.Get)
	serveConcurrently(t, engine, http.MethodGet, "/server", "")
	serveConcurrently(t, engine, http.MethodGet, "/jobs/missing", "")
}
