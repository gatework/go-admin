package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	vxlan "github.com/gatework/go-vxlan"
	service "github.com/gatework/motor/app/net/service/fabric"
	"github.com/gatework/motor/internal/testconfig"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	mycasbin "github.com/gatework/motor-core/casbin"
	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/delivery"
	models "github.com/gatework/motor/app/net/models/fabric"
	network "github.com/gatework/motor/app/net/models/inventory"
)

func TestRegisteredFabricRoutesEnforceStaticPermissionAndDeviceScope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	oldReload := mycasbin.ReloadInterval
	mycasbin.ReloadInterval = 0
	t.Cleanup(func() { mycasbin.ReloadInterval = oldReload })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	if err := db.AutoMigrate(&network.Device{}, &models.Fabric{}, &models.RuleSet{}, &models.Membership{}, &models.Observation{}, &models.Assessment{}, &delivery.Connection{}); err != nil {
		t.Fatal(err)
	}
	db.Create(&models.Fabric{ID: 1, Code: "SZX9", Version: 1, RulesVersion: 1, RuntimeMinutes: 5, ConfigMinutes: 30})
	db.Create(&models.RuleSet{ID: 1, Version: 1, Rules: []models.Rule{{FabricID: 1, Pattern: "SZX9", Enabled: true}}})
	for i, name := range []string{"SZX9-visible", "SZX9-secret"} {
		d := network.Device{IP: []string{"10.0.0.1", "10.0.0.2"}[i], Name: name, SourceTags: []string{}, Tags: []string{}, DeviceRole: "leaf_switch"}
		d.SetCreateBy(i + 7)
		if err := db.Create(&d).Error; err != nil {
			t.Fatal(err)
		}
	}
	const host = "fabric-route.test"
	enforcer := mycasbin.Setup(db, host)
	previous := sdk.Runtime.Casbin(host)
	sdk.Runtime.SetCasbin(host, enforcer)
	t.Cleanup(func() { sdk.Runtime.SetCasbin(host, previous) })
	for _, grant := range []struct{ role, path, method string }{{"reader", "/api/v1/network-fabrics/:id", "GET"}, {"members", "/api/v1/network-fabrics/members", "GET"}, {"editor", "/api/v1/network-fabrics/:id/settings", "PUT"}, {"rules", "/api/v1/network-fabrics/rules", "PUT"}, {"preview", "/api/v1/network-fabrics/:id/change-candidates", "POST"}} {
		if _, err := enforcer.AddPolicy(grant.role, grant.path, grant.method); err != nil {
			t.Fatal(err)
		}
	}
	auth, err := jwt.New(&jwt.GinJWTMiddleware{Realm: "test", Key: []byte("test-key"), Timeout: time.Hour, PayloadFunc: func(data any) jwt.MapClaims { return data.(jwt.MapClaims) }, TokenLookup: "header: Authorization", TokenHeadName: "Bearer", TimeFunc: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("db", db); c.Next() })
	RegisterFabric(engine, auth)
	for _, tc := range []struct {
		role, method, path, body string
		code                     int
	}{
		{"reader", "GET", "/api/v1/network-fabrics/1", "", 200},
		{"reader", "GET", "/api/v1/network-fabrics/rules", "", 403},
		{"reader", "GET", "/api/v1/network-fabrics/endpoints?keyword=10.0.0.1", "", 403},
		{"members", "GET", "/api/v1/network-fabrics/members?pageSize=201", "", 400},
		{"members", "GET", "/api/v1/network-fabrics/members", "", 200},
		{"editor", "PUT", "/api/v1/network-fabrics/1/settings", `{"version":1,"runtimeMinutes":5,"configMinutes":30}`, 403},
		{"rules", "PUT", "/api/v1/network-fabrics/rules", `{"version":1,"rules":[]}`, 403},
		{"editor", "PUT", "/api/v1/network-fabrics/1/settings", `{"version":1,"runtimeMinutes":5,"configMinutes":30,"script":"evil"}`, 400},
		{"reader", "POST", "/api/v1/network-fabrics/1/change-candidates", `{"sourceDeviceId":1,"kind":"svi_acl","vni":10008,"direction":"out","targetDeviceIds":[2]}`, 403},
		{"preview", "POST", "/api/v1/network-fabrics/1/change-candidates", `{"sourceDeviceId":1,"kind":"svi_acl","vni":10008,"direction":"out","targetDeviceIds":[2]}`, 403},
		{"preview", "POST", "/api/v1/network-fabrics/1/change-candidates", `{"sourceDeviceId":1,"kind":"svi_acl","vni":10008,"direction":"out","targetDeviceIds":[]}`, 400},
		{"preview", "POST", "/api/v1/network-fabrics/1/change-candidates", `{"sourceDeviceId":1,"kind":"svi_acl","vni":10008,"direction":"out","script":"evil"}`, 400},
	} {
		token, _, err := auth.TokenGenerator(jwt.MapClaims{"identity": 7, "rolekey": tc.role, "roleid": 2, "deptid": 3, "datascope": actions.DataScopeSelf})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		request.Host = host
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		var result struct {
			Code int
			Data json.RawMessage
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Code != tc.code {
			t.Errorf("%s %s: %s", tc.method, tc.path, response.Body.String())
		}
		if tc.code == 200 && strings.Contains(string(result.Data), "secret") {
			t.Fatalf("scope leak: %s", response.Body.String())
		}
	}
}

