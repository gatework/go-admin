package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Motor serves the local backend landing page without external dependencies.
func Motor(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(`<!doctype html><html lang="en"><meta charset="utf-8"><title>motor</title><body><main><h1>motor</h1><p>Network operations backend</p></main></body></html>`))
}
