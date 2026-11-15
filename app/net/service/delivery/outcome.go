package delivery

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	models "github.com/gatework/motor/app/net/models/delivery"
	"github.com/gatework/motor/app/net/models/lease"
	"github.com/gatework/motor/common/worker"
)

// persistTimeout bounds writing an outcome or a receipt after the work's own
// context has ended.
const persistTimeout = 5 * time.Second

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
	cleanup, cancel := worker.Cleanup(parent, persistTimeout)
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
