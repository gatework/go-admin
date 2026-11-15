package delivery

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	models "github.com/gatework/motor/app/net/models/delivery"
	"github.com/gatework/motor/app/net/models/lease"
)

// recoverBatch bounds the expired targets one recovery pass settles.
const recoverBatch = 100

// Recover reconciles expired work without assuming unconfirmed device outcomes succeeded.
func (w *Worker) Recover(ctx context.Context) error {
	// The sweep quarantines this module's expired leases along with every other module's.
	if e := lease.Sweep(w.s.db(ctx)); e != nil {
		return e
	}
	// Only what recovery needs, never the script or output, and a bounded
	// batch: recovery runs every tick, so a backlog drains over several.
	var targets []models.Target
	if e := w.s.db(ctx).Select("id", "order_id", "device_id", "claim_token").
		Where("status = ? AND claim_until < ? AND order_id IN (SELECT id FROM sys_delivery_order WHERE kind <> ?)", "running", time.Now().UTC(), "fabric").
		Order("id").Limit(recoverBatch).Find(&targets).Error; e != nil {
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
