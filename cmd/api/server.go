package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"errors"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/gatework/motor-core/config/source/file"
	log "github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/pkg"
	"github.com/gatework/motor-core/sdk/runtime"
	corestorage "github.com/gatework/motor-core/storage"
	"github.com/gatework/motor/common/settings"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/router"
	adminservice "github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/jobs"
	otherrouter "github.com/gatework/motor/app/other/router"
	"github.com/gatework/motor/common/database"
	"github.com/gatework/motor/common/global"
	"github.com/gatework/motor/common/health"
	common "github.com/gatework/motor/common/middleware"
	"github.com/gatework/motor/common/middleware/handler"
	"github.com/gatework/motor/common/storage"
	ext "github.com/gatework/motor/config"
)

var (
	configYml string
	apiCheck  bool
	// StartCmd exposes this command to the host CLI.
	StartCmd *cobra.Command
)

// AppRouters holds the configured approuters value.
var AppRouters = make([]func(), 0)

//nolint:gochecknoinits // Registers host commands and routes before the runtime startup registry is sealed.
func init() {
	// StartCmd exposes this command to the host CLI.
	StartCmd = &cobra.Command{
		Use:          "server",
		Short:        "Start API server",
		Example:      "motor server -c config/settings.yml",
		SilenceUsage: true,
		PreRunE:      func(cmd *cobra.Command, args []string) error { return setup() },
		RunE: func(cmd *cobra.Command, args []string) error {
			return run()
		},
	}

	StartCmd.PersistentFlags().StringVarP(&configYml, "config", "c", "config/settings.yml", "Start server with provided configuration file")
	StartCmd.PersistentFlags().BoolVarP(&apiCheck, "api", "a", false, "Start server with check api data")

	//注册路由 fixme 其他应用的路由，在本目录新建文件放在init方法
	AppRouters = append(AppRouters, router.InitRouter)
}

func setup() error {
	resetApplicationLifetime()
	// 注入配置扩展项

	// Registered before the configuration is read. SetupConfig announces
	// AfterResource as soon as the callbacks that build the resources have
	// run, so a hook added after that call would miss the first round and the
	// queue would have no consumers until somebody edited the config file.

	// On AfterListen rather than on a bare goroutine from run(). Two reasons:
	// the phase runs behind core's panic guard, which does not reach across a
	// goroutine boundary - a panic while loading jobs used to take the whole
	// process down with a stack that named this file - and the jobs it starts
	// can call the API, which is only true once the socket is accepting.
	sdk.Runtime.SetPhase(runtime.AfterListen, startCronJobs)

	// Registered before the configuration is read, because it registers a
	// callback rather than reading anything: the check runs per request and
	// asks the databases that exist then.
	registerSchemaCheck()

	//1. 读取配置
	var err error
	configurationWatcher, err = settings.Load(lifecycleContext(), sdk.Runtime, file.NewSource(file.WithPath(configYml)), database.Setup, func(candidate *config.Settings) error {
		return storage.Setup(lifecycleContext(), candidate.Settings, subscribeQueue)
	})
	if err != nil {
		stopApplication()
		return err
	}

	usageStr := `starting api server...`
	log.NewHelper(log.Default()).Info(usageStr)
	return nil
}

// startCronJobs registers the job implementations and starts a scheduler for
// every tenant database.
//
// It is synchronous, like the phase that runs it. jobs.Setup returns now that
// the `select {}` at the end of its per-tenant setup is gone, which is what
// makes that possible; while it was there this could only be a goroutine, and
// a goroutine is outside the panic guard.
func startCronJobs() {
	jobs.InitJob()
	jobs.Setup(sdk.Runtime.Databases())
}

var (
	applicationContext   = context.Background()
	stopApplication      = context.CancelFunc(func() {})
	configurationWatcher *settings.Watcher
)

func lifecycleContext() context.Context { return applicationContext }

// subscribeQueue prepares every topic before storage publishes the queue.
func subscribeQueue(ctx context.Context, q corestorage.Queue) error {
	for _, topic := range []struct {
		name    string
		handler corestorage.Handler
	}{
		{global.LoginLog, models.SaveLoginLog}, {global.OperateLog, models.SaveOperaLog}, {global.APICheck, adminservice.SyncAPIs},
	} {
		if err := q.Subscribe(ctx, topic.name, topic.handler); err != nil {
			return err
		}
	}
	return nil
}

