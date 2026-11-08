package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
)

func TestFabricEvidenceNeverEntersOperationLog(t *testing.T) {
	old := config.LoggerConfig.EnabledDB
	config.LoggerConfig.EnabledDB = true
	defer func() { config.LoggerConfig.EnabledDB = old }()
	for _, path := range []string{"/api/v1/network-fabrics", "/api/v1/network-fabrics/1", "/api/v1/network-fabrics/changes/1/execute"} {
		probe := &deliveryReadProbe{Reader: strings.NewReader(`{"evidence":"private"}`)}
		marshals := 0
		r := gin.New()
		r.Use(LoggerToFile())
		r.POST(path, func(c *gin.Context) {
			c.Set("result", deliveryResultProbe{marshals: &marshals})
			c.Status(http.StatusNotFound)
		})
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, path, probe))
		if probe.reads != 0 || marshals != 0 {
			t.Fatalf("Fabric payload logged on %s", path)
		}
	}
}
