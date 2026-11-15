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
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/contract/actions"
	models "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/internal/testconfig"
)

func TestInventoryRoutesRequireJWTAndCasbinAndApplyScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	previousInterval := mycasbin.ReloadInterval
	mycasbin.ReloadInterval = 0
	t.Cleanup(func() { mycasbin.ReloadInterval = previousInterval })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := db.DB()
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	if err := db.AutoMigrate(&models.Device{}, &models.SyncState{}, &models.DeviceGroup{}, &models.DeviceGroupLock{}); err != nil {
		t.Fatal(err)
	}
	for i, owner := range []int{7, 8} {
		row := models.Device{IP: []string{"192.0.2.1", "192.0.2.2"}[i], Name: []string{"visible", "hidden"}[i], SourceTags: []string{}, Tags: []string{}, Version: 1, Status: "pending"}
		row.SetCreateBy(owner)
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	const host = "network-authorization.test"
	enforcer := mycasbin.Setup(db, host)
	previous := sdk.Runtime.Casbin(host)
	sdk.Runtime.SetCasbin(host, enforcer)
	t.Cleanup(func() { sdk.Runtime.SetCasbin(host, previous) })
	routes := []struct{ method, path, policy, body string }{
		{"GET", "/api/v1/network-device", "/api/v1/network-device", ""},
		{"GET", "/api/v1/network-device/1", "/api/v1/network-device/:id", ""},
		{"PUT", "/api/v1/network-device/1", "/api/v1/network-device/:id", `{"version":1,"status":"active","groupSource":"auto","classificationSource":"auto"}`},
		{"GET", "/api/v1/network-device/options", "/api/v1/network-device/options", ""},
		{"GET", "/api/v1/network-device/sync-status", "/api/v1/network-device/sync-status", ""},
		{"GET", "/api/v1/network-device/export", "/api/v1/network-device/export", ""},
		{"POST", "/api/v1/network-device/sync", "/api/v1/network-device/sync", ""},
	}
	for _, route := range routes {
		if _, err := enforcer.AddPolicy("inventory", route.policy, route.method); err != nil {
			t.Fatal(err)
		}
	}
	auth, err := jwt.New(&jwt.GinJWTMiddleware{Realm: "test", Key: []byte("test-key"), Timeout: time.Hour, PayloadFunc: func(data any) jwt.MapClaims { return data.(jwt.MapClaims) }, TokenLookup: "header: Authorization", TokenHeadName: "Bearer", TimeFunc: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("db", db); c.Next() })
	RegisterInventory(engine, auth)
	tokens := map[string]string{}
	for _, role := range []string{"denied", "inventory"} {
		token, _, err := auth.TokenGenerator(jwt.MapClaims{"identity": 7, "rolekey": role, "roleid": 2, "deptid": 3, "datascope": actions.DataScopeSelf})
		if err != nil {
			t.Fatal(err)
		}
		tokens[role] = token
	}
	for _, route := range routes {
		for _, role := range []string{"", "denied", "inventory"} {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
			req.Host = host
			req.Header.Set("Content-Type", "application/json")
			if role != "" {
				req.Header.Set("Authorization", "Bearer "+tokens[role])
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			var result struct {
				Code int
				Msg  string
				Data struct {
					Count int
					List  []models.Device
				}
			}
			json.Unmarshal(response.Body.Bytes(), &result)
			switch role {
			case "":
				if response.Code != http.StatusUnauthorized && result.Code != http.StatusUnauthorized {
					t.Errorf("%s %s allowed anonymous request: %s", route.method, route.path, response.Body.String())
				}
			case "denied":
				if result.Code != 403 {
					t.Errorf("%s %s bypassed Casbin: %s", route.method, route.path, response.Body.String())
				}
			case "inventory":
				if strings.Contains(result.Msg, "没有该接口访问权限") {
					t.Errorf("granted route refused: %s %s", route.method, route.path)
				}
				if route.path == "/api/v1/network-device" && (result.Code != 200 || result.Data.Count != 1 || len(result.Data.List) != 1 || result.Data.List[0].Name != "visible") {
					t.Errorf("route lost PermissionAction: %s", response.Body.String())
				}
				if strings.HasSuffix(route.path, "/sync") && (result.Code != 403 || !strings.Contains(result.Msg, "全量数据权限")) {
					t.Errorf("route did not enforce full-scope sync: %s", response.Body.String())
				}
			}
		}
	}
}
