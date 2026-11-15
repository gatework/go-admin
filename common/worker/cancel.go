package worker

import (
	"context"
	"time"
)

// CancelInterval is how often a running job looks for a persisted request to
// cancel it.
const CancelInterval = 500 * time.Millisecond

// cancelQueryTimeout bounds each look, so a stuck query cannot hold the watch.
const cancelQueryTimeout = time.Second

// WatchCancel runs a job's cancellation watch: every CancelInterval it asks
// cancelled whether the job's persisted state says to stop, and calls cancel
// when it does - or when the question cannot be answered, since a job that
// cannot confirm it should continue must not keep writing to a device.
// The watch ends when ctx ends or stop is called; stop waits for it.
func WatchCancel(ctx context.Context, cancel context.CancelFunc, cancelled func(context.Context) (bool, error)) (stop func()) {
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		ticker := time.NewTicker(CancelInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				query, release := context.WithTimeout(ctx, cancelQueryTimeout)
				stopNow, err := cancelled(query)
				release()
				if err != nil || stopNow {
					cancel()
					return
				}
			}
		}
	}()
	return func() {
		close(done)
		<-finished
	}
}
