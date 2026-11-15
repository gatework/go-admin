package middleware

import (
	"net/http"
	"strings"

	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/pkg"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestID 自动增加requestId
func RequestID(trafficKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		requestID := c.GetHeader(trafficKey)
		if requestID == "" {
			requestID = c.GetHeader(strings.ToLower(trafficKey))
		}
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Request.Header.Set(trafficKey, requestID)
		c.Set(trafficKey, requestID)
		c.Set(pkg.LoggerKey,
			logger.NewHelper(logger.Default()).
				WithFields(map[string]any{
					trafficKey: requestID,
				}))
		c.Next()
	}
}
