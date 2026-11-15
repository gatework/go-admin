package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gatework/motor-core/sdk/config"
)

func TestCancelledReloadRetainsQueueRetirementForShutdown(t *testing.T) {
	isolate(t)
	previous := newCountingQueue()
	previous.block = make(chan struct{})
	setInstalled(previous)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reloaded := make(chan error, 1)
	go func() { reloaded <- Setup(ctx, config.Config{}, testSubscribe) }()
	previous.waitStarted(t)
	cancel()
	select {
	case err := <-reloaded:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled reload remains blocked on retirement")
	}
	queueMu.Lock()
	pending := retiringQueue
	queueMu.Unlock()
	if pending == nil {
		t.Fatal("timed-out queue was forgotten")
	}
	select {
	case <-pending.done:
		t.Fatal("timed-out retirement reported completion")
	default:
	}
	retry, cancelRetry := context.WithTimeout(t.Context(), 10*time.Millisecond)
	if err := pending.wait(retry); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unfinished retirement: %v", err)
	}
	cancelRetry()
	close(previous.block)
	shutdownQueue(t.Context())
	select {
	case <-pending.done:
	default:
		t.Fatal("shutdown did not wait for pending retirement")
	}
}

func TestShutdownRetainsCurrentQueueUntilItCompletes(t *testing.T) {
	isolate(t)
	current := newCountingQueue()
	current.block = make(chan struct{})
	setInstalled(current)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	shutdownQueue(ctx)
	cancel()
	if currentInstalled() != current {
		t.Fatal("shutdown dropped a queue that has not finished")
	}
	close(current.block)
	shutdownQueue(t.Context())
	if currentInstalled() != nil {
		t.Fatal("completed shutdown retained queue")
	}
	if current.calls.Load() != 2 {
		t.Fatal("shutdown did not retry the pending current generation")
	}
}
