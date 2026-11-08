package service

import (
	"context"
	"errors"
	"github.com/gatework/go-net-connector/connector"
	"github.com/google/uuid"
	"go-admin/app/delivery/models"
	devicelease "go-admin/app/network/lease"
	inventory "go-admin/app/network/models"
	"gorm.io/gorm"
	"sync"
	"time"
)

const executionTimeout = 10 * time.Minute
const leaseDuration = 15 * time.Minute

// Session exposes no retries: every Send represents a potentially irreversible write.
type Session interface {
	Connect(context.Context) error
	Execute(context.Context, connector.Script, ExecutionHooks) (string, error)
	Close() error
}
type ExecutionHooks struct {
	Before  func(context.Context) error
	Receipt func(string) error
}

type Factory func(models.Target) (Session, error)
type Worker struct {
	s       *Workflow
	factory Factory
	cancel  context.CancelFunc
	done    chan struct{}
	once    sync.Once
}

func NewWorker(s *Workflow, f Factory) *Worker {
	if f == nil {
		f = ConnectorFactory(s.Config)
	}
	return &Worker{s: s, factory: f, done: make(chan struct{})}
}
func (w *Worker) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	go func() {
		defer close(w.done)
		var wg sync.WaitGroup
		n := w.s.Config.Concurrency
		if n < 1 {
			n = 5
		}
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						if !w.s.Config.Enabled {
							continue
						}
						if e := w.Recover(ctx); e != nil {
							continue
						}
						t, e := w.Claim(ctx)
						if e == nil && t != nil {
							w.RunTarget(ctx, *t)
						}
					}
				}
			}()
		}
		wg.Wait()
	}()
}
func (w *Worker) Close(ctx context.Context) {
	w.once.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
	})
	if w.cancel == nil {
		return
	}
	select {
	case <-w.done:
	case <-ctx.Done():
	}
}

