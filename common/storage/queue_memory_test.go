package storage

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	corestorage "github.com/go-admin-team/go-admin-core/v2/storage"
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
			q, err := openQueue(tc.config)
			if err != nil {
				t.Fatal(err)
			}
			q.Shutdown()
			done := make(chan struct{})
			go func() { defer close(done); q.Run() }()
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
				q, err := openQueue(tc.config)
				if err != nil {
					t.Fatal(err)
				}
				var consumed atomic.Int32
				q.Register("t", func(corestorage.Messager) error { consumed.Add(1); return nil })
				for range 5 {
					if err := q.Append(swapMsg()); err != nil {
						t.Fatal(err)
					}
				}
				start, done := make(chan struct{}), make(chan struct{})
				var workers sync.WaitGroup
				for range 4 {
					workers.Add(1)
					go func() { defer workers.Done(); <-start; q.Run() }()
				}
				workers.Add(1)
				go func() { defer workers.Done(); <-start; q.Shutdown() }()
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
				if err := q.Append(swapMsg()); !errors.Is(err, corestorage.ErrQueueClosed) {
					t.Fatalf("Append after drain: %v", err)
				}
			}
		})
	}
}
