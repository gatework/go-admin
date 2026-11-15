package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk"
)

// WithContextDB handles the withcontextdb operation.
func WithContextDB(c *gin.Context) {
	db := sdk.Runtime.DB(c.Request.Host)
	if db == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "租户数据库不可用"})
		return
	}
	c.Set("db", db.WithContext(c.Request.Context()))
	c.Next()
}
