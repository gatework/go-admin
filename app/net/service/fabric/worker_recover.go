package fabric

import (
	"context"
	"time"

	"gorm.io/gorm"

	models "github.com/gatework/motor/app/net/models/fabric"
	"github.com/gatework/motor/app/net/models/lease"
)

// Recover records interrupted runs once their execution budget has elapsed.
// It never steals expired device leases or requeues a possibly attempted write.
func (w *Worker) Recover(ctx context.Context) error {
	// The sweep quarantines expired device leases and drops expired fabric locks,
	// this module's and every other module's.
	if e := lease.Sweep(opsDB(ctx, w.s)); e != nil {
		return e
	}
	now := time.Now().UTC()
	var collects []models.Collection
	if e := opsDB(ctx, w.s).Where("status = ? AND claim_until < ?", "running", now).Find(&collects).Error; e != nil {
		return e
	}
	for _, c := range collects {
		if e := w.recoverCollection(ctx, c, now); e != nil {
			return e
		}
	}
	var changes []models.Change
	if e := opsDB(ctx, w.s).Where("status = ? AND claim_until < ?", "running", now).Find(&changes).Error; e != nil {
		return e
	}
	for _, c := range changes {
		if e := w.recoverChange(ctx, c); e != nil {
			return e
		}
	}
	return nil
}

func (w *Worker) recoverCollection(ctx context.Context, c models.Collection, now time.Time) error {
	for i := range c.Targets {
		if c.Targets[i].Status == "pending" || c.Targets[i].Status == "running" {
			c.Targets[i].Status = "failed"
			c.Targets[i].ErrorCode = "worker_lost"
			if e := w.persistCollectionFailure(ctx, c, c.Targets[i], "failed", "worker_lost"); e != nil {
				return e
			}
		}
	}
	if e := opsDB(ctx, w.s).Model(&models.Collection{}).Where("id = ? AND status = ?", c.ID, "running").Updates(map[string]any{"status": "interrupted", "targets": opsJSON(c.Targets), "finished_at": now, "version": gorm.Expr("version + 1")}).Error; e != nil {
		return e
	}
	return nil
}

func (w *Worker) recoverChange(ctx context.Context, c models.Change) error {
	for i := range c.Targets {
		t := &c.Targets[i]
		if t.Status == "running" {
			t.Status = "uncertain"
			t.ErrorCode = "worker_lost"
			if t.Attempted && !t.ConfigurationVerified {
				if e := w.persistCollectionFailure(ctx, models.Collection{FabricID: c.FabricID, Mode: "all"}, models.CollectionTarget{DeviceID: t.DeviceID, Device: t.Device, Connection: t.Connection, SourceHash: t.SourceHash}, "failed", "worker_lost"); e != nil {
					return e
				}
			}
		}
		if t.Status == "pending" {
			t.Status = "cancelled"
			t.ErrorCode = "prior_target_interrupted"
		}
	}
	if e := w.finishChange(ctx, &c, "uncertain"); e != nil {
		return e
	}
	return nil
}
