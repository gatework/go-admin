package health

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gatework/motor-core/sdk"
)

type blockedCache struct {
	fakeCache
	calls   atomic.Int64
	entered chan struct{}
	release chan struct{}
}

func (c *blockedCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if c.calls.Add(1) == 1 {
		close(c.entered)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.release:
	}
	return c.fakeCache.Set(ctx, key, value, ttl)
}

func TestReadyHonorsCacheDeadlineWithoutAccumulatingWorkers(t *testing.T) {
	freshRuntime(t)
	cache := &blockedCache{entered: make(chan struct{}), release: make(chan struct{})}
	sdk.Runtime.SetCache(cache)
	done := make([]chan struct{}, 0, 5)
	for range 5 {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		returned := make(chan struct{})
		done = append(done, returned)
		go func() { defer close(returned); Ready(ctx) }()
		select {
		case <-returned:
		case <-time.After(100 * time.Millisecond):
			t.Error("readiness ignored cache deadline")
		}
		cancel()
	}
	if got := cache.calls.Load(); got != 5 {
		t.Errorf("expected one canceled operation per probe, got %d", got)
	}
	close(cache.release)
	for _, returned := range done {
		<-returned
	}
}

func TestReadyCanceledContextDoesNotStartCacheIO(t *testing.T) {
	freshRuntime(t)
	cache := &blockedCache{entered: make(chan struct{}), release: make(chan struct{})}
	close(cache.release)
	sdk.Runtime.SetCache(cache)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got := named(Ready(ctx), "cache")
	if got.OK || cache.calls.Load() != 0 {
		t.Fatalf("canceled check performed cache I/O: result=%+v calls=%d", got, cache.calls.Load())
	}
}
