package jobs

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg/cronjob"
	"github.com/robfig/cron/v3"
)

type blockingCronSchedule struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (s *blockingCronSchedule) Next(now time.Time) time.Time {
	s.once.Do(func() { close(s.entered) })
	<-s.release
	return now.Add(time.Hour)
}

func TestCronControlBoundsBlockedWorkersAndOrdersStopBeforeRestart(t *testing.T) {
	c := cronjob.NewWithSeconds()
	schedule := &blockingCronSchedule{entered: make(chan struct{}), release: make(chan struct{})}
	c.Schedule(schedule, cron.FuncJob(func() {}))
	control := &cronControl{cron: c}
	if err := control.run(t.Context(), func() error { control.start(); return nil }); err != nil {
		t.Fatal(err)
	}
	<-schedule.entered
	var calls atomic.Int64
	for range 5 {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		err := control.run(ctx, func() error { calls.Add(1); c.Entries(); return ctx.Err() })
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("blocked operation: %v", err)
		}
		control.requestStop()
	}
	if calls.Load() != 1 {
		t.Fatalf("timed-out callers spawned %d blocked workers", calls.Load())
	}
	close(schedule.release)
	// A new lease may schedule only after the delayed old Stop has completed.
	if err := control.run(t.Context(), func() error {
		control.mu.Lock()
		started := control.started
		control.mu.Unlock()
		if started {
			return errors.New("restart overtook pending stop")
		}
		control.start()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	control.requestStop()
	control.waitStopped(t.Context())
}

func TestBlockedCronCannotBlockLeaseExpiryOrShutdown(t *testing.T) {
	db := leaseDB(t)
	const key = "blocked-custom-cron"
	c := cronjob.NewWithSeconds()
	schedule := &blockingCronSchedule{entered: make(chan struct{}), release: make(chan struct{})}
	c.Schedule(schedule, cron.FuncJob(func() {}))
	sdk.Runtime.SetCrontabByTenant(key, c)
	s := newSupervisor(key, db, "blocked-owner")
	s.lease.ttl = 100 * time.Millisecond
	s.tick()
	<-schedule.entered
	reconcileDone := make(chan struct{})
	go func() { defer close(reconcileDone); s.reconcile(context.Background()) }()
	time.Sleep(150 * time.Millisecond)
	checked := make(chan bool, 1)
	go func() { checked <- s.holdsLease() }()
	select {
	case held := <-checked:
		if held {
			t.Error("blocked cron extended lease")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("blocked cron held supervisor state mutex")
	}
	close(s.done) // this fixture uses synchronous tick rather than heartbeat
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	stopped := make(chan struct{})
	go func() { defer close(stopped); s.shutdown(ctx) }()
	select {
	case <-stopped:
	case <-time.After(100 * time.Millisecond):
		t.Error("shutdown ignored context while cron blocked")
	}
	cancel()
	close(schedule.release)
	<-reconcileDone
	s.control.waitStopped(t.Context())
}
