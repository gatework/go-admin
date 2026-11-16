package router

import (
	"encoding/json"
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
	"github.com/gatework/motor/app/net/models/txlock"
	"github.com/gatework/motor/app/net/service/inventory/provider/netdisco"
	"github.com/gatework/motor/app/net/service/inventory/provider/zabbix"
	"github.com/gatework/motor/internal/testconfig"
)

func TestDeviceRegisteredRoutesIsolateStaticActionsAndScopeGroupCounts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

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
	if err := db.AutoMigrate(&models.Device{}, &models.SyncState{}, &models.DeviceGroup{}, &models.DeviceGroupLock{}, &txlock.Row{}); err != nil {
		t.Fatal(err)
	}
	group := models.DeviceGroup{Code: "region", Name: "Region", Enabled: true}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	for i, owner := range []int{7, 8} {
		row := models.Device{IP: []string{"192.0.2.1", "192.0.2.2"}[i], Name: []string{"visible", "secret"}[i], SourceTags: []string{}, GroupID: &group.ID, GroupSource: "manual", ClassificationSource: "manual", Tags: []string{}}
		row.SetCreateBy(owner)
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	const host = "device-route-isolation.test"
	enforcer := mycasbin.Setup(db, host)
	previous := sdk.Runtime.Casbin(host)
	sdk.Runtime.SetCasbin(host, enforcer)
	t.Cleanup(func() { sdk.Runtime.SetCasbin(host, previous) })
	for _, grant := range []struct{ role, path, method string }{{"detail", "/api/v1/network-device/:id", "*"}, {"tree", "/api/v1/network-device-group", "GET"}, {"assign", "/api/v1/network-device/assign-group", "PUT"}, {"creator", "/api/v1/network-device-group", "POST"}, {"editor", "/api/v1/network-device-group/:id", "PUT"}, {"remover", "/api/v1/network-device-group/:id", "DELETE"}} {
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
	RegisterInventory(engine, auth, netdisco.FromEnvironment(), zabbix.FromEnvironment())
	for _, test := range []struct {
		role, method, path, body string
		code                     int
	}{
		{"detail", "GET", "/api/v1/network-device/1", "", 200},
		{"detail", "GET", "/api/v1/network-device/options", "", 403}, {"detail", "GET", "/api/v1/network-device/export", "", 403},
		{"detail", "PUT", "/api/v1/network-device/assign-group", `{"devices":[{"id":1,"version":1}],"groupSource":"manual","groupId":null}`, 403},
		{"detail", "GET", "/api/v1/network-device-group", "", 403}, {"tree", "GET", "/api/v1/network-device-group", "", 200},
		{"tree", "POST", "/api/v1/network-device-group", `{}`, 403}, {"creator", "POST", "/api/v1/network-device-group", `{"code":"new","name":"New","enabled":true,"position":0}`, 200},
		{"editor", "PUT", "/api/v1/network-device-group/999", `{"code":"new","name":"New","version":1}`, 404},
		{"remover", "DELETE", "/api/v1/network-device-group/1", `{"version":1}`, 409},
		{"assign", "PUT", "/api/v1/network-device/assign-group", `{"devices":[{"id":1,"version":1},{"id":2,"version":1}],"groupSource":"manual","groupId":null}`, 404},
		{"assign", "PUT", "/api/v1/network-device/assign-group", `{"devices":[{"id":1,"version":1}],"groupSource":"manual","groupId":null}`, 200},
	} {
		token, _, err := auth.TokenGenerator(jwt.MapClaims{"identity": 7, "rolekey": test.role, "roleid": 2, "deptid": 3, "datascope": actions.DataScopeSelf})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		req.Host = host
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		var result struct {
			Code int
			Data struct {
				TotalDeviceCount     int64
				UngroupedDeviceCount int64
				Items                []struct {
					DeviceCount       int64
					DirectDeviceCount int64
				}
			}
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Code != test.code {
			t.Errorf("%s %s %s: %s", test.role, test.method, test.path, response.Body.String())
		}
		if test.role == "tree" && test.method == "GET" && (result.Data.TotalDeviceCount != 1 || result.Data.UngroupedDeviceCount != 0 || len(result.Data.Items) != 1 || result.Data.Items[0].DeviceCount != 1 || result.Data.Items[0].DirectDeviceCount != 1) {
			t.Errorf("tree leaked scope: %s", response.Body.String())
		}
	}
}
