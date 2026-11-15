package api

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"

	"github.com/gatework/motor/common/health"
	ext "github.com/gatework/motor/config"
)

// Graceful shutdown: drain, server shutdown and cleanup, each with its own
// budget, plus the signal handling that starts it.

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
