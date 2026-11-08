package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type deliveryReadProbe struct {
	io.Reader
	reads int
}

func (p *deliveryReadProbe) Read(b []byte) (int, error) { p.reads++; return p.Reader.Read(b) }

type deliveryResultProbe struct{ marshals *int }

func (p deliveryResultProbe) MarshalJSON() ([]byte, error) {
	*p.marshals++
	return []byte(`{"script":"secret","output":"secret"}`), nil
}
func TestDeliveryBodyAndResultNeverEnterOperationLog(t *testing.T) {
	old := config.LoggerConfig.EnabledDB
	config.LoggerConfig.EnabledDB = true
	defer func() { config.LoggerConfig.EnabledDB = old }()
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"/api/v1/network-delivery/orders", "/api/v1/network-delivery/orders/1/targets/2", "/api/v1/network-delivery/devices/1/connection"} {
		t.Run(path, func(t *testing.T) {
			probe := &deliveryReadProbe{Reader: strings.NewReader(`{"script":"secret"}`)}
			marshals := 0
			r := gin.New()
			r.Use(LoggerToFile())
			r.POST(path, func(c *gin.Context) {
				c.Set("result", deliveryResultProbe{marshals: &marshals})
				c.Status(http.StatusNotFound)
			})
			r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, path, probe))
			if probe.reads != 0 || marshals != 0 {
				t.Fatalf("sensitive payload logged: reads=%d marshals=%d", probe.reads, marshals)
			}
		})
	}
}
