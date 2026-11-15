// Package worker owns periodic background work until its parent is cancelled.
package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// AlreadyStartedErr reports a second Run on the same loop.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var AlreadyStartedErr = errors.New("worker: already started")

//revive:enable:error-naming

// Loop runs one lifecycle. Its zero value is ready to use.
type Loop struct {
	started atomic.Bool
}

// Run calls tick once per second on each worker and waits for all workers on
// cancellation. A tick must respect its context; its owner handles work errors.
func (l *Loop) Run(ctx context.Context, concurrency int, tick func(context.Context)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if concurrency < 1 || tick == nil {
		return errors.New("worker: positive concurrency and a tick function are required")
	}
	if !l.started.CompareAndSwap(false, true) {
		return AlreadyStartedErr
	}
	var workers sync.WaitGroup
	for range concurrency {
		workers.Go(func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if ctx.Err() != nil {
						return
					}
					tick(ctx)
				}
			}
		})
	}
	workers.Wait()
	return nil
}
