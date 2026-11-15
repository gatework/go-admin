package middleware

import (
	"errors"
	"net/http"
	"runtime/debug"
	"syscall"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk/api"
)

// Recover turns a panic in any later middleware or handler into a logged 500
// response. It must be the first middleware on the engine: gin.New installs no
// recovery, and CustomError deliberately re-panics values outside its own
// protocol, so without it one bad request tears down its connection with no
// response and no request-scoped log line.
func Recover(c *gin.Context) {
	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}
		if err, ok := recovered.(error); ok {
			// net/http uses this sentinel to abort a response on purpose.
			if errors.Is(err, http.ErrAbortHandler) {
				panic(recovered)
			}
			// The client is gone; there is nobody to answer.
			if errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) {
				c.Abort()
				return
			}
		}
		api.GetRequestLogger(c).Errorf("panic recovered: method=%s path=%s panic=%v\n%s", c.Request.Method, redactLogURL(c.Request.URL), recovered, debug.Stack())
		if c.Writer.Written() {
			c.Abort()
			return
		}
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"code": http.StatusInternalServerError, "msg": "internal server error"})
	}()
	c.Next()
}
