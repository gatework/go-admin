package router

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
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
	models "github.com/gatework/motor/app/net/models/firewall"
	network "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	service "github.com/gatework/motor/app/net/service/firewall"
	"github.com/gatework/motor/internal/testconfig"
)

func TestFirewallRoutesEnforceRegisteredCapabilityAndInputScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	interval := mycasbin.ReloadInterval
	mycasbin.ReloadInterval = 0
	t.Cleanup(func() { mycasbin.ReloadInterval = interval })
	db, e := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "http.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	conn, _ := db.DB()
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	if e = db.AutoMigrate(append(models.Schema(), &network.Device{}, &network.DeviceGroup{})...); e != nil {
		t.Fatal(e)
	}
	for _, owner := range []int{7, 8} {
		d := network.Device{CreateBy: owner, Name: "synthetic", IP: map[int]string{7: "192.0.2.7", 8: "192.0.2.8"}[owner], SourceTags: []string{}, Tags: []string{}}
		if e = db.Create(&d).Error; e != nil {
			t.Fatal(e)
		}
		if e = db.Create(&models.Instance{DeviceID: d.ID, DomainID: 1, Version: 1, SettingsVersion: 1}).Error; e != nil {
			t.Fatal(e)
		}
	}
	if e = db.Create(&models.Domain{ID: 1, Code: "default", Name: "Default", Version: 1}).Error; e != nil {
		t.Fatal(e)
	}
	const host = "firewall-auth.test"
	enforcer := mycasbin.Setup(db, host)
	previous := sdk.Runtime.Casbin(host)
	sdk.Runtime.SetCasbin(host, enforcer)
	t.Cleanup(func() { sdk.Runtime.SetCasbin(host, previous) })
	for _, route := range FirewallRoutes {
		if route.Capability == "list" || route.Capability == "query" {
			if _, e = enforcer.AddPolicy("reader", "/api/v1/network-firewall"+route.Path, route.Method); e != nil {
				t.Fatal(e)
			}
		}
	}
	auth, e := jwt.New(&jwt.GinJWTMiddleware{Realm: "test", Key: []byte("local-firewall-test"), Timeout: time.Hour, PayloadFunc: func(v any) jwt.MapClaims { return v.(jwt.MapClaims) }, TokenLookup: "header: Authorization", TokenHeadName: "Bearer", TimeFunc: time.Now})
	if e != nil {
		t.Fatal(e)
	}
	token, _, e := auth.TokenGenerator(jwt.MapClaims{"identity": 7, "rolekey": "reader", "roleid": 2, "deptid": 3, "datascope": actions.DataScopeSelf})
	if e != nil {
		t.Fatal(e)
	}
	t.Setenv("FIREWALL_SNAPSHOT_DIR", t.TempDir())
	service.RegisterRoot(db, host)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("db", db.WithContext(c.Request.Context())); c.Next() })
	RegisterFirewall(r, auth, deviceaccess.Config{})
	call := func(method, path, body, bearer string) (int, json.RawMessage) {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1/network-firewall"+path, strings.NewReader(body))
		req.Host = host
		req.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var out struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		if e = json.Unmarshal(w.Body.Bytes(), &out); e != nil {
			t.Fatal(w.Body.String())
		}
		return out.Code, out.Data
	}
	if code, _ := call("GET", "/instances", "", ""); code != 401 {
		t.Fatalf("anonymous code=%d", code)
	}
	code, data := call("GET", "/instances", "", token)
	var page struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal(data, &page)
	if code != 200 || page.Count != 1 {
		t.Fatalf("scope lost: code=%d data=%s", code, data)
	}
	if code, _ = call("GET", "/instances/2", "", token); code != 404 {
		t.Fatalf("hidden instance code=%d", code)
	}
	if code, _ = call("POST", "/instances/1/imports", `{}`, token); code != 403 {
		t.Fatalf("reader could import: %d", code)
	}
	if code, _ = call("POST", "/collection-batches", `{}`, token); code != 403 {
		t.Fatalf("reader could start device login: %d", code)
	}
	if code, _ = call("PUT", "/devices/1/connection", `{}`, token); code != 403 {
		t.Fatalf("reader could change connection: %d", code)
	}
	if code, _ = call("POST", "/normalize", `{"defaultProtocol":"tcp","rows":[{"source":"192.0.2.1","destination":"198.51.100.1","service":"443"}]}`, token); code != 200 {
		t.Fatalf("query POST not authorized: %d", code)
	}
	if code, _ = call("POST", "/normalize", `{"rows":[],"invented":true}`, token); code != 400 {
		t.Fatalf("unknown fields accepted: %d", code)
	}
}
