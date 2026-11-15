package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchCancelStopsTheJobWhenToldOrWhenItCannotTell(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer func(n int32) (bool, error)
	}{
		{"cancel requested", func(n int32) (bool, error) { return n >= 2, nil }},
		{"state unreadable", func(int32) (bool, error) { return false, errors.New("database is closed") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var polls atomic.Int32
			stop := WatchCancel(ctx, cancel, func(context.Context) (bool, error) { return tc.answer(polls.Add(1)) })
			defer stop()
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
				t.Fatal("the job was not cancelled")
			}
		})
	}
}

func TestWatchCancelStopWaitsAndLeavesTheJobRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var polls atomic.Int32
	stop := WatchCancel(ctx, cancel, func(context.Context) (bool, error) { polls.Add(1); return false, nil })
	time.Sleep(CancelInterval + 100*time.Millisecond)
	stop()
	seen := polls.Load()
	time.Sleep(CancelInterval + 100*time.Millisecond)
	if polls.Load() != seen || seen == 0 {
		t.Fatalf("polls %d before stop, %d after; want some, then none", seen, polls.Load())
	}
	if ctx.Err() != nil {
		t.Fatal("stopping the watch cancelled the job")
	}
}
