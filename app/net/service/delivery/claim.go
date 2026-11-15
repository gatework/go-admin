package delivery

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	models "github.com/gatework/motor/app/net/models/delivery"
	"github.com/gatework/motor/app/net/models/lease"
)

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
