package apis

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg"
	"gorm.io/gorm"

	"go-admin/app/admin/models"
)

func configDictAPI(t *testing.T, withDB bool) (*gorm.DB, *gin.Engine) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.Close() })
	if err := db.AutoMigrate(&models.SysConfig{}, &models.SysDictType{}, &models.SysDictData{}); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if withDB {
			c.Set("db", db)
		}
		c.Set(pkg.LoggerKey, logger.NewHelper(logger.DefaultLogger))
		c.Set(jwt.JwtPayloadKey, jwt.MapClaims{"identity": float64(42)})
		c.Next()
	})
	r.GET("/config", SysConfig{}.GetPage)
	r.GET("/config/:id", SysConfig{}.Get)
	r.PUT("/config/:id", SysConfig{}.Update)
	r.GET("/app-config", SysConfig{}.Get2SysApp)
	r.PUT("/dict/type/:id", SysDictType{}.Update)
	r.PUT("/dict/data/:dictCode", SysDictData{}.Update)
	return db, r
}

func configDictAPICode(t *testing.T, r *gin.Engine, method, path, body string, want int) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var result struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("unstructured response: HTTP %d body=%q error=%v", rec.Code, rec.Body.String(), err)
	}
	if result.Code != want {
		t.Fatalf("response=%s want code %d", rec.Body.String(), want)
	}
}

func TestConfigAPIUsesPathIDAndAllowsBodylessGet(t *testing.T) {
	db, r := configDictAPI(t, true)
	rows := []models.SysConfig{{ConfigKey: "first", ConfigValue: "old"}, {ConfigKey: "second", ConfigValue: "old"}}
	rows[0].Id = 1
	rows[1].Id = 2
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	configDictAPICode(t, r, http.MethodPut, "/config/2", `{"id":1,"configKey":"second","configValue":"new","createBy":999}`, 200)
	var first, second models.SysConfig
	if err := db.First(&first, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&second, 2).Error; err != nil {
		t.Fatal(err)
	}
	if first.ConfigValue != "old" || second.ConfigValue != "new" || second.UpdateBy != 42 {
		t.Fatalf("wrong routed update: first=%+v second=%+v", first, second)
	}
	var count int64
	if err := db.Model(&models.SysConfig{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("update inserted extra config: count=%d", count)
	}
	configDictAPICode(t, r, http.MethodGet, "/config/2", "", 200)
}

func TestConfigAPIInitializationErrorsAreStructured(t *testing.T) {
	for _, path := range []string{"/config", "/app-config"} {
		t.Run(path, func(t *testing.T) {
			_, r := configDictAPI(t, false)
			configDictAPICode(t, r, http.MethodGet, path, "", 500)
		})
	}
}

func TestConfigDictAPIUpdateFailuresAreStructured(t *testing.T) {
	for _, tc := range []struct {
		path, body string
		row        any
	}{{"/config/1", `{"configKey":"one"}`, &models.SysConfig{ConfigKey: "one"}}, {"/dict/type/1", `{"dictType":"one"}`, &models.SysDictType{ID: 1, DictType: "one"}}, {"/dict/data/1", `{"dictType":"one"}`, &models.SysDictData{DictCode: 1, DictType: "one"}}} {
		t.Run(tc.path, func(t *testing.T) {
			db, r := configDictAPI(t, true)
			if _, isData := tc.row.(*models.SysDictData); isData {
				if err := db.Create(&models.SysDictType{DictType: "one"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Create(tc.row).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Callback().Update().Before("gorm:update").Register("evolution:fail_update", func(tx *gorm.DB) { tx.AddError(errors.New("injected update failure")) }); err != nil {
				t.Fatal(err)
			}
			configDictAPICode(t, r, http.MethodPut, tc.path, tc.body, 500)
		})
	}
}
