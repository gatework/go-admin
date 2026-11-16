package firewall

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/gatework/firemon/snapshot"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/connection"
	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/firewall/storage"
	"github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	"github.com/gatework/motor/app/net/service/firewall/dto"
)

// MaxCollectionTargets bounds collection targets processing.
const MaxCollectionTargets = 100

// CollectionConcurrency bounds collection concurrency processing.
const CollectionConcurrency = 2

// CollectionTimeout bounds collection timeout processing.
const CollectionTimeout = 5 * time.Minute

// CollectionSource carries collection source state for this module.
type CollectionSource struct {
	Instance   models.Instance
	Device     inventory.Device
	Connection ConnectionStatus
}

var vdomContext = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,79}$`)

func (s CollectionSource) reason() string {
	if !s.Connection.Ready {
		return s.Connection.Reason
	}
	if s.Instance.Vendor != s.Connection.Vendor {
		return "profile_mismatch"
	}
	if s.Instance.Vendor == "fortinet" {
		if !vdomContext.MatchString(s.Instance.NativeContext) {
			return "context_required"
		}
	} else if s.Instance.NativeContext != "" {
		return "context_unsupported"
	}
	return ""
}
func (s CollectionSource) fingerprint() string {
	// Pin only authoritative non-secret source facts. Credentials are resolved at
	// use time and never become a persisted hash or snapshot attribute.
	return hash([]any{s.Instance.ID, s.Instance.Version, s.Instance.Vendor, s.Instance.NativeContext,
		s.Device.ID, s.Device.Version, s.Device.CreateBy, s.Device.IP, s.Device.Vendor, s.Device.OS, s.Device.HardwareModel, s.Device.Status, s.Device.DeviceType, s.Device.DeviceRole,
		s.Connection.Version, s.Connection.Profile, s.Connection.Protocol, s.Connection.Port})
}
func collectionSource(ctx context.Context, access deviceaccess.Config, db *gorm.DB, id int, lock bool) (CollectionSource, error) {
	var out CollectionSource
	q := db.WithContext(ctx)
	// Device is the shared connection-settings lock. Keep this order everywhere.
	var i models.Instance
	if err := q.First(&i, id).Error; err != nil {
		return out, err
	}
	dq := q
	if lock {
		dq = dq.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := dq.First(&out.Device, i.DeviceID).Error; err != nil {
		return out, err
	}
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.First(&out.Instance, id).Error; err != nil {
		return out, err
	}
	c, err := connection.Load(ctx, db, out.Device.ID)
	if err != nil {
		return out, err
	}
	out.Connection = effectiveFirewallConnection(access, out.Device, c)
	return out, nil
}
func batchScope(db *gorm.DB, actor int, p *actions.DataPermission) *gorm.DB {
	hidden := db.Model(&models.CollectionTarget{}).Select("batch_id").Where("device_id NOT IN (?)", deviceScope(db, p).Select("id"))
	return db.Model(&models.CollectionBatch{}).Where("actor_id = ? AND id NOT IN (?)", actor, hidden)
}

// Collection provides collection for this module.
func (s *Firewall) Collection(ctx context.Context, id int, p *actions.DataPermission) (models.CollectionBatch, error) {
	var b models.CollectionBatch
	if id <= 0 || s.Actor <= 0 {
		return b, InvalidErr
	}
	err := batchScope(s.db(ctx), s.Actor, p).Preload("Targets", func(db *gorm.DB) *gorm.DB { return db.Order("id") }).First(&b, id).Error
	if err == nil {
		revisions := []string{}
		for _, target := range b.Targets {
			if target.Revision != "" {
				revisions = append(revisions, target.Revision)
			}
		}
		if len(revisions) > 0 {
			var snapshots []models.Snapshot
			if err = s.db(ctx).Select("id", "instance_id", "revision").Where("revision IN ?", revisions).Find(&snapshots).Error; err == nil {
				for n := range b.Targets {
					for _, snapshot := range snapshots {
						if snapshot.InstanceID == b.Targets[n].InstanceID && snapshot.Revision == b.Targets[n].Revision {
							b.Targets[n].SnapshotID = snapshot.ID
						}
					}
				}
			}
		}
	}
	return b, safeError(err)
}

// Collections provides collections for this module.
func (s *Firewall) Collections(ctx context.Context, page *dto.Page, p *actions.DataPermission) ([]models.CollectionBatch, int64, error) {
	out := []models.CollectionBatch{}
	var count int64
	if err := page.Validate(); err != nil {
		return out, 0, err
	}
	q := batchScope(s.db(ctx), s.Actor, p)
	if err := q.Count(&count).Error; err != nil {
		return out, 0, err
	}
	err := pageQuery(q, *page).Preload("Targets", func(db *gorm.DB) *gorm.DB { return db.Order("id") }).Order("id DESC").Find(&out).Error
	return out, count, err
}

// EnqueueCollection records an idempotent batch and its pinned collection targets.
func (s *Firewall) EnqueueCollection(ctx context.Context, in dto.CollectionInput, p *actions.DataPermission) (models.CollectionBatch, error) {
	var b models.CollectionBatch
	if s.Actor <= 0 || !validKey(in.RequestKey) || (in.Kind != "capture" && in.Kind != "connection_test") || len(in.InstanceIDs) == 0 || len(in.InstanceIDs) > MaxCollectionTargets {
		return b, InvalidErr
	}
	ids := slices.Clone(in.InstanceIDs)
	slices.Sort(ids)
	for n, id := range ids {
		if id <= 0 || n > 0 && ids[n-1] == id {
			return b, InvalidErr
		}
	}
	payload := hash([]any{in.Kind, ids})
	err := s.db(ctx).Where("actor_id = ? AND request_key = ?", s.Actor, in.RequestKey).First(&b).Error
	if err == nil {
		if b.PayloadHash != payload {
			return b, ConflictErr
		}
		return s.Collection(ctx, b.ID, p)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return b, err
	}
	st, err := s.store()
	if err != nil {
		return b, err
	}
	if err = st.Drain(ctx); err != nil {
		return b, err
	}
	manifest, err := st.Manifest(ctx)
	if err != nil {
		return b, err
	}
	b = models.CollectionBatch{ActorID: s.Actor, RequestKey: in.RequestKey, PayloadHash: payload, Kind: in.Kind, Status: "queued", Targets: []models.CollectionTarget{}}
	err = s.db(ctx).Transaction(func(tx *gorm.DB) error {
		for _, id := range ids {
			t, err := queuedCollectionTarget(ctx, s.Access, tx, id, p, manifest)
			if err != nil {
				return err
			}
			b.Targets = append(b.Targets, t)
		}
		if err := tx.Create(&b).Error; err != nil {
			return err
		}
		return refreshCollection(tx, b.ID)
	})
	if err != nil {
		// A concurrent identical submission may have won the unique key.
		var prior models.CollectionBatch
		if e := s.db(ctx).Where("actor_id = ? AND request_key = ?", s.Actor, in.RequestKey).First(&prior).Error; e == nil {
			if prior.PayloadHash != payload {
				return models.CollectionBatch{}, ConflictErr
			}
			return s.Collection(ctx, prior.ID, p)
		}
		return models.CollectionBatch{}, err
	}
	return s.Collection(ctx, b.ID, p)
}
func refreshCollection(tx *gorm.DB, id int) error {
	var b models.CollectionBatch
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&b, id).Error; err != nil {
		return err
	}
	var rows []models.CollectionTarget
	if err := tx.Where("batch_id = ?", id).Find(&rows).Error; err != nil {
		return err
	}
	pending, running, success, partial, cancelled := 0, 0, 0, 0, 0
	for _, t := range rows {
		switch t.Status {
		case "queued":
			pending++
		case "running":
			running++
		case "succeeded":
			success++
		case "partial":
			partial++
		case "cancelled":
			cancelled++
		}
	}
	status := "failed"
	var finished *time.Time
	switch {
	case running > 0:
		status = "running"
	case pending > 0:
		status = "queued"
	case success == len(rows):
		status = "succeeded"
	case cancelled == len(rows):
		status = "cancelled"
	case success > 0 || partial > 0 || cancelled > 0:
		status = "partial"
	}
	if pending+running == 0 {
		now := time.Now().UTC()
		finished = &now
	}
	return tx.Model(&b).Updates(map[string]any{"status": status, "finished_at": finished}).Error
}

// CancelCollection provides cancel collection for this module.
func (s *Firewall) CancelCollection(ctx context.Context, id int, p *actions.DataPermission) (models.CollectionBatch, error) {
	b, err := s.Collection(ctx, id, p)
	if err != nil {
		return b, err
	}
	err = s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&b, id).Error; err != nil {
			return err
		}
		if err := tx.Model(&b).Update("cancel_requested", true).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.CollectionTarget{}).Where("batch_id = ? AND status = ?", id, "queued").Updates(map[string]any{"status": "cancelled", "stage": "finished", "error_code": "cancelled", "finished_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		return refreshCollection(tx, id)
	})
	if err != nil {
		return b, err
	}
	return s.Collection(ctx, id, p)
}

func queuedCollectionTarget(ctx context.Context, access deviceaccess.Config, tx *gorm.DB, id int, p *actions.DataPermission, manifest snapshot.Manifest) (models.CollectionTarget, error) {
	var i models.Instance
	if err := scopedInstances(tx, p).First(&i, id).Error; err != nil {
		return models.CollectionTarget{}, safeError(err)
	}
	source, err := collectionSource(ctx, access, tx, id, true)
	if err != nil {
		return models.CollectionTarget{}, safeError(err)
	}
	t := models.CollectionTarget{InstanceID: id, DeviceID: i.DeviceID, Name: source.Device.Name, IP: source.Device.IP, Vendor: i.Vendor, NativeContext: i.NativeContext, SourceHash: source.fingerprint(), ExpectedRevision: i.CurrentRevision, InstanceVersion: i.Version, ConnectionVersion: source.Connection.Version, Status: "queued", Stage: "queued", SessionClosed: true}
	t.ErrorCode = source.reason()
	if t.ErrorCode == "" && manifest.Devices[storage.DeviceKey(id)].Revision != i.CurrentRevision {
		t.ErrorCode = "evidence_unavailable"
	}
	if t.ErrorCode != "" {
		now := time.Now().UTC()
		t.Status = "failed"
		t.Stage = "preflight"
		t.FinishedAt = &now
	}
	return t, nil
}