func run() error {
	defer stopApplication()
	defer func() {
		if err := settings.CloseLogger(); err != nil {
			fmt.Fprintf(os.Stderr, "close logger: %v\n", err)
		}
	}()
	if configurationWatcher != nil {
		defer configurationWatcher.Close()
	}
	// Resolved first, and used both for the line it prints and for the
	// shutdown that spends it. Reading the configuration again at signal time
	// would let the two disagree, and the sum that gets printed is the whole
	// point of printing it.
	//
	// Refused rather than corrected, and refused before anything is built: a
	// budget that cannot be spent as written is a configuration error, and the
	// moment to say so is while nothing depends on this process yet.
	seconds, err := ext.Ext().Shutdown.Budget()
	if err != nil {
		return err
	}
	reportShutdownBudget(seconds)

	if config.Current().Settings.Application.Mode == pkg.ModeProd.String() {
		gin.SetMode(gin.ReleaseMode)
	}
	buildRouter()
	reportGeneratorWriteRoutes()

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Current().Settings.Application.Host, config.Current().Settings.Application.Port),
		Handler:      sdk.Runtime.Engine(),
		ReadTimeout:  time.Duration(config.Current().Settings.Application.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(config.Current().Settings.Application.WriterTimeout) * time.Second,
	}

	if apiCheck {
		routes, err := json.Marshal(sdk.Runtime.Routers())
		if err != nil {
			return fmt.Errorf("encode api registry: %w", err)
		}
		if err = sdk.Runtime.QueueForTenant("").Publish(lifecycleContext(), corestorage.Message{Topic: global.APICheck, Values: map[string]any{"routes": string(routes)}}); err != nil {
			return fmt.Errorf("publish api registry: %w", err)
		}
	}

	// Armed before the server starts serving, and well before the readiness
	// banner: a signal arriving between "the process is up" and "the process
	// is listening for signals" reaches the default handler and kills it
	// without any of the shutdown below. That window is the whole reason
	// arming is separate from waiting.
	quit, disarmStopSignals := armStopSignals()

	if err := startServing(srv, config.Current().Settings.SSL.Enable, config.Current().Settings.SSL.Pem, config.Current().Settings.SSL.KeyStr); err != nil {
		return err
	}

	fmt.Println(pkg.Red(string(global.LogoContent)))
	tip()
	fmt.Println(pkg.Green("Server run at:"))
	fmt.Printf("-  Local:   %s://localhost:%d/ \r\n", "http", config.Current().Settings.Application.Port)
	fmt.Printf("-  Network: %s://%s:%d/ \r\n", "http", pkg.GetLocalHost(), config.Current().Settings.Application.Port)
	fmt.Println(pkg.Green("Swagger run at:"))
	fmt.Printf("-  Local:   http://localhost:%d/swagger/admin/index.html \r\n", config.Current().Settings.Application.Port)
	fmt.Printf("-  Network: %s://%s:%d/swagger/admin/index.html \r\n", "http", pkg.GetLocalHost(), config.Current().Settings.Application.Port)
	fmt.Printf("%s Enter Control + C Shutdown Server \r\n", pkg.GetCurrentTimeStr())

	<-quit
	serverErr, cleanupErr := gracefulShutdown(srv, quit, disarmStopSignals, budgetFrom(seconds))
	if serverErr != nil {
		// Not log.Fatal: that is an unconditional os.Exit(1), and Shutdown
		// reports an error exactly when connections were still in flight -
		// which is when the cleanup that ran after it mattered most.
		log.NewHelper(log.Default()).Error("Server Shutdown: ", serverErr)
	}
	if cleanupErr != nil {
		log.NewHelper(log.Default()).Error("Cleanup: ", cleanupErr)
	}

	return nil
}

// budget is the three waits a shutdown spends, in the order it spends them.
type budget struct {
	drain   time.Duration
	server  time.Duration
	cleanup time.Duration
}

// budgetFrom turns the resolved seconds into the durations the sequence waits
// on.
func budgetFrom(s ext.ShutdownBudget) budget {
	return budget{
		drain:   time.Duration(s.Drain) * time.Second,
		server:  time.Duration(s.Server) * time.Second,
		cleanup: time.Duration(s.Cleanup) * time.Second,
	}
}

// defaultBudget is what a process with no extend.shutdown section spends.
func defaultBudget() budget {
	return budget{drain: drainTimeout, server: shutdownTimeout, cleanup: cleanupTimeout}
}

