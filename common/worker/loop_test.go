package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestLoopOwnsWorkersAndRejectsDuplicateRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop Loop
		var calls atomic.Int32
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- loop.Run(ctx, 2, func(context.Context) { calls.Add(1) })
		}()
		synctest.Wait()
		if err := loop.Run(ctx, 2, func(context.Context) {}); !errors.Is(err, AlreadyStartedErr) {
			t.Fatalf("duplicate run returned %v", err)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if got := calls.Load(); got != 2 {
			t.Fatalf("got %d worker calls, want 2", got)
		}
		cancel()
		synctest.Wait()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Second)
		if got := calls.Load(); got != 2 {
			t.Fatalf("stopped workers made another call: %d", got)
		}
	})
}

func TestLoopWaitsForActiveWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop Loop
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		release := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- loop.Run(ctx, 1, func(context.Context) { <-release })
		}()
		time.Sleep(time.Second)
		synctest.Wait()
		cancel()
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatalf("returned before active work stopped: %v", err)
		default:
		}
		close(release)
		synctest.Wait()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})
}
