package api

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gatework/motor-core/storage"
	"github.com/gatework/motor/common/global"
)

type recordingQueue struct {
	topics   []string
	handlers map[string]storage.Handler
	failAt   string
	runCalls int
}

func (q *recordingQueue) Publish(context.Context, storage.Message) error { return nil }
func (q *recordingQueue) Subscribe(ctx context.Context, name string, h storage.Handler) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if name == q.failAt {
		return errors.New("subscription failed")
	}
	if q.handlers == nil {
		q.handlers = map[string]storage.Handler{}
	}
	if _, exists := q.handlers[name]; exists {
		return storage.ErrTopicAlreadySubscribed
	}
	q.topics = append(q.topics, name)
	q.handlers[name] = h
	return nil
}
func (q *recordingQueue) Run(context.Context) error      { q.runCalls++; return nil }
func (q *recordingQueue) Shutdown(context.Context) error { return nil }

func TestQueueConsumersArePreparedBeforeStorageStarts(t *testing.T) {
	for range 2 {
		q := &recordingQueue{}
		if err := subscribeQueue(t.Context(), q); err != nil {
			t.Fatal(err)
		}
		expected := []string{global.LoginLog, global.OperateLog, global.APICheck}
		if !reflect.DeepEqual(q.topics, expected) {
			t.Fatalf("topics %v; want %v", q.topics, expected)
		}
		for name, handler := range q.handlers {
			if handler == nil {
				t.Fatalf("topic %s has no consumer", name)
			}
		}
		if q.runCalls != 0 {
			t.Fatal("subscription started the queue before storage published it")
		}
		if err := subscribeQueue(t.Context(), q); !errors.Is(err, storage.ErrTopicAlreadySubscribed) {
			t.Fatalf("duplicate subscription error = %v", err)
		}
	}
}
func TestQueueSubscriptionFailureStopsPreparation(t *testing.T) {
	q := &recordingQueue{failAt: global.OperateLog}
	if err := subscribeQueue(t.Context(), q); err == nil {
		t.Fatal("subscription error was discarded")
	}
	if _, exists := q.handlers[global.APICheck]; exists {
		t.Fatal("subscribed after failure")
	}
	if q.runCalls != 0 {
		t.Fatal("failed preparation started queue")
	}
}
func TestQueuePreparationHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	q := &recordingQueue{}
	if err := subscribeQueue(ctx, q); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
	if len(q.topics) != 0 || q.runCalls != 0 {
		t.Fatal("cancelled preparation changed queue")
	}
}
