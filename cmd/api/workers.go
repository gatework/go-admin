package api

import (
	"context"

	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"
)

// startWorker keeps startup at the application root and joins each worker
// within the same shutdown budget used for the rest of the service.
func startWorker(name string, run func(context.Context) error) {
	ctx, cancel := context.WithCancel(lifecycleContext())
	done := make(chan struct{})
	sdk.Runtime.SetShutdown(func(shutdown context.Context) {
		cancel()
		select {
		case <-done:
		case <-shutdown.Done():
			logger.NewHelper(logger.Default()).Errorf("%s worker shutdown: %v", name, shutdown.Err())
		}
	})
	go func() {
		defer close(done)
		if err := run(ctx); err != nil && ctx.Err() == nil {
			logger.NewHelper(logger.Default()).Errorf("%s worker stopped: %v", name, err)
		}
	}()
}