func TestFabricHTTPEnvelopesParsedEndpointsAndEmptyFabric(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	if err := db.AutoMigrate(&network.Device{}, &models.Fabric{}, &models.RuleSet{}, &models.Membership{}, &models.Observation{}, &models.Assessment{}, &delivery.Connection{}); err != nil {
		t.Fatal(err)
	}
	for id, code := range []string{"SZX9", "SHA"} {
		if err := db.Create(&models.Fabric{ID: id + 1, Code: code, Name: code, Version: 1, RulesVersion: 1, RuntimeMinutes: 5, ConfigMinutes: 30}).Error; err != nil {
			t.Fatal(err)
		}
	}
	db.Create(&models.RuleSet{ID: 1, Version: 1, Rules: []models.Rule{{FabricID: 1, Pattern: "SZX9", Enabled: true}}})
	device := network.Device{Name: "SZX9-LEF-1", IP: "192.0.2.1", Vendor: "Cisco", OS: "NX-OS", SourceTags: []string{}, DeviceRole: "leaf_switch", Version: 1, Tags: []string{}}
	device.SetCreateBy(7)
	if err := db.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	domain := service.Fabric{DB: db}
	if _, _, err := domain.VisibleMembers(context.Background(), 1, &actions.DataPermission{DataScope: actions.DataScopeAll}); err != nil {
		t.Fatal(err)
	}
	var member models.Membership
	db.First(&member, "device_id = ?", device.ID)
	cfg := vxlan.ParseConfig("hostname SZX9-LEF-1\ninterface Vlan8\n  vrf member red\n  ip address 192.0.2.254/24\nvlan 8\n  vn-segment 100008\n", true)
	runtime, err := vxlan.ParseRuntime("l2route_mac_ips", "Topology Mac Address Host IP Prod Flags Seq No Next-Hops\n8 0011.2233.4455 192.0.2.11 BGP -- 0 192.0.2.21 (Label: 100008)\n", true)
	if err != nil || cfg.Status != "complete" || runtime.Status != "complete" {
		t.Fatalf("parsed fixtures: %s %s %v", cfg, runtime, err)
	}
	now := time.Now().UTC()
	for _, o := range []models.Observation{{FabricID: 1, DeviceID: device.ID, Dimension: "configuration", Status: "complete", ObservedAt: now.Add(-time.Minute), MembershipVersion: member.Version, RuleVersion: member.RuleVersion, Data: cfg}, {FabricID: 1, DeviceID: device.ID, Dimension: "l2route_mac_ips", Status: "complete", ObservedAt: now, MembershipVersion: member.Version, RuleVersion: member.RuleVersion, Data: runtime}} {
		if err := db.Create(&o).Error; err != nil {
			t.Fatal(err)
		}
	}
	auth, err := jwt.New(&jwt.GinJWTMiddleware{Realm: "test", Key: []byte("test-key"), Timeout: time.Hour, PayloadFunc: func(data any) jwt.MapClaims { return data.(jwt.MapClaims) }, TokenLookup: "header: Authorization", TokenHeadName: "Bearer", TimeFunc: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set("db", db); c.Next() })
	RegisterFabric(engine, auth)
	token, _, err := auth.TokenGenerator(jwt.MapClaims{"identity": 7, "rolekey": "admin", "roleid": 1, "deptid": 3, "datascope": actions.DataScopeAll})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/network-fabrics?pageSize=20", "/api/v1/network-fabrics/1", "/api/v1/network-fabrics/2", "/api/v1/network-fabrics/endpoints?keyword=192.0.2.11&vrf=red", "/api/v1/network-fabrics/endpoints?keyword=192.0.2.11&vrf=blue"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		var envelope struct {
			Code int
			Data map[string]json.RawMessage
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Code != 200 {
			t.Fatalf("%s: %s", path, response.Body.String())
		}
		switch path {
		case "/api/v1/network-fabrics?pageSize=20":
			var rows []map[string]any
			json.Unmarshal(envelope.Data["list"], &rows)
			if len(rows) != 2 || string(envelope.Data["count"]) != "2" {
				t.Fatalf("list envelope: %s", response.Body.String())
			}
		case "/api/v1/network-fabrics/1":
			var nodes []map[string]any
			json.Unmarshal(envelope.Data["nodes"], &nodes)
			if len(nodes) != 1 || nodes[0]["name"] != "SZX9-LEF-1" || nodes[0]["configState"] != "historical" {
				t.Fatalf("detail: %s", response.Body.String())
			}
		case "/api/v1/network-fabrics/2":
			if string(envelope.Data["nodes"]) != "[]" || string(envelope.Data["state"]) != "\"unknown\"" {
				t.Fatalf("empty health: %s", response.Body.String())
			}
		case "/api/v1/network-fabrics/endpoints?keyword=192.0.2.11&vrf=red":
			var rows []map[string]any
			json.Unmarshal(envelope.Data["list"], &rows)
			if len(rows) != 1 || rows[0]["kind"] != "remote_vtep" || rows[0]["remoteVtep"] != "192.0.2.21" || rows[0]["fabricCode"] != "SZX9" {
				t.Fatalf("endpoint: %s", response.Body.String())
			}
		default:
			if string(envelope.Data["list"]) != "[]" || string(envelope.Data["count"]) != "0" {
				t.Fatalf("VRF isolation envelope: %s", response.Body.String())
			}
		}
	}
}
