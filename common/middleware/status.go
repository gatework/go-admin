package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// statusKey is where motor-core's response helpers record the status of the
// envelope they write ({code,msg,data}). They always send HTTP 200 and put the
// real outcome only in the body.
const statusKey = "status"

// ErrorStatus sends an error envelope's status on the HTTP status line too.
//
// motor-core writes every envelope with HTTP 200, so load balancers, metrics,
// client retry logic and access logs counted every failure as served. Rather
// than rewrite each handler, this wraps the response writer: when a handler
// asks for 200 but has recorded a 4xx or 5xx envelope status, the error status
// is sent instead. Successful responses and anything that sets a status of its
// own are left alone. It must be installed before any handler writes.
func ErrorStatus(c *gin.Context) {
	c.Writer = &statusWriter{ResponseWriter: c.Writer, context: c}
	c.Next()
}

type statusWriter struct {
	gin.ResponseWriter
	context *gin.Context
}

func (w *statusWriter) WriteHeader(code int) {
	if code == http.StatusOK {
		if recorded, ok := w.context.Get(statusKey); ok {
			if status, ok := recorded.(int); ok && status >= http.StatusBadRequest && status <= 599 {
				code = status
			}
		}
	}
	w.ResponseWriter.WriteHeader(code)
}
