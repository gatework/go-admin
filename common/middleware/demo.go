package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/response"
	"github.com/gatework/motor-core/sdk/config"
)

// defaultDemoMsg is what a refused request is told when nothing is configured.
//
// It is the string this middleware used to carry hard-coded, kept verbatim so
// that a deployment which never set application.demomsg is answered exactly as
// it was before.
const defaultDemoMsg = "谢谢您的参与，但为了大家更好的体验，所以本次提交就算了吧！\U0001F600\U0001F600\U0001F600"

// demoAllows reports whether demo mode lets a request through.
//
// route is the matched gin route pattern and uri the raw request target; the
// two are different things and both are needed. The route is what identifies a
// handler regardless of the values in its path parameters, and it is empty for
// a request that matched nothing - which is why the login and logout checks
// still read the raw target, as they always did.
func demoAllows(method, route, uri string) bool {
	return method == http.MethodGet ||
		method == http.MethodOptions ||
		uri == "/api/v1/login" ||
		uri == "/api/v1/logout"
}

// demoMessage is the answer a refused request gets.
//
// application.demomsg has been in the configuration all along and nothing read
// it: the message was hard-coded here, and the demo host's configured string
// happened to be identical, so the setting looked like it worked. An empty
// value falls back rather than answering with nothing.
func demoMessage() string {
	if msg := config.Current().Settings.Application.DemoMsg; msg != "" {
		return msg
	}
	return defaultDemoMsg
}

// DemoEnv refuses anything that would change state while mode is demo.
func DemoEnv() gin.HandlerFunc {
	return func(c *gin.Context) {
		if config.Current().Settings.Application.Mode != "demo" {
			c.Next()
			return
		}
		if demoAllows(c.Request.Method, c.FullPath(), c.Request.RequestURI) {
			c.Next()
			return
		}
		response.Error(c, http.StatusForbidden, nil, demoMessage())
	}
}
