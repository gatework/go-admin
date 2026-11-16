package inventory

import (
	"context"

	errs "github.com/gatework/motor/common/errors"
	"github.com/gatework/motor/common/worker"
)

// SyncWorker runs the inventory syncs users queue with RequestSync, one at a
// time per tenant database. Every replica runs one; the state row's claim
// decides which replica runs a given sync.
type SyncWorker struct {
	s    *Device
	loop worker.Loop
}

// NewSyncWorker constructs the worker; Run starts it.
func NewSyncWorker(s *Device) *SyncWorker {
	return &SyncWorker{s: s, loop: worker.Loop{Name: "inventory sync worker", Logger: s.Logger}}
}

// Run polls for queued syncs until ctx is cancelled.
func (w *SyncWorker) Run(ctx context.Context) error { return w.loop.Run(ctx, 1, w.Tick) }

// Tick runs the queued sync, if any. A sync that fails for a classified reason
// (Netdisco unreachable, a timeout) has already recorded that reason in the
// sync state; only an unexpected failure is returned, for the loop to log.
func (w *SyncWorker) Tick(ctx context.Context) error {
	_, _, err := w.s.RunQueuedSync(ctx)
	if err != nil && errs.KindOf(err) == errs.KindInternal {
		return err
	}
	return nil
}
