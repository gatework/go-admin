package delivery

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gatework/go-net-connector/connector"
	models "github.com/gatework/motor/app/net/models/delivery"
	dto "github.com/gatework/motor/app/net/service/dto/delivery"
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
	if !errors.Is(e, NoWorkErr) || second != nil {
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
	s.DB.Model(&models.Receipt{}).Count(&n)
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
	s.DB.Model(&models.Target{}).Where("id = ?", target.ID).Updates(map[string]any{"claim_until": past, "attempted": true, "completed_steps": 1, "output": "saved"})
	if e := w.Recover(ctx); e != nil {
		t.Fatal(e)
	}
	out, _ := s.Target(ctx, o.ID, target.ID, scope())
	if out.Status != "uncertain" || out.Output != "saved" || out.CompletedSteps != 1 {
		t.Fatalf("recovery %+v", out)
	}
	var lease models.DeviceLease
	s.DB.First(&lease)
	if !lease.Quarantined {
		t.Fatal("lease not quarantined")
	}
	if next, e := w.Claim(ctx); !errors.Is(e, NoWorkErr) || next != nil {
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
			return s.DB.Exec("UPDATE sys_network_device SET ip = ? WHERE id = 1", "192.0.2.99").Error
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
	workerCtx, stop := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- w.Run(workerCtx) }()
	t.Cleanup(func() {
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("worker returned %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("worker did not stop")
		}
	})
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
	for range 12 {
		go func() {
			target, e := w.Claim(context.Background())
			if target != nil {
				claims.Add(1)
			}
			done <- e
		}()
	}
	for range 12 {
		if e := <-done; e != nil && !errors.Is(e, NoWorkErr) {
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
	if e := s.DB.Exec(`CREATE TRIGGER reject_receipt BEFORE INSERT ON sys_delivery_receipt BEGIN SELECT RAISE(ABORT,'fixture'); END;`).Error; e != nil {
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
	if next, e := w.Claim(ctx); !errors.Is(e, NoWorkErr) || next != nil {
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
	s.DB.Model(&models.Receipt{}).Where("target_id = ?", target.ID).Count(&count)
	if out.CompletedSteps != 1 || count != 1 || out.Output != "confirmed\n" {
		t.Fatalf("acknowledgement lost on cancellation %+v receipts %d", out, count)
	}
}

func TestQuarantinedDeviceFailsPendingTargetVisibly(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o := queued(t, s)
	var target models.Target
	if e := s.DB.Where("order_id = ?", o.ID).First(&target).Error; e != nil {
		t.Fatal(e)
	}
	// Another module left this device quarantined.
	if e := s.DB.Create(&models.DeviceLease{DeviceID: target.DeviceID, Token: "other-module", ExpiresAt: time.Now().UTC().Add(time.Hour), Quarantined: true}).Error; e != nil {
		t.Fatal(e)
	}
	w := NewWorker(s, func(models.Target) (Session, error) { return &fakeSession{}, nil })
	if next, e := w.Claim(ctx); !errors.Is(e, NoWorkErr) || next != nil {
		t.Fatalf("claimed work on a quarantined device %+v %v", next, e)
	}
	out, _ := s.Target(ctx, o.ID, target.ID, scope())
	if out.Status != "failed" || out.ErrorCode != "device_quarantined" {
		t.Fatalf("pending target on a quarantined device %+v", out)
	}
	var row models.DeviceLease
	if e := s.DB.First(&row, "device_id = ?", target.DeviceID).Error; e != nil || !row.Quarantined || row.Token != "other-module" {
		t.Fatal("the quarantine changed", e)
	}
}

// The pre-command check reports what it found. A database it cannot read is
// a failed check, not a cancelled order or a changed device: those two decide
// what an operator is told and whether a target may be retried.
func TestCheckDoesNotDisguiseDatabaseFailures(t *testing.T) {
	for _, tc := range []struct {
		name, sql string
		want      error
	}{
		{"lease table unreadable", "DROP TABLE sys_delivery_device_lease", errCheck},
		{"device table unreadable", "DROP TABLE sys_network_device", errCheck},
		{"device deleted", "DELETE FROM sys_network_device", IdentityErr},
		{"order cancelled", "UPDATE sys_delivery_order SET cancel_requested = true", CancelledErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := fixture(t)
			queued(t, s)
			w := NewWorker(s, func(models.Target) (Session, error) { return &fakeSession{}, nil })
			target, e := w.Claim(context.Background())
			if e != nil || target == nil {
				t.Fatal(e)
			}
			if err := s.DB.Exec(tc.sql).Error; err != nil {
				t.Fatal(err)
			}
			e = w.check(context.Background(), *target)
			if !errors.Is(e, tc.want) {
				t.Fatalf("check = %v, want %v", e, tc.want)
			}
			for _, other := range []error{errCheck, IdentityErr, CancelledErr} {
				if !errors.Is(tc.want, other) && errors.Is(e, other) {
					t.Fatalf("check = %v, also reads as %v", e, other)
				}
			}
		})
	}
}

// The audit trail says which scripts were approved and how the order ended:
// create and edit carry a digest of every target's script, and the order's
// first terminal status is recorded once.
func TestAuditRecordsScriptsAndTheFinalResult(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o := queued(t, s)
	w := NewWorker(s, func(models.Target) (Session, error) { return &fakeSession{}, nil })
	target, e := w.Claim(ctx)
	if e != nil || target == nil {
		t.Fatal(e)
	}
	w.RunTarget(ctx, *target)
	if e := w.Recover(ctx); e != nil {
		t.Fatal(e)
	}
	var audits []models.Audit
	if e := s.DB.Where("order_id = ?", o.ID).Order("id").Find(&audits).Error; e != nil {
		t.Fatal(e)
	}
	var targets []models.Target
	s.DB.Where("order_id = ?", o.ID).Find(&targets)
	byAction := map[string][]models.Audit{}
	for _, a := range audits {
		byAction[a.Action] = append(byAction[a.Action], a)
	}
	if c := byAction["create"]; len(c) != 1 || c[0].ScriptsSHA256 == "" || c[0].ScriptsSHA256 != scriptsDigest(targets) {
		t.Errorf("create audit %+v; want the scripts digest %s", c, scriptsDigest(targets))
	}
	if f := byAction["finish"]; len(f) != 1 || f[0].Result != "succeeded" {
		t.Errorf("finish audits %+v; want one with result succeeded", f)
	}
}

func TestCancellingAnOrderBeforeItRunsRecordsItsResult(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o, e := s.Create(ctx, request(), 7, "tester", scope())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Action(ctx, o.ID, o.Version, "cancel", 7, scope()); e != nil {
		t.Fatal(e)
	}
	var finish []models.Audit
	s.DB.Where("order_id = ? AND action = ?", o.ID, "finish").Find(&finish)
	if len(finish) != 1 || finish[0].Result != "cancelled" {
		t.Fatalf("finish audits %+v; want one with result cancelled", finish)
	}
}

// Recovery settles a bounded batch per pass, and a larger backlog drains over
// the following passes.
func TestRecoveryDrainsABacklogInBatches(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o := queued(t, s)
	var first models.Target
	if e := s.DB.Where("order_id = ?", o.ID).First(&first).Error; e != nil {
		t.Fatal(e)
	}
	past := time.Now().UTC().Add(-time.Hour)
	rows := make([]models.Target, recoverBatch+1)
	for i := range rows {
		rows[i] = first
		rows[i].ID = 0
		rows[i].Status, rows[i].ClaimToken, rows[i].ClaimUntil = "running", "lost", &past
	}
	if e := s.DB.Where("id = ?", first.ID).Delete(&models.Target{}).Error; e != nil {
		t.Fatal(e)
	}
	if e := s.DB.CreateInBatches(rows, 50).Error; e != nil {
		t.Fatal(e)
	}
	w := NewWorker(s, func(models.Target) (Session, error) { return &fakeSession{}, nil })
	running := func() int64 {
		var n int64
		s.DB.Model(&models.Target{}).Where("status = ?", "running").Count(&n)
		return n
	}
	if e := w.Recover(ctx); e != nil || running() != 1 {
		t.Fatalf("after one pass: %d still running, err %v", running(), e)
	}
	if e := w.Recover(ctx); e != nil || running() != 0 {
		t.Fatalf("after two passes: %d still running, err %v", running(), e)
	}
}
