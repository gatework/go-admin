package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/config"

	ext "github.com/gatework/motor/config"
	"github.com/gatework/motor/internal/testconfig"
)

// The client address goes into the login log, the operation log and the login
// rate limit. gin believes X-Forwarded-For from anyone by default, so a client
// could write any address into the audit trail and dodge the limit. Only a
// configured proxy is believed now.
func TestClientAddressIgnoresForwardedHeadersFromUntrustedPeers(t *testing.T) {
	for name, tc := range map[string]struct {
		trusted []string
		want    string
	}{
		"no proxies configured":       {nil, "198.51.100.7"},
		"the peer is a trusted proxy": {[]string{"198.51.100.0/24"}, "203.0.113.9"},
	} {
		t.Run(name, func(t *testing.T) {
			freshRuntime(t)
			testconfig.Apply(t, func(settings *config.Config) {
				settings.
					JWT.Secret = "client-ip-test-secret"
			})
			previous := ext.Ext()
			t.Cleanup(func() { ext.SetExt(previous) })
			ext.SetExt(ext.Extend{TrustedProxies: tc.trusted})
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sdk.Runtime.SetDB("*", db)
			if err := initRouter(); err != nil {
				t.Fatal(err)
			}
			engine := sdk.Runtime.Engine().(*gin.Engine)
			var seen string
			engine.GET("/whoami", func(c *gin.Context) { seen = c.ClientIP() })
			request := httptest.NewRequest(http.MethodGet, "/whoami", nil)
			request.RemoteAddr = "198.51.100.7:40000"
			request.Header.Set("X-Forwarded-For", "203.0.113.9")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if seen != tc.want {
				t.Fatalf("client address %q, want %q (status %d)", seen, tc.want, response.Code)
			}
		})
	}
}
