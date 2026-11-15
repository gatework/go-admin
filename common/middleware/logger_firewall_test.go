package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gatework/motor/internal/testconfig"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk/config"
)

func TestFirewallConfigAndInputAreExcludedFromOperationLog(t *testing.T) {

	testconfig.Apply(t, func(s *config.Config) { s.Logger.EnabledDB = true })

	for _, path := range []string{"/api/v1/network-firewall/instances/1/imports", "/api/v1/network-firewall/normalize", "/api/v1/network-firewall/analyses/1"} {
		body := &deliveryReadProbe{Reader: strings.NewReader(`{"text":"synthetic-secret"}`)}
		marshals := 0
		r := gin.New()
		r.Use(LoggerToFile())
		r.POST(path, func(c *gin.Context) {
			c.Set("result", deliveryResultProbe{marshals: &marshals})
			c.Status(http.StatusNotFound)
		})
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, path, body))
		if body.reads != 0 || marshals != 0 {
			t.Fatalf("firewall payload captured by logger: reads=%d marshals=%d", body.reads, marshals)
		}
	}
}
