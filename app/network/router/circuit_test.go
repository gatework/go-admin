package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	mycasbin "github.com/go-admin-team/go-admin-core/v2/casbin"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"go-admin/app/network/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCircuitRoutesRequireJWTSeparatePermissionsAndDataScope(t *testing.T) {
	t.Setenv("ZABBIX_API_URL", "")
	t.Setenv("ZABBIX_API_TOKEN", "")
	gin.SetMode(gin.TestMode)
	oldDP := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = oldDP })
	oldInterval := mycasbin.ReloadInterval
	mycasbin.ReloadInterval = 0
	t.Cleanup(func() { mycasbin.ReloadInterval = oldInterval })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	if err = db.AutoMigrate(&models.Circuit{}); err != nil {
		t.Fatal(err)
	}
	for i, owner := range []int{7, 8} {
		row := models.Circuit{CircuitFields: models.CircuitFields{CircuitNo: []string{"C-1", "C-2"}[i], Name: []string{"visible", "hidden"}[i], Provider: "Carrier", Category: "other", Purpose: "other", Payer: "us", Currency: "CNY", Status: "planned", Version: 1}}
		row.SetCreateBy(owner)
		if err = db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	const host = "circuit-authorization.test"
	enforcer := mycasbin.Setup(db, host)
	previous := sdk.Runtime.GetCasbinByTenant(host)
	sdk.Runtime.SetCasbinByTenant(host, enforcer)
	t.Cleanup(func() { sdk.Runtime.SetCasbinByTenant(host, previous) })
	routes := []struct{ method, path, policy, body string }{
		{"GET", "/api/v1/network-circuit", "/api/v1/network-circuit", ""},
		{"GET", "/api/v1/network-circuit/2", "/api/v1/network-circuit/:id", ""},
		{"POST", "/api/v1/network-circuit", "/api/v1/network-circuit", `{}`},
		{"PUT", "/api/v1/network-circuit/2", "/api/v1/network-circuit/:id", `{"version":1,"circuitNo":"C-2","name":"attempt","provider":"Carrier"}`},
		{"DELETE", "/api/v1/network-circuit/2", "/api/v1/network-circuit/:id", `{"version":1}`},
		{"GET", "/api/v1/network-circuit/options", "/api/v1/network-circuit/options", ""},
		{"GET", "/api/v1/network-circuit/export", "/api/v1/network-circuit/export", ""},
		{"GET", "/api/v1/network-circuit/zabbix/hosts?keyword=router", "/api/v1/network-circuit/zabbix/hosts", ""},
		{"GET", "/api/v1/network-circuit/zabbix/items?hostId=1", "/api/v1/network-circuit/zabbix/items", ""},
		{"GET", "/api/v1/network-circuit/2/traffic?period=1h", "/api/v1/network-circuit/:id/traffic", ""},
	}
	for _, route := range routes {
		if _, err = enforcer.AddPolicy("circuit", route.policy, route.method); err != nil {
			t.Fatal(err)
		}
	}
	auth, err := jwt.New(&jwt.GinJWTMiddleware{Realm: "test", Key: []byte("test-key"), Timeout: time.Hour, PayloadFunc: func(data interface{}) jwt.MapClaims { return data.(jwt.MapClaims) }, TokenLookup: "header: Authorization", TokenHeadName: "Bearer", TimeFunc: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("db", db); c.Next() })
	InitBusinessRouter(engine, auth)
	tokens := map[string]string{}
	for _, role := range []string{"denied", "circuit"} {
		token, _, err := auth.TokenGenerator(jwt.MapClaims{"identity": 7, "rolekey": role, "roleid": 2, "deptid": 3, "datascope": actions.DataScopeSelf})
		if err != nil {
			t.Fatal(err)
		}
		tokens[role] = token
	}
	for _, route := range routes {
		for _, role := range []string{"", "denied", "circuit"} {
			request := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
			request.Host = host
			request.Header.Set("Content-Type", "application/json")
			if role != "" {
				request.Header.Set("Authorization", "Bearer "+tokens[role])
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			var result struct {
				Code int
				Data struct {
					Count int
					List  []models.Circuit
				}
			}
			json.Unmarshal(response.Body.Bytes(), &result)
			if role == "" && response.Code != http.StatusUnauthorized && result.Code != 401 {
				t.Errorf("anonymous allowed %s %s", route.method, route.path)
			}
			if role == "denied" && result.Code != 403 {
				t.Errorf("permission bypass %s %s: %s", route.method, route.path, response.Body.String())
			}
			if role == "circuit" {
				if result.Code == 403 {
					t.Errorf("granted endpoint refused: %s %s", route.method, route.path)
				}
				if route.method == "GET" && route.path == "/api/v1/network-circuit" && (result.Code != 200 || result.Data.Count != 1 || len(result.Data.List) != 1 || result.Data.List[0].Name != "visible") {
					t.Errorf("data scope lost: %s", response.Body.String())
				}
				if strings.Contains(route.path, "/2") && result.Code != 404 {
					t.Errorf("hidden circuit exposed %s: %s", route.path, response.Body.String())
				}
				if strings.HasSuffix(route.path, "/export") && strings.Contains(response.Body.String(), "hidden") {
					t.Error("export leaked hidden circuit")
				}
			}
		}
	}
}

func TestCircuitRoutePermissionsKeepStaticActionsSeparateFromDetail(t *testing.T) {
	t.Setenv("ZABBIX_API_URL", "")
	t.Setenv("ZABBIX_API_TOKEN", "")
	gin.SetMode(gin.TestMode)
	oldDP := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = oldDP })
	oldInterval := mycasbin.ReloadInterval
	mycasbin.ReloadInterval = 0
	t.Cleanup(func() { mycasbin.ReloadInterval = oldInterval })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	if err := db.AutoMigrate(&models.Circuit{}, &models.Device{}); err != nil {
		t.Fatal(err)
	}
	row := models.Circuit{CircuitFields: models.CircuitFields{CircuitNo: "C-1", Name: "Visible", Provider: "Carrier", Category: "other", Purpose: "other", Payer: "us", Currency: "CNY", Status: "planned", Version: 1}}
	row.SetCreateBy(7)
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	const host = "circuit-route-isolation.test"
	enforcer := mycasbin.Setup(db, host)
	previous := sdk.Runtime.GetCasbinByTenant(host)
	sdk.Runtime.SetCasbinByTenant(host, enforcer)
	t.Cleanup(func() { sdk.Runtime.SetCasbinByTenant(host, previous) })
	policies := map[string][][2]string{
		"query":      {{"/api/v1/network-circuit/:id", "GET"}},
		"edit":       {{"/api/v1/network-circuit/:id", "GET"}, {"/api/v1/network-circuit/:id", "PUT"}, {"/api/v1/network-circuit/options", "GET"}, {"/api/v1/network-circuit/zabbix/hosts", "GET"}, {"/api/v1/network-circuit/zabbix/items", "GET"}, {"/api/v1/network-device", "GET"}},
		"add":        {{"/api/v1/network-circuit", "POST"}, {"/api/v1/network-circuit/options", "GET"}, {"/api/v1/network-circuit/zabbix/hosts", "GET"}, {"/api/v1/network-circuit/zabbix/items", "GET"}, {"/api/v1/network-device", "GET"}},
		"export":     {{"/api/v1/network-circuit/export", "GET"}},
		"list":       {{"/api/v1/network-circuit", "GET"}, {"/api/v1/network-circuit/options", "GET"}},
		"traffic":    {{"/api/v1/network-circuit/:id/traffic", "GET"}},
		"id-methods": {{"/api/v1/network-circuit/:id", "*"}},
		"prefix":     {{"/api/v1/network-circuit/*", "GET"}},
		"global":     {{"*", "*"}},
		"admin":      {},
	}
	for role, grants := range policies {
		for _, grant := range grants {
			if _, err := enforcer.AddPolicy(role, grant[0], grant[1]); err != nil {
				t.Fatal(err)
			}
		}
	}
	auth, err := jwt.New(&jwt.GinJWTMiddleware{Realm: "test", Key: []byte("test-key"), Timeout: time.Hour, PayloadFunc: func(data interface{}) jwt.MapClaims { return data.(jwt.MapClaims) }, TokenLookup: "header: Authorization", TokenHeadName: "Bearer", TimeFunc: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("db", db); c.Next() })
	InitBusinessRouter(engine, auth)
	tokens := map[string]string{}
	for role := range policies {
		token, _, err := auth.TokenGenerator(jwt.MapClaims{"identity": 7, "rolekey": role, "roleid": 2, "deptid": 3, "datascope": actions.DataScopeSelf})
		if err != nil {
			t.Fatal(err)
		}
		tokens[role] = token
	}
	routes := []struct {
		method, path, body string
		allowed            []string
	}{
		{"GET", "/api/v1/network-circuit", "", []string{"list", "global", "admin"}},
		{"GET", "/api/v1/network-circuit/1", "", []string{"query", "edit", "id-methods", "prefix", "global", "admin"}},
		{"GET", "/api/v1/network-circuit/export", "", []string{"export", "prefix", "global", "admin"}},
		{"GET", "/api/v1/network-circuit/options", "", []string{"list", "add", "edit", "prefix", "global", "admin"}},
		{"GET", "/api/v1/network-circuit/zabbix/hosts?keyword=router", "", []string{"add", "edit", "prefix", "global", "admin"}},
		{"GET", "/api/v1/network-circuit/zabbix/items?hostId=1", "", []string{"add", "edit", "prefix", "global", "admin"}},
		{"GET", "/api/v1/network-circuit/1/traffic?period=1h", "", []string{"traffic", "prefix", "global", "admin"}},
		{"GET", "/api/v1/network-device", "", []string{"add", "edit", "global", "admin"}},
		{"POST", "/api/v1/network-circuit", `{}`, []string{"add", "global", "admin"}},
		{"PUT", "/api/v1/network-circuit/1", `{}`, []string{"edit", "id-methods", "global", "admin"}},
		{"DELETE", "/api/v1/network-circuit/1", `{"version":0}`, []string{"id-methods", "global", "admin"}},
	}
	for _, route := range routes {
		for role, token := range tokens {
			request := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
			request.Host = host
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			var result struct{ Code int }
			if strings.HasPrefix(response.Header().Get("Content-Type"), "text/csv") {
				result.Code = response.Code
			} else if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			allowed := slices.Contains(route.allowed, role)
			if !allowed && result.Code != 403 {
				t.Errorf("%s role inherited ungranted %s %s: code=%d", role, route.method, route.path, result.Code)
			}
			if allowed && (result.Code == 403 || result.Code == 401 || result.Code == 500) {
				t.Errorf("%s role lost granted %s %s: %s", role, route.method, route.path, response.Body.String())
			}
		}
	}
}

