package middleware

import (
	"net/http"
	"runtime"
	"strconv"
	"strings"

	"github.com/gatework/motor-core/sdk/api"

	"github.com/gin-gonic/gin"
)

// CustomError handles the customerror operation.
func CustomError(c *gin.Context) {
	defer func() {
		if err := recover(); err != nil {

			if c.IsAborted() {
				c.Status(200)
			}
			switch errStr := err.(type) {
			case string:
				p := strings.Split(errStr, "#")
				if len(p) == 3 && p[0] == "CustomError" {
					statusCode, e := strconv.Atoi(p[1])
					if e != nil {
						break
					}
					c.Status(statusCode)
					api.GetRequestLogger(c).Errorf("request failed: method=%s path=%s status=%d", c.Request.Method, redactLogURL(c.Request.URL), statusCode)
					c.JSON(http.StatusOK, gin.H{
						"code": statusCode,
						"msg":  p[2],
					})
				} else {
					c.JSON(http.StatusOK, gin.H{
						"code": 500,
						"msg":  errStr,
					})
				}
			case runtime.Error:
				c.JSON(http.StatusOK, gin.H{
					"code": 500,
					"msg":  errStr.Error(),
				})
			default:
				panic(err)
			}
		}
	}()
	c.Next()
}
