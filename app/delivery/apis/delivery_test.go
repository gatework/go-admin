package apis

import (
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"go-admin/app/delivery/models"
	"go-admin/app/delivery/service"
	inventory "go-admin/app/network/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"net/http/httptest"
	"strings"
	"testing"
)

func engine(t *testing.T) *gin.Engine {
	r, _ := engineWithScope(t, &actions.DataPermission{DataScope: actions.DataScopeAll, UserId: 7})
	return r
}
func engineWithScope(t *testing.T, permission *actions.DataPermission) (*gin.Engine, *gorm.DB) {
	t.Helper()
	t.Setenv("NETWORK_DELIVERY_ENABLED", "true")
	t.Setenv("NC_DEVICE_USERNAME", "fixture")
	t.Setenv("NC_DEVICE_PASSWORD", "fixture-secret")
	t.Setenv("NC_H3C_USERNAME", "fixture")
	t.Setenv("NC_H3C_PASSWORD", "fixture-secret")
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	if e = db.AutoMigrate(append(service.Tables(), &inventory.Device{}, &inventory.DeviceGroup{})...); e != nil {
		t.Fatal(e)
	}
	d := inventory.Device{SourceFacts: inventory.SourceFacts{IP: "192.0.2.1", Name: "fixture", SourceTags: []string{}}, Status: "active", Tags: []string{}}
	d.CreateBy = 7
	if e = db.Create(&d).Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Create(&models.Connection{DeviceID: d.Id, Profile: "h3c", Protocol: "ssh", Port: 22, Version: 1}).Error; e != nil {
		t.Fatal(e)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set(jwt.JwtPayloadKey, jwt.MapClaims{"identity": float64(7), "rolekey": "operator", "nice": "tester"})
		c.Set(actions.PermissionKey, permission)
		c.Next()
	})
	a := Delivery{}
	r.GET("/options", a.Options)
	r.GET("/devices", a.Devices)
	r.GET("/orders", a.Page)
	r.POST("/orders", a.Create)
	r.GET("/orders/:id", a.Get)
	r.PUT("/orders/:id", a.Edit)
	r.POST("/orders/:id/cancel", a.Cancel)
	return r, db
}
func call(t *testing.T, r *gin.Engine, method, path, body string) map[string]any {
	t.Helper()
	q := httptest.NewRequest(method, path, strings.NewReader(body))
	q.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, q)
	var out map[string]any
	if e := json.Unmarshal(w.Body.Bytes(), &out); e != nil {
		t.Fatal(e)
	}
	return out
}
func TestAPIRejectsInvalidPaginationAndUnknownOrLargeBodies(t *testing.T) {
	r := engine(t)
	for _, path := range []string{"/devices?pageSize=201", "/orders?pageSize=201"} {
		out := call(t, r, "GET", path, "")
		if out["code"] != float64(400) {
			t.Fatalf("invalid pagination %+v", out)
		}
	}
	for _, body := range []string{`{"requestKey":"request-00001","title":"x","targets":[],"password":"secret"}`, `{} {}`, `{"title":"` + strings.Repeat("x", (service.MaxTotalBytes*6)+(64<<10)) + `"}`} {
		out := call(t, r, "POST", "/orders", body)
		if out["code"] != float64(400) {
			t.Fatal("body accepted")
		}
		if strings.Contains(out["msg"].(string), "secret") {
			t.Fatal("unsafe error")
		}
	}
}
func TestAPIOrderEnvelopeDatesAndIdempotency(t *testing.T) {
	r := engine(t)
	body := `{"requestKey":"request-00001","title":"fixture","targets":[{"deviceId":1,"script":"system-view\nsysname test"}]}`
	out := call(t, r, "POST", "/orders", body)
	if out["code"] != float64(200) {
		t.Fatal(out)
	}
	data := out["data"].(map[string]any)
	if data["status"] != "scheduled" || data["version"] != float64(1) || data["createdBy"] != float64(7) || !strings.Contains(data["scheduledAt"].(string), "T") {
		t.Fatal(data)
	}
	target := data["targets"].([]any)[0].(map[string]any)
	if _, ok := target["output"]; ok {
		t.Fatal("output in order")
	}
	if target["scriptSha256"] == "" || target["scriptLineCount"] != float64(2) {
		t.Fatal(target)
	}
	again := call(t, r, "POST", "/orders", body)
	if again["data"].(map[string]any)["id"] != data["id"] {
		t.Fatal("double submission")
	}
	out = call(t, r, "PUT", "/orders/1", `{"version":99,"title":"changed","targets":[{"deviceId":1,"script":"system-view"}]}`)
	if out["code"] != float64(409) {
		t.Fatal("stale edit accepted", out)
	}
	out = call(t, r, "GET", "/orders", "")
	page := out["data"].(map[string]any)
	if page["count"] != float64(1) || page["pageIndex"] != float64(1) || len(page["list"].([]any)) != 1 {
		t.Fatal(page)
	}
}

