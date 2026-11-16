package fabric

import (
	"context"
	"errors"
	"time"

	"github.com/gatework/motor-core/logger"
	models "github.com/gatework/motor/app/net/models/fabric"
	"github.com/gatework/motor/app/net/models/lease"
	ds "github.com/gatework/motor/app/net/service/delivery"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	"github.com/gatework/motor/common/worker"
)

// Worker advances queued operations while holding the required device leases.
type Worker struct {
	loop      worker.Loop
	s         *Fabric
	transport Transport

	lastSchedule time.Time
}

// NewWorker constructs an application-owned background worker.
func NewWorker(s *Fabric, t Transport) *Worker {
	if t == nil {
		t = nativeTransport{access: deviceaccess.FromEnvironment()}
	}
	// A background worker has no caller to return an error to; its log line is the
	// only trace of a failed tick, so it must never depend on the host's wiring.
	if s.Logger == nil {
		s.Logger = logger.NewHelper(logger.Default())
	}
	return &Worker{s: s, transport: t, loop: worker.Loop{Name: "fabric worker", Logger: s.Logger}}
}

// Run owns the worker lifecycle until cancellation and waits for its pollers.
func (w *Worker) Run(ctx context.Context) error {
	return w.loop.Run(ctx, 1, w.Tick)
}

// Tick advances due work while preserving lease ownership and durable outcomes.
func (w *Worker) Tick(ctx context.Context) error {
	if e := w.Recover(ctx); e != nil {
		return e
	}
	if time.Since(w.lastSchedule) >= time.Minute {
		if e := w.schedule(ctx, time.Now().UTC()); e != nil {
			return e
		}
		w.lastSchedule = time.Now().UTC()
	}
	var jobs []models.Collection
	if e := w.s.db(ctx).Where("status = ?", "queued").Order("id").Limit(20).Find(&jobs).Error; e != nil {
		return e
	}
	for _, job := range jobs {
		lock, e := lease.LockFabric(w.s.db(ctx), job.FabricID, lease.Holder{Kind: lease.FabricCollection, ID: job.ID}, runTimeout)
		if errors.Is(e, lease.BusyErr) {
			continue
		} else if e != nil {
			return e
		}
		e = w.runCollection(ctx, job)
		release := w.unlock(ctx, lock)
		if e != nil {
			return e
		}
		if release != nil {
			return release
		}
		break
	}
	if !ds.FromEnvironment().Enabled {
		return nil
	}
	var changes []models.Change
	if e := w.s.db(ctx).Where("status = ?", "queued").Order("id").Limit(20).Find(&changes).Error; e != nil {
		return e
	}
	for _, c := range changes {
		lock, e := lease.LockFabric(w.s.db(ctx), c.FabricID, lease.Holder{Kind: lease.FabricChange, ID: c.ID}, runTimeout)
		if errors.Is(e, lease.BusyErr) {
			continue
		} else if e != nil {
			return e
		}
		e = w.runChange(ctx, c)
		release := w.unlock(ctx, lock)
		if e != nil {
			return e
		}
		if release != nil {
			return release
		}
		break
	}
	return nil
}
