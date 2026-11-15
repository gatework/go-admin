package delivery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/gatework/go-net-connector/connector"
	"github.com/gatework/motor-core/logger"
	models "github.com/gatework/motor/app/net/models/delivery"
	"github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/models/lease"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	"github.com/gatework/motor/common/worker"
)

const executionTimeout = 10 * time.Minute

// NoWorkErr indicates that no delivery target is currently claimable.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var NoWorkErr = errors.New("no delivery work available")

//revive:enable:error-naming

// Session exposes no retries: every Send represents a potentially irreversible write.
type Session interface {
	Connect(ctx context.Context) error
	Execute(ctx context.Context, script connector.Script, hooks ExecutionHooks) (string, error)
	Close() error
}

// ExecutionHooks records authorization and execution receipts around device commands.
type ExecutionHooks struct {
	Before  func(context.Context) error
	Receipt func(string) error
}

// Factory creates an independently owned device session for a target.
type Factory func(models.Target) (Session, error)

// Worker advances queued operations while holding the required device leases.
type Worker struct {
	loop    worker.Loop
	s       *Workflow
	factory Factory
}

// NewWorker constructs an application-owned background worker.
func NewWorker(s *Workflow, f Factory) *Worker {
	if f == nil {
		f = ConnectorFactory(s.Config)
	}
	// A background worker has no caller to return an error to; its log line is the
	// only trace of a failed tick, so it must never depend on the host's wiring.
	if s.Logger == nil {
		s.Logger = logger.NewHelper(logger.Default())
	}
	return &Worker{s: s, factory: f, loop: worker.Loop{Name: "delivery worker", Logger: s.Logger}}
}

// Run owns the worker lifecycle until cancellation and waits for its pollers.
func (w *Worker) Run(ctx context.Context) error {
	concurrency := w.s.Config.Concurrency
	if concurrency < 1 {
		concurrency = 5
	}
	return w.loop.Run(ctx, concurrency, func(ctx context.Context) error {
		if !w.s.Config.Enabled {
			return nil
		}
		if err := w.Recover(ctx); err != nil {
			return fmt.Errorf("recovery: %w", err)
		}
		target, err := w.Claim(ctx)
		if errors.Is(err, NoWorkErr) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("claim: %w", err)
		}
		if target != nil {
			w.RunTarget(ctx, *target)
		}
		return nil
	})
}

// Claim selects durable due work. The device lease is the cross-process exclusion
// boundary; work for a quarantined device fails visibly instead of waiting.
// It returns NoWorkErr when disabled or when no target is currently available.
func (w *Worker) Claim(ctx context.Context) (*models.Target, error) {
	if !w.s.Config.Enabled {
		return nil, NoWorkErr
	}
	db := w.s.db(ctx)
	now := time.Now().UTC()
	due := func() *gorm.DB {
		return db.Model(&models.Target{}).Select("sys_delivery_target.*").Joins("JOIN sys_delivery_order o ON o.id = sys_delivery_target.order_id").Where("o.kind <> 'fabric' AND sys_delivery_target.status = ? AND o.cancel_requested = ? AND (o.status IN ? OR (o.status = ? AND o.scheduled_at <= ?))", "pending", false, []string{"queued", "running"}, "scheduled", now)
	}
	if e := w.failQuarantined(ctx, due()); e != nil {
		return nil, e
	}
	var candidates []models.Target
	e := due().Scopes(lease.Free("sys_delivery_target.device_id")).Order("o.id, sys_delivery_target.id").Limit(100).Find(&candidates).Error
	if e != nil {
		return nil, e
	}
	for _, t := range candidates {
		e = db.Transaction(func(tx *gorm.DB) error { return claimTarget(tx, &t, now) })
		if e == nil {
			return &t, nil
		}
		if !errors.Is(e, ConflictErr) {
			return nil, e
		}
	}
	return nil, NoWorkErr
}

