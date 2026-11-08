package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	native "github.com/gatework/firemon/device"
	"github.com/gatework/firemon/operations"
	"github.com/gatework/firemon/snapshot"
	"github.com/gatework/go-net-connector/connector"
	"github.com/google/uuid"
	delivery "go-admin/app/delivery/models"
	"go-admin/app/firewall/models"
	"go-admin/app/firewall/storage"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CollectionDialer func(context.Context, CollectionSource) (native.Reader, error)
type CollectionWorker struct {
	s      *Firewall
	dial   CollectionDialer
	once   sync.Once
	cancel context.CancelFunc
	done   chan struct{}
}

func NewCollectionWorker(s *Firewall, dial CollectionDialer) *CollectionWorker {
	if dial == nil {
		dial = openFirewallReader
	}
	return &CollectionWorker{s: s, dial: dial, done: make(chan struct{})}
}
func (w *CollectionWorker) Start() {
	w.once.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		w.cancel = cancel
		go func() {
			defer close(w.done)
			var workers sync.WaitGroup
			for n := 0; n < CollectionConcurrency; n++ {
				workers.Add(1)
				go func() {
					defer workers.Done()
					ticker := time.NewTicker(time.Second)
					defer ticker.Stop()
					for {
						select {
						case <-ctx.Done():
							return
						case <-ticker.C:
							if err := w.Tick(ctx); err != nil && ctx.Err() == nil && w.s.Log != nil {
								w.s.Log.Error("firewall collection worker failed")
							}
						}
					}
				}()
			}
			workers.Wait()
		}()
	})
}
func (w *CollectionWorker) Close(ctx context.Context) {
	if w.cancel == nil {
		return
	}
	w.cancel()
	select {
	case <-w.done:
	case <-ctx.Done():
	}
}
func publicationID(t models.CollectionTarget) string {
	return "collection:" + strconv.Itoa(t.ID) + ":" + t.SourceHash
}
func (w *CollectionWorker) Tick(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	st, err := w.s.store()
	if err != nil {
		return err
	}
	if err = st.Drain(ctx); err != nil {
		return err
	}
	if err = w.recover(ctx, st); err != nil {
		return err
	}
	var queued []models.CollectionTarget
	busy := w.s.db(ctx).Model(&delivery.DeviceLease{}).Select("device_id").Where("quarantined = ? AND expires_at > ?", false, time.Now().UTC())
	if err = w.s.db(ctx).Model(&models.CollectionTarget{}).Where("status = ? AND device_id IN (?)", "queued", busy).Update("stage", "waiting_device").Error; err != nil {
		return err
	}
	// Filter physical leases before limiting candidates, so repeated requests
	// for busy devices cannot starve unrelated devices with available capacity.
	if err = w.s.db(ctx).Where("status = ? AND device_id NOT IN (?)", "queued", busy).Order("id").Limit(MaxCollectionTargets).Find(&queued).Error; err != nil {
		return err
	}
	for _, t := range queued {
		t, source, claimed, err := w.claim(ctx, t)
		if err != nil {
			return err
		}
		if !claimed {
			continue
		}
		if t.Status != "running" {
			continue
		}
		return w.run(ctx, t, source, st)
	}
	return nil
}
func (w *CollectionWorker) claim(ctx context.Context, t models.CollectionTarget) (models.CollectionTarget, CollectionSource, bool, error) {
	var source CollectionSource
	claimed := false
	err := w.s.db(ctx).Transaction(func(tx *gorm.DB) error {
		// One permanent domain row serializes the tenant-wide admission count across
		// processes. Physical exclusion remains shared with delivery and wireless.
		var gate models.Domain
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Order("id").First(&gate).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&models.CollectionTarget{}).Where("status = ?", "running").Count(&count).Error; err != nil {
			return err
		}
		if count >= CollectionConcurrency {
			return nil
		}
		var batch models.CollectionBatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&batch, t.BatchID).Error; err != nil {
			return err
		}
		var current models.CollectionTarget
		if err := tx.First(&current, t.ID).Error; err != nil {
			return err
		}
		t = current
		if t.Status != "queued" {
			return nil
		}
		reason := ""
		if batch.CancelRequested {
			reason = "cancelled"
		} else {
			var err error
			source, err = collectionSource(ctx, tx, t.InstanceID, true)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				reason = "source_changed"
			} else if err != nil {
				return err
			}
			if reason == "" && source.fingerprint() != t.SourceHash {
				reason = "source_changed"
			}
			if reason == "" {
				reason = source.reason()
			}
		}
		now := time.Now().UTC()
		if reason == "" {
			var lease delivery.DeviceLease
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lease, t.DeviceID).Error
			if err == nil {
				if lease.Quarantined || !lease.ExpiresAt.After(now) {
					if err = tx.Model(&lease).Update("quarantined", true).Error; err != nil {
						return err
					}
					reason = "device_quarantined"
				} else {
					return tx.Model(&t).Update("stage", "waiting_device").Error
				}
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if reason != "" {
			t.Status = "failed"
			if reason == "cancelled" {
				t.Status = "cancelled"
			}
			t.Stage = "preflight"
			t.ErrorCode = reason
			t.FinishedAt = &now
			if err := tx.Model(&t).Select("Status", "Stage", "ErrorCode", "FinishedAt").Updates(&t).Error; err != nil {
				return err
			}
			claimed = true
			return refreshCollection(tx, t.BatchID)
		}
		token := uuid.NewString()
		until := now.Add(CollectionTimeout + time.Minute)
		lease := delivery.DeviceLease{DeviceID: t.DeviceID, Token: token, ExpiresAt: until}
		q := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&lease)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return nil
		}
		t.Status = "running"
		t.Stage = "connecting"
		t.LeaseToken = token
		t.LeaseUntil = &until
		t.StartedAt = &now
		t.SessionClosed = false
		if err := tx.Model(&t).Select("Status", "Stage", "LeaseToken", "LeaseUntil", "StartedAt", "SessionClosed").Updates(&t).Error; err != nil {
			return err
		}
		claimed = true
		return refreshCollection(tx, t.BatchID)
	})
	return t, source, claimed, err
}
func collectionOwned(db *gorm.DB, t models.CollectionTarget) *gorm.DB {
	return db.Model(&models.CollectionTarget{}).Where("id = ? AND status = ? AND lease_token = ? AND lease_until > ?", t.ID, "running", t.LeaseToken, time.Now().UTC())
}
func (w *CollectionWorker) stage(ctx context.Context, t models.CollectionTarget, stage string) error {
	q := collectionOwned(w.s.db(ctx), t).Update("stage", stage)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return ErrConflict
	}
	return nil
}
func (w *CollectionWorker) run(parent context.Context, t models.CollectionTarget, source CollectionSource, st *storage.Store) error {
	ctx, cancel := context.WithTimeout(parent, CollectionTimeout)
	defer cancel()
	watchDone := make(chan struct{})
	defer func() { cancel(); <-watchDone }()
	go func() {
		defer close(watchDone)
		timer := time.NewTicker(200 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				var b models.CollectionBatch
				if err := w.s.db(ctx).Select("id", "cancel_requested").First(&b, t.BatchID).Error; err != nil || b.CancelRequested {
					cancel()
					return
				}
			}
		}
	}()
	session := &collectionSession{worker: w, target: t, source: source}
	var batch models.CollectionBatch
	if err := w.s.db(ctx).First(&batch, t.BatchID).Error; err != nil {
		return err
	}
	var result operations.CollectResult
	var opErr error
	if batch.CancelRequested {
		cancel()
	}
	if batch.Kind == "connection_test" {
		var reader native.Reader
		reader, opErr = session.OpenReadOnly(ctx, storage.DeviceKey(t.InstanceID))
		if reader != nil {
			opErr = errors.Join(opErr, reader.Close())
		}
	} else {
		collector, err := operations.NewCollector(operations.CollectorOptions{Store: &collectionStore{worker: w, target: t, Store: st}, Sessions: session, Parser: collectionParser{worker: w, target: t}, Timeout: CollectionTimeout})
		if err != nil {
			return err
		}
		result, opErr = collector.Collect(ctx, operations.CollectRequest{Device: storage.DeviceKey(t.InstanceID), Expected: t.ExpectedRevision, PublicationID: publicationID(t)})
	}
	finishCtx, finish := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer finish()
	if batch.Kind == "capture" && result.Revision == "" && opErr != nil {
		// A filesystem publication can commit before its surrounding SQL commit
		// acknowledgement fails. Reconcile before recording a terminal result.
		manifest, err := st.Manifest(finishCtx)
		if err != nil {
			return err
		}
		var found bool
		result, found, err = publishedCollection(manifest, t)
		if err != nil {
			return err
		}
		if !found {
			result = operations.CollectResult{}
		}
	}
	// Recovery can replay this outbox without another device connection.
	if result.Revision != "" {
		if err := w.stage(finishCtx, t, "projecting"); err != nil {
			return err
		}
		if err := st.Drain(finishCtx); err != nil {
			return err
		}
	}
	code := collectionError(opErr)
	closed := session.closed
	if session.attempted && !closed {
		code = "session_close_unconfirmed"
	}
	status := "succeeded"
	if result.Revision != "" && (!result.Complete || code != "") {
		status = "partial"
	} else if code != "" {
		status = "failed"
		if code == "cancelled" {
			status = "cancelled"
		}
	}
	// No open attempt (e.g. stale expected head) has no transport to retain.
	if !session.attempted {
		closed = true
	}
	return w.finish(finishCtx, t, status, code, result, closed)
}
func (w *CollectionWorker) finish(ctx context.Context, t models.CollectionTarget, status, code string, result operations.CollectResult, closed bool) error {
	return w.s.db(ctx).Transaction(func(tx *gorm.DB) error {
		var batch models.CollectionBatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&batch, t.BatchID).Error; err != nil {
			return err
		}
		q := collectionOwned(tx, t).Updates(map[string]any{"status": status, "stage": "finished", "error_code": code, "revision": result.Revision, "complete": result.Complete, "session_closed": closed, "finished_at": time.Now().UTC(), "lease_until": nil})
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return ErrConflict
		}
		if err := releaseCollectionLease(tx, t, closed); err != nil {
			return err
		}
		return refreshCollection(tx, t.BatchID)
	})
}
func releaseCollectionLease(tx *gorm.DB, t models.CollectionTarget, closed bool) error {
	q := tx.Where("device_id = ? AND token = ?", t.DeviceID, t.LeaseToken)
	if closed {
		return q.Delete(&delivery.DeviceLease{}).Error
	}
	return q.Model(&delivery.DeviceLease{}).Update("quarantined", true).Error
}

