package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/unrolled/secure"

	"github.com/gatework/motor-core/sdk/config"
)

// TLSRedirect redirects plain-HTTP requests to HTTPS on ssl.domain.
//
// The redirect is decided once per process: the options come from the
// configuration read at start-up, as the listener's TLS setting does. When it
// redirects, the chain stops. Returning without Abort let gin run every later
// handler after the redirect had already been written.
func TLSRedirect() gin.HandlerFunc {
	redirect := secure.New(secure.Options{
		SSLRedirect: true,
		SSLHost:     config.Current().Settings.SSL.Domain,
	})
	return func(c *gin.Context) {
		if err := redirect.Process(c.Writer, c.Request); err != nil {
			c.Abort()
			return
		}
		c.Next()
	}
}
