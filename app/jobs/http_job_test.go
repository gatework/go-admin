package jobs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gatework/motor-core/sdk/pkg"
)

func TestHTTPJobRetriesOnlyFailures(t *testing.T) {
	for _, successAt := range []int{1, 2, 3, 4} {
		t.Run(time.Duration(successAt).String(), func(t *testing.T) {
			calls := 0
			sentinel := errors.New("upstream failed")
			err := runHTTPJob(t.Context(), "fixture", func(context.Context, string) (string, error) {
				calls++
				if calls == successAt {
					return "done", nil
				}
				return "", sentinel
			}, func(int) time.Duration { return 0 })
			expected := min(successAt, 3)
			if calls != expected {
				t.Fatalf("calls=%d want=%d", calls, expected)
			}
			if successAt > 3 {
				if !errors.Is(err, sentinel) {
					t.Fatalf("error=%v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHTTPJobCancellationDuringBackoffStopsRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls := 0
	err := runHTTPJob(ctx, "fixture", func(context.Context, string) (string, error) {
		calls++
		cancel()
		return "", errors.New("upstream failed")
	}, func(int) time.Duration { return time.Hour })
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("calls=%d error=%v", calls, err)
	}
}

func TestHTTPJobCancellationReleasesActiveRequest(t *testing.T) {
	var calls atomic.Int32
	entered := make(chan struct{})
	disconnected := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		close(entered)
		<-r.Context().Done()
		close(disconnected)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- runHTTPJob(ctx, server.URL, pkg.Get, func(int) time.Duration { return time.Hour }) }()
	<-entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("request did not cancel")
	}
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("upstream connection remains active")
	}
	if calls.Load() != 1 {
		t.Fatal("request retried after cancellation")
	}
}
