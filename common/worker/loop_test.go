package worker

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gatework/motor-core/logger"
)

func TestLoopOwnsWorkersAndRejectsDuplicateRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var loop Loop
		var calls atomic.Int32
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- loop.Run(ctx, 2, func(context.Context) error { calls.Add(1); return nil })
		}()
		synctest.Wait()
		if err := loop.Run(ctx, 2, func(context.Context) error { return nil }); !errors.Is(err, AlreadyStartedErr) {
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
			done <- loop.Run(ctx, 1, func(context.Context) error { <-release; return nil })
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

type recordingLogger struct {
	mu    sync.Mutex
	lines []string
}

func (r *recordingLogger) Log(_ context.Context, _ logger.Level, message string, _ ...logger.Field) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, message)
}
func (r *recordingLogger) With(...logger.Field) logger.Logger { return r }
func (r *recordingLogger) Enabled(logger.Level) bool          { return true }
func (r *recordingLogger) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.lines)
}

// A failing tick is logged with its error and the worker backs off instead of
// hammering a dependency that is down every second; a panic is a failure like
// any other rather than the end of the worker.
func TestLoopLogsFailuresSurvivesPanicsAndBacksOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		recorded := &recordingLogger{}
		loop := Loop{Name: "probe worker", Logger: logger.NewHelper(recorded), MaxBackoff: 8 * time.Second}
		var calls atomic.Int32
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() {
			done <- loop.Run(ctx, 1, func(context.Context) error {
				if calls.Add(1) == 1 {
					panic("bad record")
				}
				return errors.New("database unavailable")
			})
		}()
		time.Sleep(time.Minute)
		synctest.Wait()
		cancel()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		// Without backoff a minute holds 60 ticks; capped at 8s with jitter in
		// [4s, 8s] it holds well under 20.
		if got := calls.Load(); got < 5 || got > 20 {
			t.Fatalf("%d ticks in a minute; backoff not applied", got)
		}
		lines := recorded.snapshot()
		if len(lines) != int(calls.Load()) {
			t.Fatalf("%d failures logged for %d failed ticks", len(lines), calls.Load())
		}
		if !strings.Contains(lines[0], "probe worker failed") || !strings.Contains(lines[0], "panic: bad record") {
			t.Fatalf("panic not reported: %q", lines[0])
		}
		if !strings.Contains(lines[1], "database unavailable") {
			t.Fatalf("error not reported: %q", lines[1])
		}
	})
}

func TestBackoffNeverRetriesEarlierThanTheIntervalAndIsCapped(t *testing.T) {
	for failures := 1; failures <= 20; failures++ {
		for range 50 {
			pause := backoff(time.Second, 30*time.Second, failures)
			if pause < time.Second || pause > 30*time.Second {
				t.Fatalf("failures=%d: pause %v outside [1s, 30s]", failures, pause)
			}
		}
	}
}

func TestCleanupOutlivesItsParentButIsBounded(t *testing.T) {
	parent, cancel := context.WithCancel(context.WithValue(t.Context(), cleanupKey{}, "tenant-a"))
	cancel()
	ctx, stop := Cleanup(parent, time.Minute)
	defer stop()
	if ctx.Err() != nil {
		t.Fatal("cleanup context inherited the parent's cancellation")
	}
	if ctx.Value(cleanupKey{}) != "tenant-a" {
		t.Fatal("cleanup context lost the parent's values")
	}
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("cleanup context has no deadline")
	}
}

type cleanupKey struct{}
