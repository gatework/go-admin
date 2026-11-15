package storage

import (
	"bytes"
	"context"
	"errors"
	"runtime/pprof"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/config"
	corestorage "github.com/gatework/motor-core/storage"
)

func memoryJanitors() int {
	var stacks bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&stacks, 2)
	return strings.Count(stacks.String(), "storage/cache.(*Memory).janitor(")
}

func TestCacheReloadRetiresMemoryJanitors(t *testing.T) {
	isolate(t)
	testCacheConfig = &config.Cache{Memory: &config.CacheMemory{}}
	testQueueConfig = &config.Queue{}
	before := memoryJanitors()
	for range 5 {
		setupStorage(t)
	}
	if err := sdk.Runtime.RunShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for memoryJanitors() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if after := memoryJanitors(); after > before {
		t.Fatalf("reload/shutdown leaked %d memory janitors", after-before)
	}
}

func TestDefaultQueueIsOwnedAndDrained(t *testing.T) {
	isolate(t)
	testCacheConfig = &config.Cache{Memory: &config.CacheMemory{}}
	testQueueConfig = &config.Queue{}
	entered, release := make(chan struct{}), make(chan struct{})
	var enteredOnce sync.Once
	testSubscribe = func(ctx context.Context, q corestorage.Queue) error {
		return q.Subscribe(ctx, "t", func(context.Context, corestorage.Message) error {
			enteredOnce.Do(func() { close(entered) })
			<-release
			return nil
		})
	}
	setupStorage(t)
	q := sdk.Runtime.QueueForTenant("")
	if err := q.Publish(t.Context(), swapMsg()); err != nil {
		t.Fatal(err)
	}
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	shutdownQueue(ctx)
	cancel()
	err := q.Publish(t.Context(), swapMsg())
	close(release)
	q.Shutdown(t.Context())
	if !errors.Is(err, corestorage.ErrQueueClosed) {
		t.Fatalf("default queue still accepts after drain: %v", err)
	}
}

func TestShutdownDeadlineRemainsEffectiveDuringQueueReload(t *testing.T) {
	isolate(t)
	testQueueConfig = &config.Queue{Memory: &config.QueueMemory{PoolSize: 10}}
	previous := newCountingQueue()
	previous.block = make(chan struct{})
	setInstalled(previous)
	reloaded := make(chan struct{})
	go func() { defer close(reloaded); setupStorage(t) }()
	previous.waitStarted(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	returned := make(chan struct{})
	go func() { defer close(returned); shutdownQueue(ctx) }()
	select {
	case <-returned:
	case <-time.After(200 * time.Millisecond):
		t.Error("retirement held ownership mutex and blocked shutdown deadline")
	}
	cancel()
	close(previous.block)
	<-reloaded
	<-returned
	if previous.calls.Load() != 1 {
		t.Fatal("shutdown duplicated the pending retirement")
	}
}

func TestSetupCannotRecreateResourcesAfterShutdown(t *testing.T) {
	isolate(t)
	testCacheConfig = &config.Cache{}
	testQueueConfig = &config.Queue{}
	setupStorage(t)
	generation := QueueGeneration()
	if err := sdk.Runtime.RunShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	setupStorage(t)
	if currentInstalled() != nil || installedCache != nil || QueueGeneration() != generation {
		t.Fatal("configuration reload recreated resources after shutdown")
	}
}

func TestRemovingQueueConfigurationClosesPreviousBackend(t *testing.T) {
	isolate(t)
	testQueueConfig = &config.Queue{Memory: &config.QueueMemory{PoolSize: 10}}
	setupStorage(t)
	previous := sdk.Runtime.QueueForTenant("")
	generation := QueueGeneration()
	testQueueConfig = &config.Queue{}
	setupStorage(t)
	if QueueGeneration() == generation {
		t.Fatal("removing queue configuration retained previous backend")
	}
	if err := previous.Publish(t.Context(), swapMsg()); !errors.Is(err, corestorage.ErrQueueClosed) {
		t.Fatalf("previous queue not closed: %v", err)
	}
}