func TestCircuitRoutePermissionFailsClosedWithoutRequestIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const host = "circuit-missing-enforcer.test"
	previous := sdk.Runtime.GetCasbinByTenant(host)
	sdk.Runtime.SetCasbinByTenant(host, nil)
	t.Cleanup(func() { sdk.Runtime.SetCasbinByTenant(host, previous) })
	for _, test := range []struct {
		name, route string
		claims      any
	}{
		{"missing claims", "/api/v1/network-circuit/export", nil},
		{"invalid claims", "/api/v1/network-circuit/export", "invalid"},
		{"missing role", "/api/v1/network-circuit/export", jwt.MapClaims{}},
		{"missing enforcer", "/api/v1/network-circuit/export", jwt.MapClaims{"rolekey": "query"}},
		{"unsupported route", "/unrelated", jwt.MapClaims{"rolekey": "admin"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(func(c *gin.Context) {
				if test.claims != nil {
					c.Set(jwt.JwtPayloadKey, test.claims)
				}
				c.Next()
			})
			reached := false
			engine.GET(test.route, circuitRoutePermission(), func(c *gin.Context) { reached = true })
			request := httptest.NewRequest("GET", test.route, nil)
			request.Host = host
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			var result struct{ Code int }
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if reached || result.Code != 403 {
				t.Fatalf("missing authorization state reached handler: %s", response.Body.String())
			}
		})
	}
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest("GET", "/api/v1/network-circuit/export", nil)
	ctx.Set(jwt.JwtPayloadKey, jwt.MapClaims{"rolekey": "admin"})
	circuitRoutePermission()(ctx)
	var result struct{ Code int }
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Code != 403 || !ctx.IsAborted() {
		t.Fatalf("missing registered route did not fail closed: %s %v", response.Body.String(), err)
	}
}