// gracefulShutdown takes the process down in the order that gives something
// else a chance to notice first.
//
// The whole order lives here, and run() is not the only caller: the signal
// tests run this function rather than reproducing it. A test that reproduces a
// sequence asserts against its own copy and stays green while the sequence it
// was written for regresses.
//
// The caller has already taken the first signal off quit. quit is handed on
// because a second signal during the drain window ends the window early -
// somebody sending another kill wants this over with sooner - and because
// until the window is over that signal must not reach the default handler and
// kill the process outright.
//
// disarm is therefore called at the end of the window rather than on the first
// signal. After it, a second signal is handled by the default disposition
// again, which is the only way out of a Shutdown or a cleanup callback that
// never returns. Restoring it any earlier would put every ordinary shutdown
// inside that escape hatch for the whole length of the drain, where before
// this window existed only a hung callback could reach it.
//
// The two waits' errors are returned separately rather than logged: they fail
// for different reasons, and the caller decides what each is worth.
func gracefulShutdown(srv *http.Server, quit <-chan os.Signal, disarm func(), b budget) (serverErr, cleanupErr error) {
	// Said before anything is taken apart. A configuration reload arriving in
	// this window would otherwise re-run AfterResource - rebuilding the pool
	// and the queue adapter, and re-registering consumers - on top of cleanup
	// that has already run.
	sdk.Runtime.BeginShutdown()

	// Readiness fails from here, which is before the server stops accepting.
	// That order is necessary and not sufficient: with nothing between this
	// line and the listener closing, the two are microseconds apart and a
	// poller on a multi-second interval sees the refused connection instead of
	// the 503. The window below is what turns the order into something
	// observable - extend.shutdown.drain, which is zero unless it is
	// configured.
	health.BeginDraining()

	// Keep-alive off for the same window, and for the same reason. The server
	// keeps connections alive while !disableKeepAlives && !shuttingDown(), and
	// shuttingDown() is only set by Shutdown itself - so without this line
	// every pooled connection stays open for the whole drain and is cut at the
	// end of it anyway, which is the cost of the window without its benefit.
	// This is the switch Shutdown flips, moved earlier by the window's length:
	// answers now carry Connection: close, and the idle connections a balancer
	// is holding are closed at once rather than when it next tries to use one.
	srv.SetKeepAlivesEnabled(false)

	drain(quit, b.drain)

	// Restored here, not on the first signal: from this point a second signal
	// must reach the default handler, so a shutdown that hangs can still be
	// interrupted.
	disarm()

	log.NewHelper(log.Default()).Info("Shutdown Server ... ")
	serverErr = shutdownServer(srv, b.server)
	// Runs whether or not the wait above failed, and deliberately so: Shutdown
	// reports an error exactly when connections were still in flight, which is
	// when there is most left to clean up after.
	if configurationWatcher != nil {
		_ = configurationWatcher.Close()
	}
	stopApplication()
	cleanupErr = runShutdownHooks(b.cleanup)
	log.NewHelper(log.Default()).Info("Server exiting")

	return serverErr, cleanupErr
}

// drain keeps serving for d, or until another stop signal arrives.
//
// Requests are answered normally throughout. Refusing them would move the
// outage earlier rather than avoid it - the point of the window is that this
// instance is still able to work while whoever routes to it stops routing.
func drain(quit <-chan os.Signal, d time.Duration) {
	if d <= 0 {
		return
	}
	log.NewHelper(log.Default()).Infof("Draining for %s: still serving, /ready answers 503 from here", d)

	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-quit:
		log.NewHelper(log.Default()).Info("Second signal during the drain window, closing the listener now")
	case <-timer.C:
	}
}

// Reference stop grace periods, printed when nothing was configured to compare
// against. They are three times apart, which is why the check below needs a
// configured value rather than a constant of its own: a budget that overruns
// under one of them fits comfortably under the other.
const (
	dockerStopGraceSeconds = 10
	kubernetesGraceSeconds = 30
)

// reportGeneratorWriteRoutes says whether this process serves the code
// generator's writing endpoints, and to whom.
//
// The endpoints are gated on the mode, and the shipped configuration says dev -
// so the deployment most likely to be exposed is the one that changed nothing,
// and the one least likely to go looking. Silence there would leave the gate
// technically correct and practically useless.
//
// Nothing is said in demo mode. The routes are registered, but DemoEvn refuses
// all three by name, so a warning would describe an exposure that is not there.
func reportGeneratorWriteRoutes() {
	if !generatorWriteRoutesNeedWarning() {
		return
	}
	log.NewHelper(log.Default()).Warnf("the code generator's writing endpoints are served in mode %q: "+
		"/api/v1/gen/{toproject,apitofile,todb} write Go and Vue source onto this host and rows "+
		"into this database, and they are in CasbinExclude, so any account that can log in may "+
		"call them. Set application.mode to prod or test on anything that is not a workstation, "+
		"then restart: these routes were registered at start-up and a configuration reload does "+
		"not rebuild them.",
		config.Current().Settings.Application.Mode)
}

