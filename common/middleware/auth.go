package middleware

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	jwt "github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/pkg"
	"github.com/gatework/motor/common/middleware/auth"
)

// authMiddleware is the single JWT middleware instance the whole process
// shares. InitMiddleware builds it once, before any module registers its
// routes; GetAuthMiddleware is how a module gets it back instead of calling
// AuthInit itself and building another, functionally-equivalent-but-distinct
// instance.
var authMiddleware *jwt.GinJWTMiddleware

// minProductionSecret is the shortest HMAC-SHA256 signing key accepted in
// production: a key shorter than the hash output can be brute-forced offline
// from any token the server has issued.
const minProductionSecret = 32

// defaultTokenTimeout applies when jwt.timeout is not set.
const defaultTokenTimeout = time.Hour

// AuthInit builds the JWT middleware from the current configuration.
//
// An empty secret is refused in every mode: core only rejects a nil key, so
// an empty one used to sign tokens with no key at all, and anyone could mint
// an administrator's token. Production also needs a key of at least
// minProductionSecret bytes. Tokens live for jwt.timeout in every mode; dev
// used to issue tokens valid for a hundred years.
func AuthInit() (*jwt.GinJWTMiddleware, error) {
	settings := config.Current().Settings
	secret := settings.JWT.Secret
	if err := checkSecret(settings.Application.Mode, secret); err != nil {
		return nil, err
	}
	timeout := defaultTokenTimeout
	if settings.JWT.Timeout > 0 {
		timeout = time.Duration(settings.JWT.Timeout) * time.Second
	}
	payload, authorize := auth.Callbacks(secret)
	return jwt.New(&jwt.GinJWTMiddleware{
		Realm:           "motor",
		Key:             []byte(secret),
		Timeout:         timeout,
		MaxRefresh:      time.Hour,
		PayloadFunc:     payload,
		IdentityHandler: auth.IdentityHandler,
		Authenticator:   auth.Authenticator,
		Authorizator:    authorize,
		Unauthorized:    auth.Unauthorized,
		// No query parameter: a token in a URL lands in access logs, proxy
		// logs and browser history. The log websocket, which a browser cannot
		// give headers, goes through WebsocketToken instead.
		TokenLookup:   "header: Authorization, cookie: jwt",
		TokenHeadName: "Bearer",
		TimeFunc:      time.Now,
	})

}

func checkSecret(mode, secret string) error {
	switch {
	case secret == "":
		return errors.New("jwt.secret is empty: tokens would be signed with no key")
	case mode == pkg.ModeProd.String() && len(secret) < minProductionSecret:
		return fmt.Errorf("jwt.secret is %d bytes; production needs at least %d", len(secret), minProductionSecret)
	default:
		return nil
	}
}

// WebsocketToken lets a browser authenticate a websocket upgrade with
// ?token=, the one place it cannot send an Authorization header. It moves the
// token into the header and removes it from the URL before anything logs it.
// Install it before the JWT middleware on websocket routes only.
func WebsocketToken(c *gin.Context) {
	query := c.Request.URL.Query()
	token := query.Get("token")
	if token != "" && c.GetHeader("Authorization") == "" && strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
		c.Request.Header.Set("Authorization", "Bearer "+token)
	}
	if query.Has("token") {
		query.Del("token")
		c.Request.URL.RawQuery = query.Encode()
	}
	c.Next()
}

// GetAuthMiddleware returns the shared JWT middleware instance InitMiddleware
// built at startup. Application modules (app/admin, app/jobs, app/other,
// app/demo) call this instead of AuthInit so their router chains - which
// still need the instance itself for authMiddleware.MiddlewareFunc() and
// authMiddleware.LoginHandler, not just the bound closure registered under
// sdk.Runtime's JwtTokenCheck key - end up using the same instance the host
// registered, rather than one each.
//
// It fails loudly instead of returning nil: an InitRouter that runs before
// InitMiddleware has a real startup-ordering bug, not a case to paper over
// with a nil *jwt.GinJWTMiddleware that would panic much further down the
// call chain with a far less useful stack trace.
func GetAuthMiddleware() *jwt.GinJWTMiddleware {
	if authMiddleware == nil {
		// A startup-ordering bug, not a runtime condition: panic rather than
		// log.Fatal, which would skip every deferred cleanup on the way out.
		panic("JWT middleware not initialized; InitMiddlewareWithDatabase must run before any module's InitRouter")
	}
	return authMiddleware
}
