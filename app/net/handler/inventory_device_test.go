package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/sdk/contract/actions"
	models "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/models/txlock"
	inventorysvc "github.com/gatework/motor/app/net/service/inventory"
	"github.com/gatework/motor/app/net/service/inventory/provider/netdisco"
)

func apiTestEngine(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&models.Device{}, &models.SyncState{}, &models.DeviceGroup{}, &models.DeviceGroupLock{}, &txlock.Row{}); err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set(jwt.JwtPayloadKey, jwt.MapClaims{"identity": float64(7), "rolekey": "operator"})
		c.Set(actions.PermissionKey, &actions.DataPermission{DataScope: actions.DataScopeAll, UserID: 7})
		c.Next()
	})
	api := Device{Netdisco: netdisco.FromEnvironment()}
	engine.GET("/devices", api.List)
	engine.GET("/devices/options", api.Options)
	engine.GET("/devices/sync-status", api.SyncStatus)
	engine.GET("/devices/export", api.Export)
	engine.GET("/devices/:id", api.Get)
	engine.PUT("/devices/:id", api.Update)
	engine.POST("/devices/sync", api.Sync)
	return engine, db
}
func serve(t *testing.T, engine *gin.Engine, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	var value map[string]any
	if !strings.HasPrefix(response.Header().Get("Content-Type"), "text/csv") {
		if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
			t.Fatalf("invalid response: %s", response.Body.String())
		}
	}
	return response, value
}
func TestAPIRejectsSourceMutationAndStaleVersion(t *testing.T) {
	engine, db := apiTestEngine(t)
	row := models.Device{IP: "192.0.2.1", Name: "source name", SourceTags: []string{}, Status: "pending", Tags: []string{}, Version: 1}
	row.SetCreateBy(7)
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"version":1,"status":"active","groupSource":"auto","classificationSource":"auto","name":"injected"}`, `{"version":1,"status":"active","groupSource":"auto","classificationSource":"auto","group":"legacy writable label"}`, `{"version":1,"status":"active","groupSource":"auto","classificationSource":"auto"} {}`, `{}`, `{"version":1,"status":"unsupported"}`} {
		_, result := serve(t, engine, "PUT", "/devices/1", body)
		if result["code"] != float64(400) {
			t.Fatalf("invalid input accepted: %s %+v", body, result)
		}
	}
	_, result := serve(t, engine, "PUT", "/devices/1", `{"version":1,"status":"active","groupSource":"auto","classificationSource":"auto","owner":"Alice","tags":[]}`)
	if result["code"] != float64(200) {
		t.Fatal(result)
	}
	_, result = serve(t, engine, "PUT", "/devices/1", `{"version":1,"status":"disabled","groupSource":"auto","classificationSource":"auto","owner":"Bob","tags":[]}`)
	if result["code"] != float64(409) {
		t.Fatalf("missing conflict: %+v", result)
	}
	db.First(&row, 1)
	if row.Name != "source name" || row.Owner != "Alice" || row.Status != "active" {
		t.Fatal("failed request changed data")
	}
}
func TestAPIStatusPaginationCSVAndSafeProviderErrors(t *testing.T) {
	t.Setenv("NETDISCO_URL", "")
	t.Setenv("NETDISCO_API_KEY", "")
	t.Setenv("NETDISCO_USERNAME", "")
	t.Setenv("NETDISCO_PASSWORD", "")
	engine, _ := apiTestEngine(t)
	_, body := serve(t, engine, "GET", "/devices/sync-status", "")
	data := body["data"].(map[string]any)
	if data["configured"] != false || data["status"] != "idle" {
		t.Fatalf("wrong status: %+v", body)
	}
	_, body = serve(t, engine, "GET", "/devices?pageSize=201", "")
	if body["code"] != float64(400) {
		t.Fatal("unbounded page size")
	}
	_, body = serve(t, engine, "GET", "/devices", "")
	data = body["data"].(map[string]any)
	if _, ok := data["list"].([]any); !ok || data["count"] != float64(0) {
		t.Fatalf("wrong pagination shape: %+v", body)
	}
	response, _ := serve(t, engine, "GET", "/devices/export", "")
	if !strings.Contains(response.Header().Get("Content-Disposition"), "attachment;") || !strings.Contains(response.Body.String(), "管理 IP") {
		t.Fatal("missing CSV download")
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("password=fixture-secret"))
	}))
	defer upstream.Close()
	t.Setenv("NETDISCO_URL", upstream.URL)
	t.Setenv("NETDISCO_API_KEY", "fixture-secret")
	// Configuration is read once, when routes are built.
	engine, db := apiTestEngine(t)
	_, body = serve(t, engine, "POST", "/devices/sync", "")
	if body["code"] != float64(200) || body["data"].(map[string]any)["status"] != "queued" {
		t.Fatalf("sync was not queued: %+v", body)
	}
	if _, ran, err := (&inventorysvc.Device{DB: db, Source: netdisco.New(netdisco.FromEnvironment())}).RunQueuedSync(context.Background()); !ran || err == nil {
		t.Fatalf("worker run: ran %v, err %v; want the upstream failure", ran, err)
	}
	response, body = serve(t, engine, "GET", "/devices/sync-status", "")
	data = body["data"].(map[string]any)
	if data["status"] != "failed" || data["message"] != "inventory.netdisco_unavailable" || strings.Contains(response.Body.String(), "fixture-secret") || strings.Contains(response.Body.String(), upstream.URL) {
		t.Fatalf("sync failure not persisted safely: %s", response.Body.String())
	}
}
