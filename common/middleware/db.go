package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"net/http"
)

func WithContextDb(c *gin.Context) {
	db := sdk.Runtime.GetDbByTenant(c.Request.Host)
	if db == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 503, "msg": "租户数据库不可用"})
		return
	}
	c.Set("db", db.WithContext(c.Request.Context()))
	c.Next()
}
