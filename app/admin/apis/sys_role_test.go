package apis

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg"
	"gorm.io/gorm"

	"go-admin/app/admin/models"
)

func TestRoleUpdateServiceFailureHasStructuredResponse(t *testing.T) {
	db, tenant := setupPrivescDB(t)
	if err := db.AutoMigrate(&models.SysRole{}, &models.SysMenu{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.SysRole{RoleId: 1, RoleKey: "original", DataScope: "1"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("role:fail_update", func(tx *gorm.DB) { tx.AddError(errors.New("injected role update failure")) }); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set(pkg.LoggerKey, logger.NewHelper(logger.DefaultLogger))
		c.Next()
	})
	r.PUT("/role/:id", SysRole{}.Update)
	req := httptest.NewRequest(http.MethodPut, "/role/1", strings.NewReader(`{"roleKey":"original","roleName":"changed","dataScope":"1"}`))
	req.Host = tenant
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not structured JSON: status=%d body=%q error=%v", rec.Code, rec.Body.String(), err)
	}
	if body.Code != 500 || body.Msg == "" {
		t.Fatalf("missing service failure response: %s", rec.Body.String())
	}
}
