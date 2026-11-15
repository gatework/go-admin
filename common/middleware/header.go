package middleware

import (
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor/config"
)

// NoCache is a middleware function that appends headers
// to prevent the client from caching the HTTP response.
func NoCache(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, no-store, max-age=0, must-revalidate")
	c.Header("Expires", "Thu, 01 Jan 1970 00:00:00 GMT")
	c.Header("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	c.Next()
}

// CORS grants cross-origin access to the origins listed in extend.cors.origins
// and answers every preflight.
//
// The browser UI is normally served from the API's own origin and needs no
// CORS at all; a development UI on another port is the usual exception. Every
// response used to carry Access-Control-Allow-Origin: *, opening every route
// to scripts on any site. An origin not on the list gets no CORS headers, so
// the browser refuses it.
func CORS(c *gin.Context) {
	origin := c.GetHeader("Origin")
	allowed := origin != "" && slices.Contains(config.Ext().CORS.Origins, origin)
	if allowed {
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Access-Control-Expose-Headers", "X-Request-Id")
	}
	c.Writer.Header().Add("Vary", "Origin")
	if c.Request.Method != http.MethodOptions {
		c.Next()
		return
	}
	if allowed {
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "authorization, origin, content-type, accept, x-request-id")
		c.Header("Access-Control-Max-Age", "600")
	}
	c.Header("Allow", "HEAD,GET,POST,PUT,PATCH,DELETE,OPTIONS")
	c.AbortWithStatus(http.StatusNoContent)
}

// Secure is a middleware function that appends security
// and resource access headers.
func Secure(c *gin.Context) {
	// Uploaded content must not execute in the administration origin, including
	// previously uploaded files. Raster images remain usable as avatars.
	// Filesystems may resolve path components without regard to case.
	if strings.HasPrefix(strings.ToLower(path.Clean(c.Request.URL.Path)), "/static/uploadfile/") {
		c.Header("Content-Security-Policy", "sandbox; default-src 'none'")
		switch strings.ToLower(path.Ext(c.Request.URL.Path)) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".ico":
		default:
			c.Header("Content-Disposition", "attachment")
		}
	}
	//c.Header("X-Frame-Options", "DENY")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-XSS-Protection", "1; mode=block")
	if c.Request.TLS != nil {
		c.Header("Strict-Transport-Security", "max-age=31536000")
	}

	// Also consider adding Content-Security-Policy headers
	// c.Header("Content-Security-Policy", "script-src 'self' https://cdnjs.cloudflare.com")
}