// generatorWriteRoutesNeedWarning reports whether there is an exposure to warn
// about: the endpoints are served, and nothing else is refusing them.
//
// Split from the logging so the decision can be tested. A warning nobody can
// make fire is indistinguishable from no warning at all, and this one exists
// precisely for the case nobody is looking at.
func generatorWriteRoutesNeedWarning() bool {
	return otherrouter.GenWriteRoutesEnabled() && config.Current().Settings.Application.Mode != "demo"
}

// reportShutdownBudget states what a shutdown will spend and whether it fits.
//
// The sum is taken from the resolved values, not from the configuration file:
// a field left out of extend.shutdown still costs its default, so adding up
// what was written down understates the budget by exactly the fields nobody
// wrote.
func reportShutdownBudget(s ext.ShutdownBudget) {
	log.NewHelper(log.Default()).Infof("shutdown budget: drain %ds + server %ds + cleanup %ds = %ds",
		s.Drain, s.Server, s.Cleanup, s.Total())

	if s.Grace <= 0 {
		log.NewHelper(log.Default()).Infof("shutdown budget: extend.shutdown.grace is not set, so nothing is compared against it - "+
			"for reference `docker stop` allows %ds and Kubernetes terminationGracePeriodSeconds defaults to %ds",
			dockerStopGraceSeconds, kubernetesGraceSeconds)
		return
	}
	if over := s.Overrun(); over > 0 {
		// A minimum, not a target. This is somebody else's deployment under
		// constraints this process cannot see, so the honest thing to state is
		// how much is missing - the repository's own files are where there is
		// standing to ask for headroom on top, and checksilent does that.
		log.NewHelper(log.Default()).Warnf("shutdown budget of %ds does not fit inside the %ds of extend.shutdown.grace: "+
			"SIGKILL arrives while the cleanup callbacks are still running, and the work they "+
			"were about to finish is lost. It needs at least %ds more, or %ds less budget.",
			s.Total(), s.Grace, over, over)
		return
	}
	log.NewHelper(log.Default()).Infof("shutdown budget of %ds fits inside the %ds of extend.shutdown.grace", s.Total(), s.Grace)
}

// The budgets a shutdown spends when extend.shutdown configures nothing:
// drainTimeout keeps the process serving after the stop signal, then
// shutdownTimeout waits for in-flight requests, then cleanupTimeout is what
// the BeforeExit callbacks get.
//
// The seconds come from config, which is where an absent field falls back, so
// the default is one number rather than two that can drift apart.
//
// They are consumed one after the other, so their sum is what has to stay
// inside the orchestrator's grace period: `docker stop` allows 10s by default
// before it sends SIGKILL, and 0+5+3 leaves room for the process to finish
// returning. Raising one without lowering another buys nothing - the budget
// that runs out is the orchestrator's, and reportShutdownBudget is what says
// so at start-up.
var (
	drainTimeout    = time.Duration(ext.DefaultDrainSeconds) * time.Second
	shutdownTimeout = time.Duration(ext.DefaultServerSeconds) * time.Second
	cleanupTimeout  = time.Duration(ext.DefaultCleanupSeconds) * time.Second
)

// armStopSignals registers for the stop signals and returns the channel they
// arrive on together with the function that restores the default disposition.
//
// SIGTERM is what actually arrives in production: `docker stop`, a Kubernetes
// pod deletion and `systemctl stop` all send it, and Go terminates the process
// immediately for a signal nobody listens for. Registering only os.Interrupt
// meant every graceful shutdown below the wait was dead code outside a
// terminal.
//
// Registering is separate from waiting so a caller can arm before it announces
// that it is ready: a signal that arrives between the two is delivered to the
// default handler, which for both of these means the process dies without
// running any of this.
func armStopSignals() (<-chan os.Signal, func()) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	return quit, func() { signal.Stop(quit) }
}