// Claim selects durable due work. A unique device row is the cross-process exclusion
// boundary. Expired rows are never stolen: the former process may still be alive.
func (w *Worker) Claim(ctx context.Context) (*models.Target, error) {
	if !w.s.Config.Enabled {
		return nil, nil
	}
	db := w.s.db(ctx)
	now := time.Now().UTC()
	var candidates []models.Target
	e := db.Model(&models.Target{}).Select("sys_delivery_target.*").Joins("JOIN sys_delivery_order o ON o.id = sys_delivery_target.order_id").Where("o.kind <> 'fabric' AND sys_delivery_target.device_id NOT IN (?) AND sys_delivery_target.status = ? AND o.cancel_requested = ? AND (o.status IN ? OR (o.status = ? AND o.scheduled_at <= ?))", devicelease.OccupiedDevices(w.s.db(ctx)), "pending", false, []string{"queued", "running"}, "scheduled", now).Order("o.id, sys_delivery_target.id").Limit(100).Find(&candidates).Error
	if e != nil {
		return nil, e
	}
	for _, t := range candidates {
		token := uuid.NewString()
		until := now.Add(leaseDuration)
		e = db.Transaction(func(tx *gorm.DB) error {
			acquired, err := devicelease.TryAcquire(tx, devicelease.Claim{Owner: devicelease.Owner{DeviceID: t.DeviceID, Token: token}, TargetID: t.ID, ExpiresAt: until})
			if err != nil {
				return err
			}
			if !acquired {
				return ErrConflict
			}
			var o models.Order
			if e := tx.First(&o, t.OrderID).Error; e != nil {
				return e
			}
			if o.Kind == "fabric" || o.CancelRequested || !(o.Status == "running" || o.Status == "queued" || o.Status == "scheduled" && o.ScheduledAt != nil && !o.ScheduledAt.After(now)) {
				return ErrConflict
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
				return ErrConflict
			}
			q = tx.Model(&models.Target{}).Where("id = ? AND status = ?", t.ID, "pending").Updates(map[string]any{"status": "running", "started_at": now, "lease_token": token, "lease_until": until})
			if q.Error != nil {
				return q.Error
			}
			if q.RowsAffected != 1 {
				return ErrConflict
			}
			t.Status = "running"
			t.LeaseToken = token
			t.LeaseUntil = &until
			t.StartedAt = &now
			return nil
		})
		if e == nil {
			return &t, nil
		}
		if !errors.Is(e, ErrConflict) {
			return nil, e
		}
	}
	return nil, nil
}
func (w *Worker) check(ctx context.Context, t models.Target) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	var current models.Target
	if e := w.s.db(ctx).First(&current, t.ID).Error; e != nil {
		return e
	}
	if current.Status != "running" || current.LeaseToken != t.LeaseToken || current.LeaseUntil == nil || time.Now().After(*current.LeaseUntil) {
		return ErrCancelled
	}
	var o models.Order
	if e := w.s.db(ctx).First(&o, t.OrderID).Error; e != nil {
		return e
	}
	if o.CancelRequested {
		return ErrCancelled
	}
	var d inventory.Device
	if e := w.s.db(ctx).First(&d, t.DeviceID).Error; e != nil {
		return ErrIdentity
	}
	if d.Status != "active" {
		return ErrIdentity
	}
	c, e := w.s.connection(ctx, d)
	if e != nil {
		return ErrIdentity
	}
	if !matchesSnapshot(deviceSnapshot(d, c.Profile), t.Device) || c.Version != t.ConnectionVersion || c.Profile != t.Connection.Profile || c.Port != t.Connection.Port || c.Protocol != t.Connection.Protocol {
		return ErrIdentity
	}
	return nil
}
func (w *Worker) RunTarget(parent context.Context, t models.Target) {
	ctx, cancel := context.WithTimeout(parent, executionTimeout)
	defer cancel()
	// Observe persisted cancellation while slow authentication/commands are pending.
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-watchDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				var o models.Order
				query, c := context.WithTimeout(context.Background(), time.Second)
				e := w.s.db(query).Select("id", "cancel_requested").First(&o, t.OrderID).Error
				c()
				if e != nil || o.CancelRequested {
					cancel()
					return
				}
			}
		}
	}()
	status, code := "failed", "connection_failed"
	output := t.Output
	attempted := t.Attempted
	steps := t.CompletedSteps
	var session Session
	// Close transport before removing its device lease. Cleanup failure quarantines.
	defer func() {
		closeOK := true
		if session != nil {
			closeOK = session.Close() == nil
		}
		if !closeOK {
			status = "uncertain"
			code = "close_unconfirmed"
		}
		cleanup, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = w.finish(cleanup, t, status, code, output, steps, attempted, closeOK)
	}()
	fail := func(e error) {
		if attempted {
			status = "uncertain"
			code = "execution_unconfirmed"
		} else if errors.Is(e, ErrCancelled) || errors.Is(e, context.Canceled) {
			status = "cancelled"
			code = "cancelled"
		} else if errors.Is(e, ErrIdentity) {
			status = "failed"
			code = "device_changed"
		}
	}
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
		if err := w.check(commandCtx, t); err != nil {
			return err
		}
		q := w.s.db(commandCtx).Model(&models.Target{}).Where("id = ? AND status = ? AND lease_token = ?", t.ID, "running", t.LeaseToken).Update("attempted", true)
		if q.Error != nil {
			code = "persistence_failed"
			return q.Error
		}
		if q.RowsAffected != 1 {
			code = "persistence_failed"
			return ErrConflict
		}
		attempted = true
		return w.check(commandCtx, t)
	}
	receipt := func(part string) error {
		if len(output)+len(part) > MaxOutputBytes {
			output += part[:max(0, MaxOutputBytes-len(output))]
			code = "output_limit"
			return ErrInvalid
		}
		output += part
		next := steps + 1
		// An acknowledged remote command is evidence even if cancellation arrived
		// in the same instant. Persist it under its own bounded cleanup context.
		persist, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		err := w.s.db(persist).Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&models.Receipt{TargetID: t.ID, Step: next, Output: part}).Error; err != nil {
				return err
			}
			q := tx.Model(&models.Target{}).Where("id = ? AND status = ? AND lease_token = ?", t.ID, "running", t.LeaseToken).Updates(map[string]any{"completed_steps": next, "output": output})
			if q.Error != nil {
				return q.Error
			}
			if q.RowsAffected != 1 {
				return ErrConflict
			}
			return nil
		})
		if err != nil {
			code = "receipt_persistence_failed"
			return err
		}
		steps = next
		return nil
	}
	tail, executeErr := session.Execute(ctx, parsed, ExecutionHooks{Before: before, Receipt: receipt})
	if len(output)+len(tail) > MaxOutputBytes {
		tail = tail[:max(0, MaxOutputBytes-len(output))]
		executeErr = ErrInvalid
	}
	output += tail
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
func (w *Worker) finish(ctx context.Context, t models.Target, status, code, output string, steps int, attempted, closed bool) error {
	return w.s.db(ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&models.Target{}).Where("id = ? AND status = ? AND lease_token = ?", t.ID, "running", t.LeaseToken).Updates(map[string]any{"status": status, "error_code": code, "output": output, "completed_steps": steps, "attempted": attempted, "finished_at": time.Now().UTC()})
		if q.Error != nil {
			return q.Error
		}
		if err := devicelease.Complete(tx, devicelease.Owner{DeviceID: t.DeviceID, Token: t.LeaseToken}, closed); err != nil {
			return err
		}
		return summarize(tx, t.OrderID)
	})
}
func summarize(tx *gorm.DB, id int) error {
	// Lock the order through a write so concurrent completions cannot overwrite counts
	// computed before another target commits (works on SQLite as well as SQL engines).
	if e := tx.Model(&models.Order{}).Where("id = ?", id).Update("version", gorm.Expr("version + 1")).Error; e != nil {
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
	}
	return tx.Model(&models.Order{}).Where("id = ?", id).Updates(values).Error
}
func (w *Worker) Recover(ctx context.Context) error {
	var targets []models.Target
	if e := w.s.db(ctx).Where("status = ? AND lease_until < ? AND order_id IN (SELECT id FROM sys_delivery_order WHERE kind <> ?)", "running", time.Now().UTC(), "fabric").Find(&targets).Error; e != nil {
		return e
	}
	for _, t := range targets {
		if e := w.s.db(ctx).Transaction(func(tx *gorm.DB) error {
			q := tx.Model(&models.Target{}).Where("id = ? AND status = ? AND lease_until < ?", t.ID, "running", time.Now().UTC()).Updates(map[string]any{"status": "uncertain", "error_code": "worker_lost", "finished_at": time.Now().UTC()})
			if q.Error != nil {
				return q.Error
			}
			if q.RowsAffected == 0 {
				return nil
			}
			if e := devicelease.Complete(tx, devicelease.Owner{DeviceID: t.DeviceID, Token: t.LeaseToken}, false); e != nil {
				return e
			}
			return summarize(tx, t.OrderID)
		}); e != nil {
			return e
		}
	}
	return nil
}
