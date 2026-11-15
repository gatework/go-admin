package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	mycasbin "github.com/gatework/motor-core/casbin"
	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/sdk"
	service "github.com/gatework/motor/app/net/service/diagnostics"
)

func TestDiagnosticsRoutesRequireJWTAndCasbin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousInterval := mycasbin.ReloadInterval
	mycasbin.ReloadInterval = 0
	t.Cleanup(func() { mycasbin.ReloadInterval = previousInterval })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { connection.Close() })
	const host = "diagnostics-authorization.test"
	enforcer := mycasbin.Setup(db, host)
	previous := sdk.Runtime.Casbin(host)
	sdk.Runtime.SetCasbin(host, enforcer)
	t.Cleanup(func() { sdk.Runtime.SetCasbin(host, previous) })
	routes := []struct {
		method, path, policy, body string
		code                       int
	}{
		{"GET", "/server", "/server", "", 200},
		{"POST", "/jobs", "/jobs", `{"tool":"invalid"}`, 400},
		{"GET", "/jobs/unknown", "/jobs/:id", "", 404},
		{"DELETE", "/jobs/unknown", "/jobs/:id", "", 404},
	}
	const base = "/api/v1/network-diagnostics"
	for _, route := range routes {
		if _, err := enforcer.AddPolicy("diagnostics", base+route.policy, route.method); err != nil {
			t.Fatal(err)
		}
	}
	auth, err := jwt.New(&jwt.GinJWTMiddleware{Realm: "test", Key: []byte("test-key"), Timeout: time.Hour, PayloadFunc: func(data any) jwt.MapClaims { return data.(jwt.MapClaims) }, TokenLookup: "header: Authorization", TokenHeadName: "Bearer", TimeFunc: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	manager := service.NewManager()
	t.Cleanup(manager.Close)
	engine := gin.New()
	RegisterDiagnostics(engine, auth, manager)
	for _, role := range []string{"", "denied", "diagnostics"} {
		token, _, err := auth.TokenGenerator(jwt.MapClaims{"identity": 7, "rolekey": role, "roleid": 2})
		if err != nil {
			t.Fatal(err)
		}
		for _, route := range routes {
			req := httptest.NewRequest(route.method, base+route.path, strings.NewReader(route.body))
			req.Host = host
			req.Header.Set("Content-Type", "application/json")
			if role != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			var result struct{ Code int }
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			want := route.code
			switch role {
			case "":
				want = http.StatusUnauthorized
			case "denied":
				want = http.StatusForbidden
			}
			if result.Code != want && response.Code != want {
				t.Errorf("role=%q %s %s: want %d, got %s", role, route.method, route.path, want, response.Body.String())
			}
		}
	}
}
