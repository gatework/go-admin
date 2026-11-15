package dto_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/jwtauth"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
	"github.com/gatework/motor/common/actions"
)

// The login log's time range filter named a ctime column the table does not
// have; any request using it failed with a SQL error.
func TestLoginLogFiltersByTimeRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SysLoginLog{}); err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)} {
		if err := db.Create(&models.SysLoginLog{Username: "admin", CreatedAt: at}).Error; err != nil {
			t.Fatal(err)
		}
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set(jwtauth.JwtPayloadKey, jwtauth.MapClaims{"identity": float64(7)})
	}, actions.Unscoped)
	engine.GET("/sys-login-log", actions.Index[models.SysLoginLog, dto.LoginLogSearch]())
	engine.DELETE("/sys-login-log", actions.Delete[models.SysLoginLog, dto.LoginLogByID]())

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/sys-login-log?pageIndex=1&pageSize=10&beginTime=2026-03-01&endTime=2026-12-31", nil))
	if !strings.Contains(response.Body.String(), `"count":1`) {
		t.Fatalf("time range filter: %s", response.Body.String())
	}
	request := httptest.NewRequest(http.MethodDelete, "/sys-login-log", strings.NewReader(`{"ids":[1,2]}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	var left int64
	db.Model(&models.SysLoginLog{}).Count(&left)
	if left != 0 {
		t.Fatalf("delete left %d rows: %s", left, response.Body.String())
	}
}
