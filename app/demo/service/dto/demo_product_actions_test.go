package dto_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"gorm.io/gorm"

	demo "go-admin/app/demo/models"
	demodto "go-admin/app/demo/service/dto"
	"go-admin/common/actions"
)

func probeDB(t *testing.T, _ string, _ any) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}
func probeEngine(db *gorm.DB, register func(gin.IRoutes)) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set(jwt.JwtPayloadKey, jwt.MapClaims{"identity": float64(7)})
		c.Next()
	})
	register(r)
	return r
}
func serve(r *gin.Engine, method, path, body string) (int, string) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestDemoProductUpdateExplicitZero(t *testing.T) {
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = false
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	db := probeDB(t, t.Name(), nil)
	if err := db.AutoMigrate(&demo.DemoProduct{}); err != nil {
		t.Fatal(err)
	}
	product := demo.DemoProduct{Name: "widget", Code: "w", Price: 99, Remark: "old"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	r := probeEngine(db, func(r gin.IRoutes) { r.PUT("/product", actions.Update[demo.DemoProduct, demodto.DemoProductControl]()) })
	req := httptest.NewRequest(http.MethodPut, "/product", strings.NewReader(`{"id":1,"name":"widget","code":"w","price":0,"remark":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var after demo.DemoProduct
	if err := db.First(&after, product.Id).Error; err != nil {
		t.Fatal(err)
	}
	if after.Price != 0 || after.Remark != "" {
		t.Fatalf("explicit zero values ignored despite success: HTTP=%d body=%s price=%v remark=%q", rec.Code, rec.Body.String(), after.Price, after.Remark)
	}
}

func TestDemoProductCreateValidation(t *testing.T) {
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = false
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	db := probeDB(t, t.Name(), nil)
	if err := db.AutoMigrate(&demo.DemoProduct{}); err != nil {
		t.Fatal(err)
	}
	r := probeEngine(db, func(r gin.IRoutes) {
		r.POST("/product", actions.Create[demo.DemoProduct, demodto.DemoProductControl]())
	})
	req := httptest.NewRequest(http.MethodPost, "/product", strings.NewReader(`{"price":-1}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var count int64
	if err := db.Model(&demo.DemoProduct{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("required name/code and nonnegative price rules bypassed: HTTP=%d body=%s rows=%d", rec.Code, rec.Body.String(), count)
	}
}

// The URI selects the row; omitted business fields and creator metadata survive.
func TestGenericUpdatePreservesOmittedFields(t *testing.T) {
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = false
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	db := probeDB(t, t.Name(), nil)
	if err := db.AutoMigrate(&demo.DemoProduct{}); err != nil {
		t.Fatal(err)
	}
	product := demo.DemoProduct{Name: "widget", Code: "w", Price: 99, Status: "1", Remark: "keep"}
	product.CreateBy = 42
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	r := probeEngine(db, func(r gin.IRoutes) {
		r.PUT("/product/:id", actions.Update[demo.DemoProduct, demodto.DemoProductControl]())
	})
	_, body := serve(r, http.MethodPut, "/product/1", `{"id":999,"price":0,"createBy":999}`)
	var result struct{ Code int }
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != 200 {
		t.Fatalf("update failed: %s", body)
	}
	var after demo.DemoProduct
	if err := db.First(&after, product.Id).Error; err != nil {
		t.Fatal(err)
	}
	if after.Id != product.Id || after.Name != "widget" || after.Code != "w" || after.Price != 0 || after.Status != "1" || after.Remark != "keep" || after.CreateBy != 42 || after.UpdateBy != 7 {
		t.Fatalf("partial update lost data or attribution: %+v", after)
	}
}

func TestGenericDemoValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"missing name", `{"code":"w","price":1}`, 422},
		{"missing code", `{"name":"widget","price":1}`, 422},
		{"negative price", `{"name":"widget","code":"w","price":-1}`, 422},
		{"valid zero price", `{"name":"widget","code":"w","price":0}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := probeDB(t, strings.ReplaceAll(t.Name(), "/", "-"), nil)
			if err := db.AutoMigrate(&demo.DemoProduct{}); err != nil {
				t.Fatal(err)
			}
			r := probeEngine(db, func(r gin.IRoutes) {
				r.POST("/product", actions.Create[demo.DemoProduct, demodto.DemoProductControl]())
			})
			_, body := serve(r, http.MethodPost, "/product", tc.body)
			var result struct{ Code int }
			if err := json.Unmarshal([]byte(body), &result); err != nil {
				t.Fatal(err)
			}
			if result.Code != tc.want {
				t.Fatalf("response=%s want code %d", body, tc.want)
			}
		})
	}
}
