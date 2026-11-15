package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/unrolled/secure"

	"github.com/gatework/motor-core/sdk/config"
)

// TLSHandler handles the tlshandler operation.
func TLSHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		secureMiddleware := secure.New(secure.Options{
			SSLRedirect: true,
			SSLHost:     config.Current().Settings.SSL.Domain,
		})
		err := secureMiddleware.Process(c.Writer, c.Request)
		if err != nil {
			return
		}
		c.Next()
	}
}