// RunTarget provides the run target operation within the caller's authorized delivery scope.
func (w *Worker) RunTarget(parent context.Context, t models.Target) {
	ctx, cancel := context.WithTimeout(parent, executionTimeout)
	defer cancel()
	// Observe persisted cancellation while slow authentication/commands are pending.
	defer worker.WatchCancel(ctx, cancel, func(query context.Context) (bool, error) {
		var o models.Order
		err := w.s.db(query).Select("id", "cancel_requested").First(&o, t.OrderID).Error
		return o.CancelRequested, err
	})()
	status, code := "failed", "connection_failed"
	output := t.Output
	attempted := t.Attempted
	steps := t.CompletedSteps
	var session Session
	// Close transport before removing its device lease. Cleanup failure quarantines.
	defer func() {
		closeOK, finalStatus, finalCode := closeSession(session, status, code)
		status, code = finalStatus, finalCode
		w.record(parent, t, status, code, output, steps, attempted, closeOK)
	}()
	fail := func(e error) { status, code = executionFailure(e, attempted, status, code) }
	if e := w.check(ctx, t); e != nil {
		fail(e)
		return
	}
	parsed, e := connector.ParseScript(t.Script, "delivery")
	if e != nil || len(parsed.Commands) == 0 {
		code = "invalid_script"
		return
	}
	session, e = w.factory(t)
	if e != nil {
		return
	}
	if e = session.Connect(ctx); e != nil {
		fail(e)
		return
	}
	if e = w.check(ctx, t); e != nil {
		fail(e)
		return
	}
	before := func(commandCtx context.Context) error {
		return w.beforeCommand(commandCtx, t, &code, &attempted)
	}
	receipt := func(part string) error {
		return w.recordReceipt(ctx, t, part, &output, &code, &steps)
	}
	tail, executeErr := session.Execute(ctx, parsed, ExecutionHooks{Before: before, Receipt: receipt})
	output, full := appendCapped(output, tail, MaxOutputBytes)
	if !full {
		executeErr = InvalidErr
	}
	if executeErr != nil {
		if code == "receipt_persistence_failed" || code == "persistence_failed" {
			if attempted {
				status = "uncertain"
			}
		} else {
			fail(executeErr)
		}
		return
	}

	status = "succeeded"
	code = ""
}

// Recover reconciles expired work without assuming unconfirmed device outcomes succeeded.
func (w *Worker) Recover(ctx context.Context) error {
	// The sweep quarantines this module's expired leases along with every other module's.
	if e := lease.Sweep(w.s.db(ctx)); e != nil {
		return e
	}
	var targets []models.Target
	if e := w.s.db(ctx).Where("status = ? AND claim_until < ? AND order_id IN (SELECT id FROM sys_delivery_order WHERE kind <> ?)", "running", time.Now().UTC(), "fabric").Find(&targets).Error; e != nil {
		return e
	}
	for _, t := range targets {
		if e := w.s.db(ctx).Transaction(func(tx *gorm.DB) error {
			q := tx.Model(&models.Target{}).Where("id = ? AND status = ? AND claim_until < ?", t.ID, "running", time.Now().UTC()).Updates(map[string]any{"status": "uncertain", "error_code": "worker_lost", "finished_at": time.Now().UTC()})
			if q.Error != nil {
				return q.Error
			}
			if q.RowsAffected == 0 {
				return nil
			}
			// The lost worker's session outcome is unknown: quarantine its lease if
			// the sweep has not already.
			if e := lease.Release(tx, lease.Lease{DeviceID: t.DeviceID, Token: t.ClaimToken}, lease.Unconfirmed); e != nil && !errors.Is(e, lease.LostErr) {
				return e
			}
			return summarize(tx, t.OrderID)
		}); e != nil {
			return e
		}
	}
	return nil
}

// failQuarantined fails due targets whose device is quarantined, so they show
// why they cannot run instead of waiting silently.
func (w *Worker) failQuarantined(ctx context.Context, due *gorm.DB) error {
	var blocked []models.Target
	if e := due.Scopes(lease.Quarantined("sys_delivery_target.device_id")).Limit(100).Find(&blocked).Error; e != nil {
		return e
	}
	for _, t := range blocked {
		if e := w.s.db(ctx).Transaction(func(tx *gorm.DB) error {
			q := tx.Model(&models.Target{}).Where("id = ? AND status = ?", t.ID, "pending").Updates(map[string]any{"status": "failed", "error_code": "device_quarantined", "finished_at": time.Now().UTC()})
			if q.Error != nil || q.RowsAffected == 0 {
				return q.Error
			}
			return summarize(tx, t.OrderID)
		}); e != nil {
			return e
		}
	}
	return nil
}

// errCheck is a pre-command check that could not run, usually because the
// database could not be read. It is not a finding about the order or device.
var errCheck = errors.New("delivery: pre-command check could not run")

