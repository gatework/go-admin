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

	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	corestorage "github.com/go-admin-team/go-admin-core/v2/storage"
)

func memoryJanitors() int {
	var stacks bytes.Buffer
	_ = pprof.Lookup("goroutine").WriteTo(&stacks, 2)
	return strings.Count(stacks.String(), "storage/cache.(*Memory).janitor(")
}

func TestCacheReloadRetiresMemoryJanitors(t *testing.T) {
	isolate(t)
	config.CacheConfig = &config.Cache{Memory: struct{}{}}
	config.QueueConfig = &config.Queue{}
	before := memoryJanitors()
	for range 5 {
		Setup()
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
	config.CacheConfig = &config.Cache{Memory: struct{}{}}
	config.QueueConfig = &config.Queue{}
	Setup()
	q := sdk.Runtime.GetQueuePrefix("")
	entered, release := make(chan struct{}), make(chan struct{})
	var enteredOnce sync.Once
	q.Register("t", func(corestorage.Messager) error { enteredOnce.Do(func() { close(entered) }); <-release; return nil })
	if err := q.Append(swapMsg()); err != nil {
		t.Fatal(err)
	}
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	shutdownQueue(ctx)
	cancel()
	err := q.Append(swapMsg())
	close(release)
	q.Shutdown()
	if !errors.Is(err, corestorage.ErrQueueClosed) {
		t.Fatalf("default queue still accepts after drain: %v", err)
	}
}

func TestShutdownDeadlineRemainsEffectiveDuringQueueReload(t *testing.T) {
	isolate(t)
	config.QueueConfig = &config.Queue{Memory: &config.QueueMemory{PoolSize: 10}}
	previous := newCountingQueue()
	previous.block = make(chan struct{})
	setInstalled(previous)
	reloaded := make(chan struct{})
	go func() { defer close(reloaded); setupQueue() }()
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
	config.CacheConfig = &config.Cache{}
	config.QueueConfig = &config.Queue{}
	Setup()
	generation := QueueGeneration()
	if err := sdk.Runtime.RunShutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	Setup()
	if currentInstalled() != nil || installedCache != nil || QueueGeneration() != generation {
		t.Fatal("configuration reload recreated resources after shutdown")
	}
}

func TestRemovingQueueConfigurationClosesPreviousBackend(t *testing.T) {
	isolate(t)
	config.QueueConfig = &config.Queue{Memory: &config.QueueMemory{PoolSize: 10}}
	setupQueue()
	previous := sdk.Runtime.GetQueuePrefix("")
	generation := QueueGeneration()
	config.QueueConfig = &config.Queue{}
	setupQueue()
	if QueueGeneration() == generation {
		t.Fatal("removing queue configuration retained previous backend")
	}
	if err := previous.Append(swapMsg()); !errors.Is(err, corestorage.ErrQueueClosed) {
		t.Fatalf("previous queue not closed: %v", err)
	}
}
