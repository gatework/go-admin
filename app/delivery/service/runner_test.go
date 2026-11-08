package service

import (
	"context"
	"errors"
	"github.com/gatework/go-net-connector/connector"
	"go-admin/app/delivery/models"
	"go-admin/app/delivery/service/dto"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSession struct {
	connect func(context.Context) error
	send    func(context.Context, string) (string, error)
	closed  atomic.Bool
}

func (f *fakeSession) Connect(c context.Context) error {
	if f.connect != nil {
		return f.connect(c)
	}
	return nil
}
func (f *fakeSession) Send(c context.Context, s string) (string, error) {
	if f.send != nil {
		return f.send(c, s)
	}
	return "ok\n", nil
}
func (f *fakeSession) Execute(ctx context.Context, script connector.Script, h ExecutionHooks) (string, error) {
	for _, c := range script.Commands {
		if err := h.Before(ctx); err != nil {
			return "", err
		}
		out, err := f.Send(ctx, c.Text)
		if err != nil {
			return out, err
		}
		if err = h.Receipt(out); err != nil {
			return "", err
		}
		if err = ctx.Err(); err != nil {
			return "", err
		}
	}
	return "", nil
}
func (f *fakeSession) Close() error { f.closed.Store(true); return nil }
func queued(t *testing.T, s *Workflow) models.Order {
	t.Helper()
	o, e := s.Create(context.Background(), request(), 7, "tester", scope())
	if e != nil {
		t.Fatal(e)
	}
	o, e = s.Action(context.Background(), o.ID, o.Version, "execute", 7, scope())
	if e != nil {
		t.Fatal(e)
	}
	return o
}
func TestClaimsAndCrossOrderDeviceLease(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	queued(t, s)
	r := request()
	r.RequestKey = "request-unique-0002"
	o, e := s.Create(ctx, r, 7, "tester", scope())
	if e != nil {
		t.Fatal(e)
	}
	s.Action(ctx, o.ID, o.Version, "execute", 7, scope())
	w := NewWorker(s, func(models.Target) (Session, error) { return &fakeSession{}, nil })
	first, e := w.Claim(ctx)
	if e != nil || first == nil {
		t.Fatal(e)
	}
	second, e := w.Claim(ctx)
	if e != nil || second != nil {
		t.Fatalf("overlap %+v %v", second, e)
	}
	w.RunTarget(ctx, *first)
	second, e = w.Claim(ctx)
	if e != nil || second == nil {
		t.Fatalf("lease not released %v", e)
	}
}
func TestCancellationAfterSlowAuthenticationSendsNothing(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o := queued(t, s)
	var sends atomic.Int32
	f := &fakeSession{connect: func(context.Context) error {
		current, e := s.Get(ctx, o.ID, scope())
		if e != nil {
			return e
		}
		_, e = s.Action(ctx, o.ID, current.Version, "cancel", 7, scope())
		return e
	}, send: func(context.Context, string) (string, error) { sends.Add(1); return "", nil }}
	w := NewWorker(s, func(models.Target) (Session, error) { return f, nil })
	target, e := w.Claim(ctx)
	if e != nil {
		t.Fatal(e)
	}
	w.RunTarget(ctx, *target)
	out, e := s.Target(ctx, o.ID, target.ID, scope())
	if e != nil || sends.Load() != 0 || out.Status != "cancelled" || !f.closed.Load() {
		t.Fatalf("cancel race: %+v sends %d %v", out, sends.Load(), e)
	}
}
func TestReceiptSurvivesLaterFailureAndCannotRetryUncertain(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o := queued(t, s)
	sent := 0
	w := NewWorker(s, func(models.Target) (Session, error) {
		return &fakeSession{send: func(context.Context, string) (string, error) {
			sent++
			if sent == 2 {
				return "partial", errors.New("timeout")
			}
			return "first receipt\n", nil
		}}, nil
	})
	target, _ := w.Claim(ctx)
	w.RunTarget(ctx, *target)
	out, e := s.Target(ctx, o.ID, target.ID, scope())
	if e != nil || out.Status != "uncertain" || out.CompletedSteps != 1 || out.Output != "first receipt\npartial" || out.Retryable {
		t.Fatalf("receipt %+v %v", out, e)
	}
	o, _ = s.Get(ctx, o.ID, scope())
	if _, e = s.Retry(ctx, o.ID, dto.Action{Version: o.Version, RequestKey: "retry-00001"}, 7, "tester", scope()); e == nil {
		t.Fatal("uncertain retried")
	}
	var n int64
	s.Orm.Model(&models.Receipt{}).Count(&n)
	if n != 1 {
		t.Fatal(n)
	}
}
func TestRecoveryQuarantinesWithoutReplaying(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o := queued(t, s)
	w := NewWorker(s, nil)
	target, _ := w.Claim(ctx)
	past := time.Now().UTC().Add(-time.Hour)
	s.Orm.Model(&models.Target{}).Where("id = ?", target.ID).Updates(map[string]any{"lease_until": past, "attempted": true, "completed_steps": 1, "output": "saved"})
	if e := w.Recover(ctx); e != nil {
		t.Fatal(e)
	}
	out, _ := s.Target(ctx, o.ID, target.ID, scope())
	if out.Status != "uncertain" || out.Output != "saved" || out.CompletedSteps != 1 {
		t.Fatalf("recovery %+v", out)
	}
	var lease models.DeviceLease
	s.Orm.First(&lease)
	if !lease.Quarantined {
		t.Fatal("lease not quarantined")
	}
	if next, e := w.Claim(ctx); e != nil || next != nil {
		t.Fatal("replayed recovered target")
	}
}

func TestConnectionFailureCanRetryOnlySafeTargets(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o := queued(t, s)
	w := NewWorker(s, func(models.Target) (Session, error) {
		return &fakeSession{connect: func(context.Context) error { return errors.New("refused") }}, nil
	})
	target, _ := w.Claim(ctx)
	w.RunTarget(ctx, *target)
	o, _ = s.Get(ctx, o.ID, scope())
	if !o.Retryable || !o.Targets[0].Retryable {
		t.Fatalf("safe failed target not retryable %+v", o)
	}
	retry, e := s.Retry(ctx, o.ID, dto.Action{Version: o.Version, RequestKey: "retry-safe-0001"}, 7, "tester", scope())
	if e != nil || retry.RetryOfID == nil || *retry.RetryOfID != o.ID || retry.Status != "scheduled" || len(retry.Targets) != 1 {
		t.Fatalf("retry %+v %v", retry, e)
	}
}
func TestInventoryChangedDuringAuthenticationPreventsWrites(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o := queued(t, s)
	var sent atomic.Int32
	w := NewWorker(s, func(models.Target) (Session, error) {
		return &fakeSession{connect: func(context.Context) error {
			return s.Orm.Exec("UPDATE sys_network_device SET ip = ? WHERE id = 1", "192.0.2.99").Error
		}, send: func(context.Context, string) (string, error) { sent.Add(1); return "", nil }}, nil
	})
	target, _ := w.Claim(ctx)
	w.RunTarget(ctx, *target)
	out, _ := s.Target(ctx, o.ID, target.ID, scope())
	if out.Status != "failed" || out.ErrorCode != "device_changed" || sent.Load() != 0 {
		t.Fatalf("identity %+v", out)
	}
}
func TestDueScheduledOrderDoesNotNeedBrowserPolling(t *testing.T) {
	s := fixture(t)
	s.Config.Grace = -time.Minute
	ctx := context.Background()
	o, e := s.Create(ctx, request(), 7, "tester", scope())
	if e != nil {
		t.Fatal(e)
	}
	w := NewWorker(s, func(models.Target) (Session, error) { return &fakeSession{}, nil })
	w.Start()
	defer w.Close(ctx)
	deadline := time.After(4 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("due job not executed")
		case <-ticker.C:
			current, e := s.Get(ctx, o.ID, scope())
			if e != nil {
				t.Fatal(e)
			}
			if current.Status == "succeeded" {
				return
			}
		}
	}
}
func TestConcurrentClaimsExecuteTargetOnce(t *testing.T) {
	s := fixture(t)
	queued(t, s)
	w := NewWorker(s, nil)
	var claims atomic.Int32
	done := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func() {
			target, e := w.Claim(context.Background())
			if target != nil {
				claims.Add(1)
			}
			done <- e
		}()
	}
	for i := 0; i < 12; i++ {
		if e := <-done; e != nil {
			t.Fatal(e)
		}
	}
	if claims.Load() != 1 {
		t.Fatalf("claims %d", claims.Load())
	}
}

