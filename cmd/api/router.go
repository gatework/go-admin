package api

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/runtime"

	otherrouter "github.com/gatework/motor/app/other/router"
	common "github.com/gatework/motor/common/middleware"
	ext "github.com/gatework/motor/config"
)

// Engine assembly: host middleware, then every module's routes.

// buildRouter announces BeforeRouter, builds the engine, and then drains the
// startup registries.
//
// The order is the contract. BeforeRouter is the last point at which a module
// can still affect how routes are built, so it has to run while there is no
// engine yet. The before registry runStartupHooks drains is a different moment
// despite the name: those callbacks run after initRouter has built the engine.
// Two lines apart, and describing them as equivalent is a mistake this
// repository has already made once in writing.
func buildRouter() error {
	sdk.Runtime.RunPhase(runtime.BeforeRouter)
	if err := initRouter(); err != nil {
		return fmt.Errorf("build router: %w", err)
	}
	runStartupHooks()
	return nil
}

// runStartupHooks runs the router registries and then the before callbacks.
//
// The package-level slice runs first and in its existing order, so a fork that
// only ever appended to AppRouters sees no change at all. The core registry
// runs second, through RunAppRouters: a module can register through
// sdk.Runtime.SetAppRouters and no longer has to import this command package -
// which is a main package's plumbing - just to be routed.
//
// The loop over the core registry now lives in core, which is what brings the
// panic guard and the registration seal with it. RunBefore closes a gap rather
// than moving one: the open-source edition never executed the before callbacks
// at all, so SetBefore was accepted and silently ignored. It has to stay ahead
// of ListenAndServe, because a callback registered WithFatal exits the process
// and that must not happen to one that is already serving.
func runStartupHooks() {
	for _, f := range AppRouters {
		f()
	}
	sdk.Runtime.RunAppRouters()
	sdk.Runtime.RunBefore()
}

func initRouter() error {
	h := sdk.Runtime.Engine()
	if h == nil {
		h = gin.New()
		sdk.Runtime.SetEngine(h)
	}
	r, ok := h.(*gin.Engine)
	if !ok {
		return fmt.Errorf("host engine is %T, not a gin engine", h)
	}
	// Client addresses feed audit logs and the login rate limit. gin trusts
	// every proxy by default, which lets any client name its own address in
	// X-Forwarded-For; only the configured proxies are believed.
	if err := r.SetTrustedProxies(ext.Ext().TrustedProxies); err != nil {
		return fmt.Errorf("extend.trustedProxies: %w", err)
	}
	// First, so that a panic anywhere below still produces a logged 500.
	r.Use(common.Recover)
	// Second, so that every error envelope written below - by Recover's
	// successors, middleware and handlers alike - carries its real HTTP status.
	r.Use(common.ErrorStatus)
	if config.Current().Settings.SSL.Enable {
		r.Use(common.TLSRedirect())
	}
	limiter, err := common.Sentinel()
	if err != nil {
		return err
	}
	r.Use(exemptProbes(limiter)).
		// Assigns or keeps the X-Request-Id, echoes it on the response and
		// binds a request logger carrying it.
		Use(api.SetRequestLogger)
	return common.InitMiddlewareWithDatabase(r, exemptProbes(common.WithContextDB))
}

// probePaths are the two routes the rate limiter must not answer for.
var probePaths = map[string]bool{
	otherrouter.APIPrefix + otherrouter.HealthPath: true,
	otherrouter.APIPrefix + otherrouter.ReadyPath:  true,
}

// exemptProbes wraps a middleware so the health and readiness routes skip it.
//
// The limiter is installed on the engine and the probes are routes like any
// other, so above the threshold they are answered with 429 as well. A liveness
// probe that collects 429s fails its threshold and the container is restarted,
// which takes capacity out of a deployment that is already short of it and
// pushes the rest closer to the threshold - the limiter working exactly as
// intended is what causes it. It is the argument common/health makes about
// restarting a process whose database is unreachable, applied to load.
//
// Wrapping rather than teaching the limiter about these paths: the limiter
// lives under common/, which may not import the package that registers them.
func exemptProbes(h gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if probePaths[c.FullPath()] {
			c.Next()
			return
		}
		h(c)
	}
}
