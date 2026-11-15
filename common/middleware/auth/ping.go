package auth

import (
	"github.com/gin-gonic/gin"
)

// Ping handles the ping operation.
func Ping(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "ok",
	})
}
