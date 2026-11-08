package service

import (
	"context"
	devicelease "go-admin/app/network/lease"
	"time"
)

// Coordinators own no transport. Negative keys retain the existing storage
// convention, while callers use positive Fabric and operation identifiers.
func (w *Worker) acquireCoordinator(ctx context.Context, fabricID int, token string, operationID int) error {
	return w.acquire(ctx, -fabricID, token, -operationID)
}
func (w *Worker) releaseCoordinator(ctx context.Context, fabricID int, token string) error {
	return w.release(ctx, -fabricID, token, true)
}
func (w *Worker) recoverCoordinators(ctx context.Context, before time.Time) error {
	return opsDB(w.s, ctx).Where("device_id < 0 AND expires_at < ?", before).Delete(&devicelease.Record{}).Error
}