// startServing binds srv.Addr, hands the listener to srv on its own goroutine,
// and announces AfterListen.
//
// The bind is done here rather than left to ListenAndServe, which binds on the
// goroutine that serves. That put the failure every deployment actually hits -
// "address already in use" - on a goroutine nobody was reading, so the banner
// went on to claim the server was up, and there would be no way to keep
// AfterListen from announcing a socket that does not exist. A hook there is
// promised a reachable port; the only way to keep that promise is for the bind
// to have already happened on this goroutine.
//
// AfterListen is announced synchronously. Running it in a goroutine to save the
// few milliseconds would let it overlap the shutdown: on a fast SIGTERM the
// cleanup callbacks could finish before the startup ones had.
//
// Both ways of failing to start are therefore checked before the announcement:
// the bind, and - with ssl enabled - the certificate.
func startServing(srv *http.Server, useTLS bool, pem, key string) error {
	if useTLS {
		// Read the certificate before anything is announced. ServeTLS reads
		// these files itself, but on the serving goroutine - so a bad
		// certificate used to surface after AfterListen had already promised a
		// reachable port. Loading it here costs one extra read and moves the
		// failure onto this goroutine, where run() can return it.
		//
		// ServeTLS still does the real work below rather than this handing it a
		// tls.Listener: that is what sets up HTTP/2 negotiation, and taking it
		// over here would quietly drop h2 for every TLS deployment.
		if _, err := tls.LoadX509KeyPair(pem, key); err != nil {
			return fmt.Errorf("tls certificate: %w", err)
		}
	}

	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", srv.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	go func() {
		// 服务连接
		var err error
		if useTLS {
			err = srv.ServeTLS(ln, pem, key)
		} else {
			err = srv.Serve(ln)
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			// Still fatal, as it was. Neither the bind nor the certificate is
			// among the errors that reach here any more - both are checked
			// above, on the caller's goroutine. What is left is a serve that
			// failed after the port was taken, and carrying on would park the
			// process on <-quit with nothing serving.
			log.NewHelper(log.Default()).Fatal("serve: ", err)
		}
	}()

	sdk.Runtime.RunPhase(runtime.AfterListen)
	return nil
}

// shutdownServer stops srv, giving in-flight requests up to timeout to finish.
//
// It returns the error instead of exiting on it. A caller that exits here skips
// its own cleanup, and Shutdown fails precisely when there was something left
// to clean up after.
func shutdownServer(srv *http.Server, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return srv.Shutdown(ctx)
}

// runShutdownHooks runs the BeforeExit callbacks with timeout to share.
//
// What the budget bounds is the wait, not the work. When it is gone RunShutdown
// stops waiting and returns; a callback that never looks at its context carries
// on until the process exits, and may leave a partial write behind. Go cannot
// cancel a function that does not check for cancellation, which is why the
// callbacks are handed a context at all.
func runShutdownHooks(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return sdk.Runtime.RunShutdown(ctx)
}

// buildRouter announces BeforeRouter, builds the engine, and then drains the
// startup registries.
//
// The order is the contract. BeforeRouter is the last point at which a module
// can still affect how routes are built, so it has to run while there is no
// engine yet. The before registry runStartupHooks drains is a different moment
// despite the name: those callbacks run after initRouter has built the engine.
// Two lines apart, and describing them as equivalent is a mistake this
// repository has already made once in writing.
func buildRouter() {
	sdk.Runtime.RunPhase(runtime.BeforeRouter)
	initRouter()
	runStartupHooks()
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

//var Router runtime.Router

func tip() {
	usageStr := `欢迎使用 ` + pkg.Green(`motor `+global.Version) + ` 可以使用 ` + pkg.Red(`-h`) + ` 查看命令`
	fmt.Printf("%s \n\n", usageStr)
}

func initRouter() {
	var r *gin.Engine
	h := sdk.Runtime.Engine()
	if h == nil {
		h = gin.New()
		sdk.Runtime.SetEngine(h)
	}
	switch h := h.(type) {
	case *gin.Engine:
		r = h
	default:
		log.NewHelper(log.Default()).Fatal("not support other engine")
		//os.Exit(-1)
	}
	// First, so that a panic anywhere below still produces a logged 500.
	r.Use(common.Recover)
	if config.Current().Settings.SSL.Enable {
		r.Use(handler.TLSHandler())
	}
	//r.Use(middleware.Metrics())
	r.Use(exemptProbes(common.Sentinel())).
		Use(common.RequestID(pkg.TrafficKey)).
		Use(api.SetRequestLogger)

	common.InitMiddlewareWithDatabase(r, exemptProbes(common.WithContextDB))

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

func applicationLifetime(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithCancel(parent)
}

//nolint:fatcontext // The host explicitly replaces the process lifecycle at command startup.
func resetApplicationLifetime() {
	applicationContext, stopApplication = applicationLifetime(StartCmd.Context())
}
