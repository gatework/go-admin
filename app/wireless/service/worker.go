package service

import (
	"context"
	"errors"
	wireless "github.com/gatework/go-wireless"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"sync"
	"time"

	delivery "go-admin/app/delivery/models"
	"go-admin/app/network/lease"
	network "go-admin/app/network/models"
	"go-admin/app/wireless/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Worker holds no device connection until an authorized collection is queued.
type Worker struct {
	s         *Wireless
	collector Collector
	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	start     sync.Once
	stop      sync.Once
}

func NewWorker(s *Wireless, c Collector) *Worker {
	if c == nil {
		c = nativeCollector{}
	}
	return &Worker{s: s, collector: c, done: make(chan struct{})}
}

// collectionTimeout bounds one controller collection; its device lease adds lease.Margin.
const collectionTimeout = 14 * time.Minute

// Recover marks timed-out work interrupted. The lease sweep quarantines its
// expired device lease: a lease is never reused merely because time elapsed.
func (w *Worker) Recover(ctx context.Context) error {
	now := time.Now().UTC()
	if err := lease.Sweep(w.s.db(ctx)); err != nil {
		return err
	}
	return w.s.db(ctx).Model(&models.Collection{}).Where("status = ? AND lease_until < ?", "running", now).Updates(map[string]any{"status": "interrupted", "error_code": "worker_lost", "finished_at": now, "version": gorm.Expr("version + 1")}).Error
}

func (w *Worker) release(ctx context.Context, held lease.Lease, closed bool) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	outcome := lease.Closed
	if !closed {
		outcome = lease.Unconfirmed
	}
	// A lost lease was quarantined meanwhile; that quarantine stays.
	if err := lease.Release(w.s.db(ctx), held, outcome); err != nil && !errors.Is(err, lease.ErrLost) {
		return err
	}
	return nil
}

// failQuarantined fails queued collections whose controller is quarantined, so
// they show why they cannot run instead of waiting silently.
func (w *Worker) failQuarantined(ctx context.Context) error {
	now := time.Now().UTC()
	return w.s.db(ctx).Model(&models.Collection{}).Where("status = ?", "queued").Scopes(lease.Quarantined("sys_wireless_collection.controller_id")).Updates(map[string]any{"status": "failed", "error_code": "device_quarantined", "finished_at": now, "version": gorm.Expr("version + 1")}).Error
}

