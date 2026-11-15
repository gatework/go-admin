// Package worker owns periodic background work until its parent is cancelled.
//
// A background tick has no caller to return an error to, so the loop is where
// its failures are reported: it logs them, survives a panic in one tick, and
// backs off while a dependency keeps failing instead of retrying every second.
package worker

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gatework/motor-core/logger"
)

// AlreadyStartedErr reports a second Run on the same loop.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var AlreadyStartedErr = errors.New("worker: already started")

//revive:enable:error-naming

// Defaults applied to a zero Loop.
const (
	DefaultInterval   = time.Second
	DefaultMaxBackoff = 30 * time.Second
)

// Loop runs one lifecycle. Its zero value is ready to use; set the fields
// before Run and do not change them afterwards.
type Loop struct {
	// Name prefixes every log line, e.g. "fabric worker".
	Name string
	// Logger receives tick failures. Nil means the default logger.
	Logger *logger.Helper
	// Interval is the pause between successful ticks. Zero means DefaultInterval.
	Interval time.Duration
	// MaxBackoff caps the pause after consecutive failures. Zero means
	// DefaultMaxBackoff.
	MaxBackoff time.Duration

	started atomic.Bool
}

// Run calls tick on each of concurrency goroutines, pausing Interval between
// ticks, until ctx is cancelled; then it waits for every goroutine. A tick must
// respect its context. A tick that fails or panics is logged, and that
// goroutine waits longer before the next one: the pause doubles with each
// consecutive failure up to MaxBackoff, with jitter so that several goroutines
// do not retry a recovering dependency in step. Cancellation is not a failure.
func (l *Loop) Run(ctx context.Context, concurrency int, tick func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if concurrency < 1 || tick == nil {
		return errors.New("worker: positive concurrency and a tick function are required")
	}
	if !l.started.CompareAndSwap(false, true) {
		return AlreadyStartedErr
	}
	log := l.Logger
	if log == nil {
		log = logger.NewHelper(logger.Default())
	}
	interval, maxBackoff := l.Interval, l.MaxBackoff
	if interval <= 0 {
		interval = DefaultInterval
	}
	if maxBackoff < interval {
		maxBackoff = max(DefaultMaxBackoff, interval)
	}
	var workers sync.WaitGroup
	for range concurrency {
		workers.Go(func() {
			failures := 0
			timer := time.NewTimer(interval)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-timer.C:
				}
				err := guarded(ctx, tick)
				if ctx.Err() != nil {
					return
				}
				if err == nil {
					failures = 0
					timer.Reset(interval)
					continue
				}
				failures++
				log.Errorf("%s failed (%d in a row): %v", l.name(), failures, err)
				timer.Reset(backoff(interval, maxBackoff, failures))
			}
		})
	}
	workers.Wait()
	return nil
}

func (l *Loop) name() string {
	if l.Name == "" {
		return "worker"
	}
	return l.Name
}

// guarded turns a panic in one tick into an error, so one bad record cannot
// stop the worker for the rest of the process's life.
func guarded(ctx context.Context, tick func(context.Context) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("panic: %v\n%s", recovered, debug.Stack())
		}
	}()
	return tick(ctx)
}

// backoff is the pause after the given number of consecutive failures:
// interval doubled per failure, capped, and drawn from the upper half so that
// goroutines failing together spread out without ever retrying early.
func backoff(interval, maxBackoff time.Duration, failures int) time.Duration {
	pause := interval
	for range failures {
		if pause >= maxBackoff/2 {
			pause = maxBackoff
			break
		}
		pause *= 2
	}
	half := pause / 2
	return half + rand.N(half+1) //nolint:gosec // G404: jitter, not a secret.
}
