package storage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gatework/motor-core/sdk/config"
	corestorage "github.com/gatework/motor-core/storage"
)

func memoryQueueConfigs() []struct {
	name   string
	config *config.Queue
} {
	return []struct {
		name   string
		config *config.Queue
	}{
		{"default", &config.Queue{}},
		{"configured", &config.Queue{Memory: &config.QueueMemory{PoolSize: 10}}},
	}
}

func TestMemoryQueueRunAfterShutdownReturns(t *testing.T) {
	for _, tc := range memoryQueueConfigs() {
		t.Run(tc.name, func(t *testing.T) {
			q, err := openQueue(t.Context(), tc.config)
			if err != nil {
				t.Fatal(err)
			}
			q.Shutdown(t.Context())
			done := make(chan struct{})
			go func() { defer close(done); q.Run(t.Context()) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("Run started after Shutdown remains blocked")
			}
		})
	}
}

func TestMemoryQueueConcurrentRunAndDrain(t *testing.T) {
	for _, tc := range memoryQueueConfigs() {
		t.Run(tc.name, func(t *testing.T) {
			for range 100 {
				q, err := openQueue(t.Context(), tc.config)
				if err != nil {
					t.Fatal(err)
				}
				var consumed atomic.Int32
				q.Subscribe(t.Context(), "t", func(context.Context, corestorage.Message) error { consumed.Add(1); return nil })
				for range 5 {
					if err := q.Publish(t.Context(), swapMsg()); err != nil {
						t.Fatal(err)
					}
				}
				start, done := make(chan struct{}), make(chan struct{})
				var workers sync.WaitGroup
				for range 4 {
					workers.Go(func() { ; <-start; q.Run(t.Context()) })
				}
				workers.Go(func() { ; <-start; q.Shutdown(t.Context()) })
				go func() { workers.Wait(); close(done) }()
				close(start)
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("Run/Shutdown overlap did not complete")
				}
				if consumed.Load() != 5 {
					t.Fatalf("drained %d messages, want 5", consumed.Load())
				}
				if err := q.Publish(t.Context(), swapMsg()); !errors.Is(err, corestorage.ErrQueueClosed) {
					t.Fatalf("Append after drain: %v", err)
				}
			}
		})
	}
}