// check confirms, before the target touches its device, that it still holds
// its device lease and claim, that its order was not cancelled, and that the
// device and its connection are still the ones planned. A failed finding is
// CancelledErr or IdentityErr; a check that could not run is errCheck.
func (w *Worker) check(ctx context.Context, t models.Target) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	failed := func(e error) error { return fmt.Errorf("%w: %w", errCheck, e) }
	db := w.s.db(ctx)
	if e := lease.Check(db, lease.Lease{DeviceID: t.DeviceID, Token: t.ClaimToken}); errors.Is(e, lease.LostErr) {
		return CancelledErr
	} else if e != nil {
		return failed(e)
	}
	// The target's claim and its order's cancel flag, in one read.
	var current struct {
		Status          string
		ClaimToken      string
		ClaimUntil      *time.Time
		CancelRequested bool
	}
	read := db.Table(models.Target{}.TableName()+" t").
		Select("t.status, t.claim_token, t.claim_until, o.cancel_requested").
		Joins("JOIN "+models.Order{}.TableName()+" o ON o.id = t.order_id").
		Where("t.id = ?", t.ID).Take(&current)
	if read.Error != nil {
		return failed(read.Error)
	}
	if current.Status != "running" || current.ClaimToken != t.ClaimToken || current.ClaimUntil == nil || time.Now().After(*current.ClaimUntil) || current.CancelRequested {
		return CancelledErr
	}
	var d inventory.Device
	if e := db.First(&d, t.DeviceID).Error; errors.Is(e, gorm.ErrRecordNotFound) {
		return IdentityErr
	} else if e != nil {
		return failed(e)
	}
	if !deviceaccess.Write.Allows(d.Status) {
		return IdentityErr
	}
	c, e := w.s.connection(ctx, d)
	if e != nil {
		return failed(e)
	}
	if !matchesSnapshot(deviceSnapshot(d, c.Profile), t.Device) || c.Version != t.ConnectionVersion || c.Profile != t.Connection.Profile || c.Port != t.Connection.Port || c.Protocol != t.Connection.Protocol {
		return IdentityErr
	}
	return nil
}

func summarize(tx *gorm.DB, id int) error {
	// Lock the order through a write so concurrent completions cannot overwrite counts
	// computed before another target commits (works on SQLite as well as SQL engines).
	if e := tx.Model(&models.Order{}).Where("id = ?", id).Update("version", gorm.Expr("version + 1")).Error; e != nil {
		return e
	}
	var before models.Order
	if e := tx.Select("status").First(&before, id).Error; e != nil {
		return e
	}
	var rows []models.Target
	if e := tx.Select("status").Where("order_id = ?", id).Find(&rows).Error; e != nil {
		return e
	}
	counts := map[string]int{}
	for _, t := range rows {
		counts[t.Status]++
	}
	values := map[string]any{"succeeded_count": counts["succeeded"], "failed_count": counts["failed"], "uncertain_count": counts["uncertain"], "cancelled_count": counts["cancelled"]}
	if counts["pending"]+counts["running"] == 0 {
		status := "failed"
		switch {
		case counts["uncertain"] > 0:
			status = "uncertain"
		case counts["succeeded"] == len(rows):
			status = "succeeded"
		case counts["cancelled"] == len(rows):
			status = "cancelled"
		case counts["succeeded"] > 0:
			status = "partially_failed"
		}
		values["status"] = status
		values["finished_at"] = time.Now().UTC()
		if !finished(before.Status) {
			// The worker ended it, so there is no actor.
			if e := tx.Create(&models.Audit{OrderID: id, Action: "finish", Result: status}).Error; e != nil {
				return e
			}
		}
	}
	return tx.Model(&models.Order{}).Where("id = ?", id).Updates(values).Error
}

// record persists a target's outcome with its own short deadline, because the
// execution context is usually already cancelled or expired by this point.
func (w *Worker) record(parent context.Context, t models.Target, status, code, output string, steps int, attempted, closed bool) {
	cleanup, cancel := worker.Cleanup(parent, 5*time.Second)
	defer cancel()
	if e := w.finish(cleanup, t, status, code, output, steps, attempted, closed); e != nil {
		// The outcome is now unknown to the database: the target stays running and
		// its device lease is left to expire, which the lease sweep turns into a
		// quarantine. Say so here, or the operator only sees a device that
		// stopped taking work.
		w.s.Logger.Errorf("delivery result not recorded, device lease left to quarantine: order=%d target=%d device=%d status=%s: %v", t.OrderID, t.ID, t.DeviceID, status, e)
	}
}

