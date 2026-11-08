package apis

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"go-admin/app/diagnostics/service"
)

func TestDiagnosticsRejectsMalformedBodyAndUnknownJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := service.NewManager()
	defer manager.Close()
	api := Diagnostics{Manager: manager}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(jwt.JwtPayloadKey, jwt.MapClaims{"identity": float64(7), "rolekey": "operator"})
		c.Next()
	})
	engine.POST("/jobs", api.Start)
	engine.GET("/jobs/:id", api.Get)
	engine.GET("/server", api.Server)
	for _, body := range []string{`{}`, `{"tool":"ping","target":"--help"}`, `{"tool":"ping","target":"localhost","args":"bad"}`, `{"tool":"ping","target":"localhost"} {}`} {
		response := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/jobs", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(response, req)
		var result struct{ Code int }
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Code != 400 {
			t.Fatalf("body %s: %s", body, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("GET", "/jobs/missing", nil))
	var result struct{ Code int }
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != 404 {
		t.Fatalf("unknown job: %s", response.Body.String())
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("GET", "/server", nil))
	if !strings.Contains(response.Body.String(), `"id":"backend"`) || !strings.Contains(response.Body.String(), `"sessionSeconds":120`) {
		t.Fatal(response.Body.String())
	}
}