func TestOptionsInventoryGroupsAreScopedAndRetainUnclassifiedDevices(t *testing.T) {
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	r, db := engineWithScope(t, &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 7})
	for i, owner := range []int{7, 7, 8, 7} {
		d := inventory.Device{SourceFacts: inventory.SourceFacts{IP: []string{"192.0.2.2", "192.0.2.3", "192.0.2.4", "192.0.2.5"}[i], Vendor: "Cisco", HardwareModel: "N9K", SourceTags: []string{}}, DeviceType: "switch", DeviceRole: "leaf_switch", Status: "active", Tags: []string{}}
		d.CreateBy = owner
		if err := db.Create(&d).Error; err != nil {
			t.Fatal(err)
		}
		if i == 3 {
			if err := db.Delete(&d).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	out := call(t, r, "GET", "/options", "")
	data, ok := out["data"].(map[string]any)
	if !ok || out["code"] != float64(200) {
		t.Fatalf("options: %+v", out)
	}
	groups, ok := data["inventoryGroups"].([]any)
	if !ok || len(groups) != 2 {
		t.Fatalf("scoped groups: %+v", data)
	}
	blank, named := groups[0].(map[string]any), groups[1].(map[string]any)
	if blank["deviceType"] != "" || blank["deviceRole"] != "" || blank["vendor"] != "" || blank["model"] != "" || blank["count"] != float64(1) || named["deviceType"] != "switch" || named["deviceRole"] != "leaf_switch" || named["vendor"] != "Cisco" || named["model"] != "N9K" || named["count"] != float64(2) {
		t.Fatalf("wrong groups: %+v", groups)
	}
}

func TestOptionsReusesScopedDeviceGroupTree(t *testing.T) {
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	r, db := engineWithScope(t, &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 7})
	region := inventory.DeviceGroup{Code: "region", Name: "Region", Enabled: true}
	if err := db.Create(&region).Error; err != nil {
		t.Fatal(err)
	}
	campus := inventory.DeviceGroup{Code: "campus", Name: "Campus", ParentID: &region.Id, Enabled: true}
	if err := db.Create(&campus).Error; err != nil {
		t.Fatal(err)
	}
	for i, owner := range []int{7, 7, 8} {
		group := region.Id
		if i > 0 {
			group = campus.Id
		}
		d := inventory.Device{SourceFacts: inventory.SourceFacts{IP: []string{"192.0.2.2", "192.0.2.3", "192.0.2.4"}[i], SourceTags: []string{}}, GroupID: &group, Status: "active", Tags: []string{}}
		d.CreateBy = owner
		if err := db.Create(&d).Error; err != nil {
			t.Fatal(err)
		}
	}
	out := call(t, r, "GET", "/options", "")
	data, ok := out["data"].(map[string]any)
	if !ok || out["code"] != float64(200) {
		t.Fatalf("options: %+v", out)
	}
	tree, ok := data["deviceGroups"].(map[string]any)
	if !ok || tree["totalDeviceCount"] != float64(3) || tree["ungroupedDeviceCount"] != float64(1) {
		t.Fatalf("scoped group tree: %+v", data)
	}
	roots, ok := tree["items"].([]any)
	if !ok || len(roots) != 1 {
		t.Fatalf("group roots: %+v", tree)
	}
	root := roots[0].(map[string]any)
	children, ok := root["children"].([]any)
	if !ok || len(children) != 1 || root["name"] != "Region" || root["directDeviceCount"] != float64(1) || root["deviceCount"] != float64(2) {
		t.Fatalf("region counts: %+v", root)
	}
	child := children[0].(map[string]any)
	if child["name"] != "Campus" || child["deviceCount"] != float64(1) || child["directDeviceCount"] != float64(1) {
		t.Fatalf("child scope: %+v", child)
	}
}