func (w *Worker) finish(ctx context.Context, t models.Target, status, code, output string, steps int, attempted, closed bool) error {
	return w.s.db(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&models.Target{}).Where("id = ? AND status = ? AND claim_token = ?", t.ID, "running", t.ClaimToken).Updates(map[string]any{"status": status, "error_code": code, "output": output, "completed_steps": steps, "attempted": attempted, "finished_at": time.Now().UTC()})
		if q.Error != nil {
			return q.Error
		}
		outcome := lease.Closed
		if !closed {
			outcome = lease.Unconfirmed
		}
		// A lost lease was quarantined meanwhile; the quarantine stays and the result is still recorded.
		if e := lease.Release(tx, lease.Lease{DeviceID: t.DeviceID, Token: t.ClaimToken}, outcome); e != nil && !errors.Is(e, lease.LostErr) {
			return e
		}
		return summarize(tx, t.OrderID)
	})
}

func claimTarget(tx *gorm.DB, t *models.Target, now time.Time) error {
	held, e := lease.Acquire(tx, t.DeviceID, lease.Holder{Kind: lease.DeliveryTarget, ID: t.ID}, executionTimeout)
	if errors.Is(e, lease.BusyErr) || errors.Is(e, lease.QuarantinedErr) {
		return ConflictErr
	}
	if e != nil {
		return e
	}
	token, until := held.Token, held.ExpiresAt
	var o models.Order
	if e := tx.First(&o, t.OrderID).Error; e != nil {
		return e
	}
	if o.Kind == "fabric" || o.CancelRequested || !(o.Status == "running" || o.Status == "queued" || o.Status == "scheduled" && o.ScheduledAt != nil && !o.ScheduledAt.After(now)) {
		return ConflictErr
	}
	// Order CAS serializes claim versus cancel/edit, including already-running jobs.
	values := map[string]any{"status": "running", "version": gorm.Expr("version + 1")}
	if o.StartedAt == nil {
		values["started_at"] = now
	}
	q := tx.Model(&models.Order{}).Where("id = ? AND version = ? AND cancel_requested = ?", o.ID, o.Version, false).Updates(values)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return ConflictErr
	}
	q = tx.Model(&models.Target{}).Where("id = ? AND status = ?", t.ID, "pending").Updates(map[string]any{"status": "running", "started_at": now, "claim_token": token, "claim_until": until})
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return ConflictErr
	}
	t.Status = "running"
	t.ClaimToken = token
	t.ClaimUntil = &until
	t.StartedAt = &now
	return nil
}

func (w *Worker) beforeCommand(commandCtx context.Context, t models.Target, code *string, attempted *bool) error {
	if err := w.check(commandCtx, t); err != nil {
		return err
	}
	q := w.s.db(commandCtx).Model(&models.Target{}).Where("id = ? AND status = ? AND claim_token = ?", t.ID, "running", t.ClaimToken).Update("attempted", true)
	if q.Error != nil {
		*code = "persistence_failed"
		return q.Error
	}
	if q.RowsAffected != 1 {
		*code = "persistence_failed"
		return ConflictErr
	}
	*attempted = true
	return w.check(commandCtx, t)
}

func (w *Worker) recordReceipt(ctx context.Context, t models.Target, part string, output, code *string, steps *int) error {
	capped, full := appendCapped(*output, part, MaxOutputBytes)
	*output = capped
	if !full {
		*code = "output_limit"
		return InvalidErr
	}
	next := *steps + 1
	// An acknowledged remote command is evidence even if cancellation arrived
	// in the same instant. Persist it under its own bounded cleanup context.
	persist, done := worker.Cleanup(ctx, 5*time.Second)
	defer done()
	err := w.s.db(persist).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&models.Receipt{TargetID: t.ID, Step: next, Output: part}).Error; err != nil {
			return err
		}
		q := tx.Model(&models.Target{}).Where("id = ? AND status = ? AND claim_token = ?", t.ID, "running", t.ClaimToken).Updates(map[string]any{"completed_steps": next, "output": *output})
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return ConflictErr
		}
		return nil
	})
	if err != nil {
		*code = "receipt_persistence_failed"
		return err
	}
	*steps = next
	return nil
}

func executionFailure(e error, attempted bool, status, code string) (string, string) {
	if attempted {
		status = "uncertain"
		code = "execution_unconfirmed"
	} else if errors.Is(e, CancelledErr) || errors.Is(e, context.Canceled) {
		status = "cancelled"
		code = "cancelled"
	} else if errors.Is(e, IdentityErr) {
		status = "failed"
		code = "device_changed"
	} else if errors.Is(e, errCheck) {
		status = "failed"
		code = "check_failed"
	}
	return status, code
}

func closeSession(session Session, status, code string) (bool, string, string) {
	closed := session == nil || session.Close() == nil
	if !closed {
		return false, "uncertain", "close_unconfirmed"
	}
	return true, status, code
}
