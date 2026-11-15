package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gatework/motor-core/sdk/api"

	"github.com/gin-gonic/gin"
)

// CustomError answers the "CustomError#code#msg" string-panic protocol used by
// legacy generator handlers and rethrows every other panic to Recover, which is
// installed ahead of it and answers 500 without echoing panic detail.
func CustomError(c *gin.Context) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		statusCode, message, ok := customErrorPanic(recovered)
		if !ok {
			panic(recovered)
		}
		api.GetRequestLogger(c).Errorf("request failed: method=%s path=%s status=%d", c.Request.Method, redactLogURL(c.Request.URL), statusCode)
		c.JSON(http.StatusOK, gin.H{"code": statusCode, "msg": message})
	}()
	c.Next()
}

func customErrorPanic(recovered any) (statusCode int, message string, ok bool) {
	text, isString := recovered.(string)
	if !isString {
		return 0, "", false
	}
	parts := strings.Split(text, "#")
	if len(parts) != 3 || parts[0] != "CustomError" {
		return 0, "", false
	}
	statusCode, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, "", false
	}
	return statusCode, parts[2], true
}