func publishedCollection(manifest snapshot.Manifest, target models.CollectionTarget) (operations.CollectResult, bool, error) {
	publication, found := manifest.Publications[publicationID(target)]
	if !found {
		return operations.CollectResult{}, false, nil
	}
	if publication.Device != storage.DeviceKey(target.InstanceID) || publication.Reservation != "" || publication.KeepReservation {
		return operations.CollectResult{}, true, ErrConflict
	}
	var previous, current snapshot.Event
	for _, event := range manifest.Events {
		if event.Device != publication.Device {
			continue
		}
		if event.Generation < publication.Generation && event.Generation > previous.Generation {
			previous = event
		}
		if event.Generation == publication.Generation {
			current = event
		}
	}
	if current.Revision != publication.Revision || previous.Revision != target.ExpectedRevision {
		return operations.CollectResult{}, true, ErrConflict
	}
	return operations.CollectResult{Revision: publication.Revision, Generation: publication.Generation, Complete: current.Complete}, true, nil
}
func (w *CollectionWorker) recover(ctx context.Context, st *storage.Store) error {
	var expired []models.CollectionTarget
	if err := w.s.db(ctx).Where("status = ? AND lease_until < ?", "running", time.Now().UTC()).Find(&expired).Error; err != nil {
		return err
	}
	if len(expired) == 0 {
		return nil
	}
	manifest, err := st.Manifest(ctx)
	if err != nil {
		return err
	}
	for _, t := range expired {
		err = w.s.db(ctx).Transaction(func(tx *gorm.DB) error {
			var b models.CollectionBatch
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&b, t.BatchID).Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&t, t.ID).Error; err != nil {
				return err
			}
			if t.Status != "running" || t.LeaseUntil == nil || t.LeaseUntil.After(time.Now().UTC()) {
				return nil
			}
			status, code, revision, complete := "failed", "interrupted", "", false
			published, found, err := publishedCollection(manifest, t)
			if err != nil {
				return err
			}
			if found {
				revision, complete = published.Revision, published.Complete
				status = "partial"
				if complete {
					status = "succeeded"
				}
				code = ""
				if !complete {
					code = "capture_incomplete"
				}
			}
			if !t.SessionClosed {
				code = "session_close_unconfirmed"
				if revision != "" {
					status = "partial"
				}
			}
			if err := tx.Model(&t).Updates(map[string]any{"status": status, "stage": "finished", "error_code": code, "revision": revision, "complete": complete, "finished_at": time.Now().UTC(), "lease_until": nil}).Error; err != nil {
				return err
			}
			if err := releaseCollectionLease(tx, t, t.SessionClosed); err != nil {
				return err
			}
			return refreshCollection(tx, t.BatchID)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
func collectionError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, errCollectionSource):
		return "source_changed"
	case errors.Is(err, connector.ErrAuthentication):
		return "authentication_failed"
	case errors.Is(err, connector.ErrOutputLimit), errors.Is(err, connector.ErrResponseLimit):
		return "output_limit"
	case errors.Is(err, snapshot.ErrConflict), errors.Is(err, ErrConflict):
		return "version_conflict"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, operations.ErrIncomplete):
		return "capture_incomplete"
	case errors.Is(err, connector.ErrConnection):
		return "connection_failed"
	default:
		return "collection_failed"
	}
}