func TestReceiptPersistenceFailureStopsSubsequentSends(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o := queued(t, s)
	if e := s.Orm.Exec(`CREATE TRIGGER reject_receipt BEFORE INSERT ON sys_delivery_receipt BEGIN SELECT RAISE(ABORT,'fixture'); END;`).Error; e != nil {
		t.Fatal(e)
	}
	var sent atomic.Int32
	w := NewWorker(s, func(models.Target) (Session, error) {
		return &fakeSession{send: func(context.Context, string) (string, error) { sent.Add(1); return "acknowledged\n", nil }}, nil
	})
	target, _ := w.Claim(ctx)
	w.RunTarget(ctx, *target)
	out, _ := s.Target(ctx, o.ID, target.ID, scope())
	if sent.Load() != 1 || out.Status != "uncertain" || out.ErrorCode != "receipt_persistence_failed" || out.Output != "acknowledged\n" {
		t.Fatalf("receipt failure replayed writes %+v sent %d", out, sent.Load())
	}
}
func TestExecutionTimeoutIsUncertainAndNeverReclaimed(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o := queued(t, s)
	var sent atomic.Int32
	w := NewWorker(s, func(models.Target) (Session, error) {
		return &fakeSession{send: func(c context.Context, _ string) (string, error) {
			sent.Add(1)
			// A connector command timeout starts after entering Send. Expire
			// that deadline here so slow pre-write DB checks cannot consume it.
			commandCtx, cancel := context.WithDeadline(c, time.Now())
			defer cancel()
			<-commandCtx.Done()
			return "partial\n", commandCtx.Err()
		}}, nil
	})
	target, err := w.Claim(ctx)
	if err != nil || target == nil {
		t.Fatalf("claim: target=%v error=%v", target, err)
	}
	w.RunTarget(ctx, *target)
	out, err := s.Target(ctx, o.ID, target.ID, scope())
	if err != nil || sent.Load() != 1 || out.Status != "uncertain" || out.Output != "partial\n" {
		t.Fatalf("timeout %+v sends=%d error=%v", out, sent.Load(), err)
	}
	if next, e := w.Claim(ctx); e != nil || next != nil {
		t.Fatalf("timeout replay %+v %v", next, e)
	}
}

func TestAcknowledgedReceiptPersistsWhenCancellationArrivesAtReturn(t *testing.T) {
	s := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	o := queued(t, s)
	w := NewWorker(s, func(models.Target) (Session, error) {
		return &fakeSession{send: func(context.Context, string) (string, error) { cancel(); return "confirmed\n", nil }}, nil
	})
	target, _ := w.Claim(ctx)
	w.RunTarget(ctx, *target)
	out, _ := s.Target(context.Background(), o.ID, target.ID, scope())
	var count int64
	s.Orm.Model(&models.Receipt{}).Where("target_id = ?", target.ID).Count(&count)
	if out.CompletedSteps != 1 || count != 1 || out.Output != "confirmed\n" {
		t.Fatalf("acknowledgement lost on cancellation %+v receipts %d", out, count)
	}
}