func (w *Worker) Start() {
	w.start.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		w.cancel = cancel
		go func() {
			defer close(w.done)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if e := w.Tick(ctx); e != nil && w.s.Log != nil {
						w.s.Log.Error("wireless worker database operation failed")
					}
				}
			}
		}()
	})
}
func (w *Worker) Close(ctx context.Context) {
	w.stop.Do(func() {
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
func (w *Worker) Tick(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if e := w.Recover(ctx); e != nil {
		return e
	}
	if e := w.failQuarantined(ctx); e != nil {
		return e
	}
	var jobs []models.Collection
	if e := w.s.db(ctx).Where("status = ?", "queued").Scopes(lease.Free("sys_wireless_collection.controller_id")).Order("id").Limit(20).Find(&jobs).Error; e != nil {
		return e
	}
	for _, job := range jobs {
		if job.ControllerID <= 0 {
			return ErrInvalid
		}
		held, e := lease.Acquire(w.s.db(ctx), job.ControllerID, 0, collectionTimeout)
		if errors.Is(e, lease.ErrBusy) || errors.Is(e, lease.ErrQuarantined) {
			continue
		}
		if e != nil {
			return e
		}
		return w.run(ctx, job, held)
	}
	return nil
}
func (w *Worker) run(ctx context.Context, job models.Collection, held lease.Lease) (result error) {
	closed := true
	defer func() {
		if e := w.release(ctx, held, closed); result == nil {
			result = e
		}
	}()
	token := held.Token
	started := time.Now().UTC()
	until := held.ExpiresAt
	q := w.s.db(ctx).Model(&models.Collection{}).Where("id = ? AND status = ?", job.ID, "queued").Updates(map[string]any{"status": "running", "started_at": started, "lease_token": token, "lease_until": until, "version": gorm.Expr("version + 1")})
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return nil
	}
	p := &actions.DataPermission{DataScope: actions.DataScopeAll}
	d, e := w.s.controller(ctx, job.ControllerID, p)
	c, ce := w.s.connection(ctx, d)
	code := ""
	var snap wireless.Snapshot
	if e != nil || ce != nil || d.Status != "active" || source(d, c) != job.SourceHash {
		code = "source_changed"
	} else {
		run, cancel := context.WithTimeout(ctx, collectionTimeout)
		snap, closed, e = w.collector.Collect(run, d, c, job)
		cancel()
		if e != nil {
			code = "collection_failed"
		}
		if !closed {
			code = "session_close_unconfirmed"
		}
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	return w.s.db(finishCtx).Transaction(func(tx *gorm.DB) error {
		scoped := *w.s
		scoped.Orm = tx
		var current models.Collection
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ? AND lease_token = ? AND lease_until > ?", job.ID, "running", token, time.Now().UTC()).First(&current).Error; e != nil {
			return e
		}
		if e := lease.Check(tx, held); e != nil {
			return e
		}
		status := "failed"
		results := []models.DimensionResult{}
		if ctx.Err() != nil {
			code = "interrupted"
			status = "interrupted"
		}
		// Fence both inventory edits and connection creation/updates. The
		// connection writer locks the same parent row, including its absent
		// default-configuration case.
		var device network.Device
		deviceErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND device_type = ?", job.ControllerID, "wireless_controller").First(&device).Error
		if deviceErr != nil && !errors.Is(deviceErr, gorm.ErrRecordNotFound) {
			return deviceErr
		}
		var stored delivery.Connection
		connectionErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("device_id = ?", job.ControllerID).First(&stored).Error
		if connectionErr != nil && !errors.Is(connectionErr, gorm.ErrRecordNotFound) {
			return connectionErr
		}
		connection, ce := scoped.connection(finishCtx, device)
		if ce != nil {
			return ce
		}
		validSource := deviceErr == nil && device.Status == "active" && source(device, connection) == job.SourceHash
		if !validSource {
			code = "source_changed"
		} else {
			if code == "" {
				expected := map[wireless.Dimension]bool{}
				for _, name := range job.Dimensions {
					expected[name] = true
				}
				for name := range snap.Dimensions() {
					if name != wireless.Collection && !expected[name] {
						code = "invalid_snapshot"
					}
				}
				for name := range expected {
					if snap.Dimension(name).Status() == wireless.NotRequested {
						code = "invalid_snapshot"
					}
				}
			}
			if code != "" {
				var e error
				snap, e = failureSnapshot(job, code, started, time.Now().UTC())
				if e != nil {
					return e
				}
			}
			e := scoped.Persist(finishCtx, job.ControllerID, job.SourceHash, snap, started, time.Now().UTC(), job.APNames...)
			if errors.Is(e, ErrInvalid) {
				code = "invalid_snapshot"
				snap, e = failureSnapshot(job, code, started, time.Now().UTC())
				if e == nil {
					e = scoped.Persist(finishCtx, job.ControllerID, job.SourceHash, snap, started, time.Now().UTC(), job.APNames...)
				}
			}
			if errors.Is(e, ErrConflict) {
				code = "superseded_observation"
			} else if e != nil {
				return e
			} else {
				available := false
				if code == "" {
					status = "succeeded"
				}
				for name, o := range snap.Dimensions() {
					if o.Status() == wireless.NotRequested {
						continue
					}
					codes := []string{}
					for _, issue := range o.Issues() {
						if v := safeCode(text(issue["code"])); v != "" {
							codes = append(codes, v)
						}
					}
					results = append(results, models.DimensionResult{Name: string(name), Status: string(o.Status()), Count: len(o.Records()), Issues: codes})
					available = available || o.Available()
					if code == "" && !o.Complete() {
						status = "partial"
					}
				}
				if code == "" && !available {
					status = "failed"
				}
			}
		}
		return tx.Model(&models.Collection{}).Where("id = ? AND status = ? AND lease_token = ?", job.ID, "running", token).Updates(map[string]any{"status": status, "error_code": code, "results": jsonText(results), "finished_at": time.Now().UTC(), "version": gorm.Expr("version + 1")}).Error
	})
}

// A failure is evidence about this attempt, not authority to retire prior facts.
func failureSnapshot(job models.Collection, code string, started, finished time.Time) (wireless.Snapshot, error) {
	observations := map[wireless.Dimension]wireless.Observation{}
	for _, name := range job.Dimensions {
		o, e := wireless.NewObservation(wireless.ObservationOptions{Name: name, Status: wireless.Failed, StartedAt: &started, CompletedAt: &finished, Issues: []wireless.Record{{"code": code}}})
		if e != nil {
			return wireless.Snapshot{}, e
		}
		observations[name] = o
	}
	return wireless.NewSnapshot(wireless.SnapshotOptions{Target: wireless.Record{"controllerId": job.ControllerID}, Dimensions: observations, ObservedAt: finished})
}
