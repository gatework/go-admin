package storage

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/gatework/motor-core/sdk/config"
	corestorage "github.com/gatework/motor-core/storage"
)

// redisAddrEnv points these tests at a server. They are skipped without it, so
// a developer with no redis running still gets a green run - and CI sets it,
// which is the point: the ordering rule they cover is invisible on the memory
// backend, and memory is the default. A suite that only ever exercised the
// default would report success for a queue that silently drops every consumer.
const redisAddrEnv = "MOTOR_TEST_REDIS_ADDR"

func redisAddr(t *testing.T) string {
	t.Helper()
	addr := os.Getenv(redisAddrEnv)
	if addr != "" {
		return addr
	}
	// Skipping locally is the point; skipping in CI is the failure this whole
	// file exists to prevent. A workflow that renamed the variable, or dropped
	// the service, would otherwise go green while these two tests quietly did
	// nothing - which is the same shape as the defect they cover.
	if os.Getenv("CI") != "" {
		t.Fatalf("%s is not set while CI is: the redis-backed queue tests must not skip here", redisAddrEnv)
	}
	t.Skipf("%s is not set; skipping the redis-backed queue tests", redisAddrEnv)
	return ""
}

// newRedisQueue opens the same owned backend used by the host.
func newRedisQueue(t *testing.T, prefix string) corestorage.Queue {
	t.Helper()
	previous := testQueueConfig
	t.Cleanup(func() { testQueueConfig = previous })

	testQueueConfig = &config.Queue{
		Redis: &config.RedisQueue{
			Addr:      redisAddr(t),
			Group:     prefix,
			KeyPrefix: prefix,
		},
	}
	q, err := openQueue(t.Context(), testQueueConfig)
	if err != nil {
		t.Fatalf("queue setup: %v", err)
	}
	t.Cleanup(func() { _ = q.Shutdown(context.Background()) })
	return q
}

func message(t *testing.T, stream string) corestorage.Message {
	t.Helper()
	return corestorage.Message{Topic: stream, Values: map[string]any{"hello": "world"}}
}

// Registered first, then started: the consumer gets the message. This is the
// order setupQueue and attachQueueConsumers now produce between them.
func TestRedisQueueDeliversToAConsumerRegisteredBeforeTheStart(t *testing.T) {
	stream := "t-ordered"
	q := newRedisQueue(t, "gotest-ordered")

	got := make(chan struct{}, 1)
	q.Subscribe(t.Context(), stream, func(context.Context, corestorage.Message) error {
		select {
		case got <- struct{}{}:
		default:
		}
		return nil
	})
	go q.Run(t.Context())

	// Give Start a moment to reach its read loop before publishing.
	time.Sleep(500 * time.Millisecond)
	if err := q.Publish(t.Context(), message(t, stream)); err != nil {
		t.Fatalf("append: %v", err)
	}

	select {
	case <-got:
	case <-time.After(15 * time.Second):
		t.Fatal("the consumer never received the message")
	}
}

// A running queue rejects late subscribers and cannot publish unknown topics.
func TestRedisQueueRefusesAConsumerRegisteredAfterTheStart(t *testing.T) {
	stream := "t-late"
	q := newRedisQueue(t, "gotest-late")

	go q.Run(t.Context())
	time.Sleep(500 * time.Millisecond)

	if err := q.Subscribe(t.Context(), stream, func(context.Context, corestorage.Message) error { return nil }); !errors.Is(err, corestorage.ErrQueueAlreadyStarted) {
		t.Fatalf("late subscribe: %v", err)
	}

	err := q.Publish(t.Context(), message(t, stream))
	if err == nil {
		t.Fatal("a message was accepted for a topic whose registration came after Start; " +
			"if the backend now accepts late registration, the ordering rule in setupQueue can be revisited")
	}
	if !errors.Is(err, corestorage.ErrNoHandler) {
		t.Fatalf("append failed with %v, want %v - the test is meant to pin the "+
			"missing-consumer path, not any error at all", err, corestorage.ErrNoHandler)
	}
}
