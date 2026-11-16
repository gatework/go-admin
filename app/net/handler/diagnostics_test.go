package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/gatework/motor/app/net/models/diagnosticrun"
	"github.com/gatework/motor/common/account"

	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	service "github.com/gatework/motor/app/net/service/diagnostics"
)

func TestDiagnosticsRejectsMalformedBodyAndUnknownJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := service.NewManager()
	defer manager.Close()
	api := Diagnostics{Manager: manager}
	engine := gin.New()
	diagnosticsDB(t, engine)
	engine.Use(func(c *gin.Context) {
		c.Set(jwt.JwtPayloadKey, jwt.MapClaims{"identity": float64(7), "rolekey": "operator"})
		c.Next()
	})
	engine.POST("/jobs", api.Start)
	engine.GET("/jobs/:id", api.Get)
	engine.GET("/server", api.Server)
	for _, body := range []string{`{}`, `{"tool":"ping","target":"--help"}`, `{"tool":"ping","target":"localhost","args":"bad"}`, `{"tool":"ping","target":"localhost"} {}`} {
		response := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(response, req)
		var result struct {
			Code int
			Msg  string
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Code != 400 || result.Msg != "diagnostics.invalid_input" {
			t.Fatalf("body %s: %s", body, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/jobs/missing", nil))
	var result struct {
		Code int
		Msg  string
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != 404 || result.Msg != "diagnostics.not_found" {
		t.Fatalf("unknown job: %s", response.Body.String())
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server", nil))
	if !strings.Contains(response.Body.String(), `"id":"backend"`) || !strings.Contains(response.Body.String(), `"sessionSeconds":120`) {
		t.Fatal(response.Body.String())
	}
}

// diagnosticsDB gives every request the tenant database diagnostics audits
// and rate-limits in.
func diagnosticsDB(t *testing.T, engine *gin.Engine) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "diagnostics.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&diagnosticrun.Record{}, &account.Limit{}); err != nil {
		t.Fatal(err)
	}
	engine.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
}
