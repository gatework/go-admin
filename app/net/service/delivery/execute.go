package delivery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/gatework/go-net-connector/connector"
	models "github.com/gatework/motor/app/net/models/delivery"
	"github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/models/lease"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	"github.com/gatework/motor/common/worker"
)

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

// checkErr is a pre-command check that could not run, usually because the
// database could not be read. It is not a finding about the order or device.
//
//revive:disable-next-line:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var checkErr = errors.New("delivery: pre-command check could not run")

// check confirms, before the target touches its device, that it still holds
// its device lease and claim, that its order was not cancelled, and that the
// device and its connection are still the ones planned. A failed finding is
// CancelledErr or IdentityErr; a check that could not run is errCheck.
func (w *Worker) check(ctx context.Context, t models.Target) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	failed := func(e error) error { return fmt.Errorf("%w: %w", checkErr, e) }
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
	persist, done := worker.Cleanup(ctx, persistTimeout)
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
	} else if errors.Is(e, checkErr) {
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
