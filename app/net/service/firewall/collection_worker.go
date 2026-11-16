package firewall

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/gatework/motor/app/net/models/lease"
	"github.com/gatework/motor/common/worker"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	native "github.com/gatework/firemon/device"
	"github.com/gatework/firemon/operations"
	"github.com/gatework/firemon/snapshot"
	"github.com/gatework/go-net-connector/connector"
	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/firewall/storage"
	"github.com/gatework/motor/app/net/models/txlock"
	"github.com/gatework/motor/app/net/service/deviceaccess"
)

// CollectionDialer carries collection dialer state for this module.
type CollectionDialer func(context.Context, CollectionSource) (native.Reader, error)

// CollectionWorker carries collection worker state for this module.
type CollectionWorker struct {
	loop worker.Loop
	s    *Firewall
	dial CollectionDialer
}

// NewCollectionWorker provides new collection worker for this module.
func NewCollectionWorker(s *Firewall, dial CollectionDialer) *CollectionWorker {
	if dial == nil {
		dial = nativeDialer(s.Access)
	}
	return &CollectionWorker{s: s, dial: dial, loop: worker.Loop{Name: "firewall collection worker", Logger: s.Logger}}
}

// Run provides run for this module.
func (w *CollectionWorker) Run(ctx context.Context) error {
	return w.loop.Run(ctx, CollectionConcurrency, w.Tick)
}

func publicationID(t models.CollectionTarget) string {
	return "collection:" + strconv.Itoa(t.ID) + ":" + t.SourceHash
}

// Tick provides tick for this module.
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
	const device = "sys_firewall_collection_target.device_id"
	if err = w.s.db(ctx).Model(&models.CollectionTarget{}).Where("status = ?", "queued").Scopes(lease.Held(device)).Update("stage", "waiting_device").Error; err != nil {
		return err
	}
	// Filter held devices before limiting candidates, so repeated requests for
	// busy devices cannot starve unrelated devices with available capacity.
	// Quarantined devices stay candidates: claim fails them visibly.
	var quarantined []models.CollectionTarget
	if err = w.s.db(ctx).Where("status = ?", "queued").Scopes(lease.Quarantined(device)).Order("id").Limit(MaxCollectionTargets).Find(&quarantined).Error; err != nil {
		return err
	}
	if err = w.s.db(ctx).Where("status = ?", "queued").Scopes(lease.Free(device)).Order("id").Limit(MaxCollectionTargets).Find(&queued).Error; err != nil {
		return err
	}
	queued = append(quarantined, queued...)
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
		// Serialize the tenant-wide admission count across processes. Physical
		// exclusion per device is the device lease, shared with every module.
		if err := txlock.Hold(tx, txlock.FirewallCollectionAdmission); err != nil {
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
		var reason string
		var err error
		source, reason, err = claimCollectionSource(ctx, w.s.Access, tx, t, batch.CancelRequested)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		var held lease.Lease
		if reason == "" {
			var err error
			held, err = lease.Acquire(tx, t.DeviceID, lease.Holder{Kind: lease.FirewallCollection, ID: t.ID}, CollectionTimeout)
			switch {
			case errors.Is(err, lease.BusyErr):
				return tx.Model(&t).Update("stage", "waiting_device").Error
			case errors.Is(err, lease.QuarantinedErr):
				reason = "device_quarantined"
			case err != nil:
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
		token, until := held.Token, held.ExpiresAt
		t.Status = "running"
		t.Stage = "connecting"
		t.ClaimToken = token
		t.ClaimUntil = &until
		t.StartedAt = &now
		t.SessionClosed = false
		if err := tx.Model(&t).Select("Status", "Stage", "ClaimToken", "ClaimUntil", "StartedAt", "SessionClosed").Updates(&t).Error; err != nil {
			return err
		}
		claimed = true
		return refreshCollection(tx, t.BatchID)
	})
	return t, source, claimed, err
}
func collectionOwned(db *gorm.DB, t models.CollectionTarget) *gorm.DB {
	return db.Model(&models.CollectionTarget{}).Where("id = ? AND status = ? AND claim_token = ? AND claim_until > ?", t.ID, "running", t.ClaimToken, time.Now().UTC())
}
func (w *CollectionWorker) stage(ctx context.Context, t models.CollectionTarget, stage string) error {
	q := collectionOwned(w.s.db(ctx), t).Update("stage", stage)
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return ConflictErr
	}
	return nil
}
func (w *CollectionWorker) run(parent context.Context, t models.CollectionTarget, source CollectionSource, st *storage.Store) error {
	ctx, cancel := context.WithTimeout(parent, CollectionTimeout)
	defer cancel()
	defer worker.WatchCancel(ctx, cancel, func(query context.Context) (bool, error) {
		var b models.CollectionBatch
		err := w.s.db(query).Select("id", "cancel_requested").First(&b, t.BatchID).Error
		return b.CancelRequested, err
	})()
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
	finishCtx, finish := worker.Cleanup(ctx, 15*time.Second)
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
	status, code, closed := collectionOutcome(result, opErr, session)
	return w.finish(finishCtx, t, status, code, result, closed)
}
func (w *CollectionWorker) finish(ctx context.Context, t models.CollectionTarget, status, code string, result operations.CollectResult, closed bool) error {
	return w.s.db(ctx).Transaction(func(tx *gorm.DB) error {
		var batch models.CollectionBatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&batch, t.BatchID).Error; err != nil {
			return err
		}
		q := collectionOwned(tx, t).Updates(map[string]any{"status": status, "stage": "finished", "error_code": code, "revision": result.Revision, "complete": result.Complete, "session_closed": closed, "finished_at": time.Now().UTC(), "claim_until": nil})
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return ConflictErr
		}
		if err := releaseCollectionLease(tx, t, closed); err != nil {
			return err
		}
		return refreshCollection(tx, t.BatchID)
	})
}

// releaseCollectionLease ends the target's device lease. A lost lease was
// quarantined meanwhile; that quarantine stays, even after a recorded clean close.
func releaseCollectionLease(tx *gorm.DB, t models.CollectionTarget, closed bool) error {
	outcome := lease.Closed
	if !closed {
		outcome = lease.Unconfirmed
	}
	if err := lease.Release(tx, lease.Lease{DeviceID: t.DeviceID, Token: t.ClaimToken}, outcome); err != nil && !errors.Is(err, lease.LostErr) {
		return err
	}
	return nil
}

func publishedCollection(manifest snapshot.Manifest, target models.CollectionTarget) (operations.CollectResult, bool, error) {
	publication, found := manifest.Publications[publicationID(target)]
	if !found {
		return operations.CollectResult{}, false, nil
	}
	if publication.Device != storage.DeviceKey(target.InstanceID) || publication.Reservation != "" || publication.KeepReservation {
		return operations.CollectResult{}, true, ConflictErr
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
		return operations.CollectResult{}, true, ConflictErr
	}
	return operations.CollectResult{Revision: publication.Revision, Generation: publication.Generation, Complete: current.Complete}, true, nil
}
func (w *CollectionWorker) recover(ctx context.Context, st *storage.Store) error {
	if err := lease.Sweep(w.s.db(ctx)); err != nil {
		return err
	}
	var expired []models.CollectionTarget
	if err := w.s.db(ctx).Where("status = ? AND claim_until < ?", "running", time.Now().UTC()).Find(&expired).Error; err != nil {
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
		err = w.recoverTarget(ctx, t, manifest)
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
	case errors.Is(err, collectionSourceErr):
		return "source_changed"
	case errors.Is(err, connector.ErrAuthentication):
		return "authentication_failed"
	case errors.Is(err, connector.ErrOutputLimit), errors.Is(err, connector.ErrResponseLimit):
		return "output_limit"
	case errors.Is(err, snapshot.ErrConflict), errors.Is(err, ConflictErr):
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

func claimCollectionSource(ctx context.Context, access deviceaccess.Config, tx *gorm.DB, t models.CollectionTarget, cancelled bool) (CollectionSource, string, error) {
	var source CollectionSource
	reason := ""
	if cancelled {
		reason = "cancelled"
	} else {
		var err error
		source, err = collectionSource(ctx, access, tx, t.InstanceID, true)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			reason = "source_changed"
		} else if err != nil {
			return source, "", err
		}
		if reason == "" && source.fingerprint() != t.SourceHash {
			reason = "source_changed"
		}
		if reason == "" {
			reason = source.reason()
		}
	}
	return source, reason, nil
}

func collectionOutcome(result operations.CollectResult, opErr error, session *collectionSession) (string, string, bool) {
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
	return status, code, closed
}

func (w *CollectionWorker) recoverTarget(ctx context.Context, t models.CollectionTarget, manifest snapshot.Manifest) error {
	return w.s.db(ctx).Transaction(func(tx *gorm.DB) error {
		var b models.CollectionBatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&b, t.BatchID).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&t, t.ID).Error; err != nil {
			return err
		}
		if t.Status != "running" || t.ClaimUntil == nil || t.ClaimUntil.After(time.Now().UTC()) {
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
		if err := tx.Model(&t).Updates(map[string]any{"status": status, "stage": "finished", "error_code": code, "revision": revision, "complete": complete, "finished_at": time.Now().UTC(), "claim_until": nil}).Error; err != nil {
			return err
		}
		if err := releaseCollectionLease(tx, t, t.SessionClosed); err != nil {
			return err
		}
		return refreshCollection(tx, t.BatchID)
	})
}
