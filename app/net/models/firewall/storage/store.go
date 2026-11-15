// Package storage keeps immutable native evidence and transactional SQL projections.
// The caller supplies a tenant-specific DB and filesystem root and authorizes every access.
package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/gatework/firemon/flow"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/projection"
	"github.com/gatework/firemon/snapshot"
	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/firewall/types"
)

// InvalidErr reports invalid.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var InvalidErr = errors.New("invalid firewall storage argument")

//revive:enable:error-naming

// VersionErr reports version.
//
//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var VersionErr = errors.New("firewall settings version conflict")

//revive:enable:error-naming

// Store carries store state for this module.
type Store struct {
	db    *gorm.DB
	files *snapshot.Store
}

// ImportOptions carries import options state for this module.
type ImportOptions struct{ ExpectedRevision, PublicationID string }

// New provides new for this module.
func New(db *gorm.DB, root string) (*Store, error) {
	if db == nil {
		return nil, InvalidErr
	}
	files, err := snapshot.Open(root)
	if err != nil {
		return nil, err
	}
	return &Store{db: db, files: files}, nil
}

// DeviceKey provides device key for this module.
func DeviceKey(instanceID int) string { return "instance:" + strconv.Itoa(instanceID) }
func instanceID(key string) (int, error) {
	if !strings.HasPrefix(key, "instance:") {
		return 0, InvalidErr
	}
	id, err := strconv.Atoi(strings.TrimPrefix(key, "instance:"))
	if err != nil || id <= 0 {
		return 0, InvalidErr
	}
	return id, nil
}

// Import provides import for this module.
func (s *Store) Import(ctx context.Context, id int, fw *fm.Firewall, o ImportOptions) (models.Snapshot, error) {
	if fw == nil || id <= 0 || fw.Metadata().Device != DeviceKey(id) || strings.TrimSpace(o.PublicationID) == "" {
		return models.Snapshot{}, InvalidErr
	}
	var instance models.Instance
	if err := s.db.WithContext(ctx).First(&instance, id).Error; err != nil {
		return models.Snapshot{}, err
	}
	// Complete imports publish with filesystem CAS. Incomplete captures explicitly
	// archive: they never update the current head and have no outbox event.
	if !fw.Complete() {
		revision, err := s.files.Archive(ctx, fw)
		if err != nil {
			return models.Snapshot{}, err
		}
		if err = s.Apply(ctx, projection.Batch{Device: DeviceKey(id), Revision: revision, Complete: false, Firewall: fw}); err != nil {
			return models.Snapshot{}, err
		}
		return s.snapshot(ctx, id, revision)
	}
	r, err := s.files.Publish(ctx, fw, snapshot.PublishOptions{Expected: o.ExpectedRevision, PublicationID: o.PublicationID})
	if err != nil {
		return models.Snapshot{}, err
	}
	if err = s.Drain(ctx); err != nil {
		return models.Snapshot{}, err
	}
	return s.snapshot(ctx, id, r.Revision)
}

// Load provides load for this module.
func (s *Store) Load(ctx context.Context, id int, revision string) (*fm.Firewall, error) {
	if _, err := s.snapshot(ctx, id, revision); err != nil {
		return nil, err
	}
	fw, err := s.files.Load(ctx, revision)
	if err == nil && fw.Metadata().Device != DeviceKey(id) {
		return nil, snapshot.ErrFormat
	}
	return fw, err
}

// Drain provides drain for this module.
func (s *Store) Drain(ctx context.Context) error { return projection.Drain(ctx, s.files, "sql", s) }

// Publish writes native evidence and its durable outbox. The caller fences live
// device ownership; Drain runs after that SQL transaction has released its locks.
func (s *Store) Publish(ctx context.Context, fw *fm.Firewall, options snapshot.PublishOptions) (snapshot.PublishResult, error) {
	return s.files.Publish(ctx, fw, options)
}

// Apply verifies and projects an archived revision before advancing its generation.
func (s *Store) Apply(ctx context.Context, b projection.Batch) error {
	id, err := instanceID(b.Device)
	if err != nil {
		return err
	}
	if b.Firewall == nil || b.Firewall.Metadata().Device != b.Device || b.Complete != b.Firewall.Complete() {
		return snapshot.ErrFormat
	}
	// Verify exact content identity even when Apply is called directly by recovery.
	actual, err := s.files.Archive(ctx, b.Firewall)
	if err != nil {
		return err
	}
	if actual != b.Revision {
		return snapshot.ErrFormat
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var instance models.Instance
		if err := tx.First(&instance, id).Error; err != nil {
			return err
		}
		var row models.Snapshot
		err := tx.Where("instance_id = ? AND revision = ?", id, b.Revision).First(&row).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			metadata := b.Firewall.Metadata()
			row = models.Snapshot{SourceDigest: metadata.SourceDigest, SemanticDigest: b.Firewall.SemanticDigest(), ParserVersion: metadata.ParserVersion, CapturedAt: metadata.CapturedAt, Software: metadata.Software, InstanceID: id, Revision: b.Revision, Generation: b.Generation, Complete: b.Complete, Diagnostics: diagnostics(b.Firewall.Diagnostics()), ProjectionState: "building"}
			if err = tx.Create(&row).Error; err != nil {
				return err
			}
		}
		if row.ProjectionState != "ready" {
			if err = project(tx, &row, b.Firewall); err != nil {
				return err
			}
			if err = tx.Model(&row).Update("projection_state", "ready").Error; err != nil {
				return err
			}
		}
		return publishGeneration(tx, instance, row, b)
	})
}

// SaveSettings atomically replaces bindings using optimistic version checks.
func (s *Store) SaveSettings(ctx context.Context, id int, settings types.Settings) (types.Settings, error) {
	var result types.Settings
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var i models.Instance
		if err := tx.First(&i, id).Error; err != nil {
			return err
		}
		if settings.Version != i.SettingsVersion {
			return VersionErr
		}
		settings.Version++
		result = settings
		updated := tx.Model(&models.Instance{}).Where("id = ? AND settings_version = ?", id, i.SettingsVersion).Select("Settings", "SettingsVersion").Updates(&models.Instance{Settings: settings, SettingsVersion: settings.Version})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return VersionErr
		}
		if err := tx.Where("instance_id = ?", id).Delete(&models.ScopeBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("instance_id = ?", id).Delete(&models.NetworkBinding{}).Error; err != nil {
			return err
		}
		if err := tx.Where("instance_id = ?", id).Delete(&models.ZoneBinding{}).Error; err != nil {
			return err
		}
		if err := writeSettingsBindings(tx, id, settings); err != nil {
			return err
		}
		// Old coverage must never be mistaken for evidence under the new settings.
		return tx.Where("instance_id = ?", id).Delete(&models.Coverage{}).Error
	})
	return result, err
}
func diagnostics(in []fm.Diagnostic) []models.Diagnostic {
	out := make([]models.Diagnostic, 0, len(in))
	for _, d := range in {
		out = append(out, models.Diagnostic{Code: d.Code, Message: d.Message, Object: string(d.Object)})
	}
	return out
}
func key(scope, id string) string {
	sum := sha256.Sum256([]byte(scope + "\x00" + id))
	return hex.EncodeToString(sum[:])
}

// SaveCoverage provides save coverage for this module.
func (s *Store) SaveCoverage(ctx context.Context, id int, revision string, version int64, rows []types.Coverage) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var i models.Instance
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&i, id).Error; err != nil {
			return err
		}
		if i.CurrentRevision != revision || i.SettingsVersion != version {
			return VersionErr
		}
		if err := tx.Where("instance_id = ?", id).Delete(&models.Coverage{}).Error; err != nil {
			return err
		}
		for _, r := range rows {
			if r.InstanceID != id || r.Revision != revision || r.BindingsVersion != version {
				return fmt.Errorf("%w: coverage evidence", InvalidErr)
			}
			row := models.Coverage{InstanceID: id, DomainID: r.DomainID, Revision: revision, BindingsVersion: version, Zone: r.Zone, Scope: r.Scope, RoutingScope: r.RoutingScope, Network: r.Network, Protected: r.Protected, Origin: r.Origin, ObjectID: r.ObjectID, Known: r.Known}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Manifest provides manifest for this module.
func (s *Store) Manifest(ctx context.Context) (snapshot.Manifest, error) {
	return s.files.Manifest(ctx)
}

func (s *Store) snapshot(ctx context.Context, id int, revision string) (models.Snapshot, error) {
	var row models.Snapshot
	err := s.db.WithContext(ctx).Where("instance_id = ? AND revision = ?", id, revision).First(&row).Error
	return row, err
}

func bindingRange(original string) (flow.AddressRange, error) {
	value := strings.TrimSpace(original)
	if prefix, err := netip.ParsePrefix(value); err == nil {
		if interval, err := flow.PrefixRange(prefix); err == nil {
			return interval, nil
		}
	}
	if address, err := netip.ParseAddr(value); err == nil {
		interval := flow.AddressRange{First: address, Last: address}
		if interval.Valid() {
			return interval, nil
		}
	}
	return flow.AddressRange{}, fmt.Errorf("%w: invalid binding network", InvalidErr)
}

func publishGeneration(tx *gorm.DB, instance models.Instance, row models.Snapshot, b projection.Batch) error {
	// An outbox generation also represents observed incomplete live state.
	// Archive-only offline imports carry generation zero and cannot move head.
	if b.Generation > instance.Generation {
		result := tx.Model(&models.Instance{}).Where("id = ? AND generation < ?", instance.ID, b.Generation).Updates(map[string]any{"generation": b.Generation, "current_revision": b.Revision})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return snapshot.ErrConflict
		}
		if err := tx.Model(&models.Snapshot{}).Where("instance_id = ? AND current = ?", instance.ID, true).Update("current", false).Error; err != nil {
			return err
		}
		if err := tx.Model(&row).Updates(map[string]any{"current": true, "generation": b.Generation}).Error; err != nil {
			return err
		}
		if err := tx.Where("instance_id = ?", instance.ID).Delete(&models.Coverage{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func writeSettingsBindings(tx *gorm.DB, id int, settings types.Settings) error {
	for _, v := range settings.Scopes {
		if err := tx.Create(&models.ScopeBinding{InstanceID: id, RoutingScope: v.RoutingScope, DomainID: v.DomainID, Version: settings.Version}).Error; err != nil {
			return err
		}
	}
	for _, v := range settings.Zones {
		zone := models.ZoneBinding{InstanceID: id, Zone: v.Zone, Scope: v.Scope, RoutingScope: v.RoutingScope, DomainID: v.DomainID, Protected: v.Protected, Networks: v.Networks, Version: settings.Version}
		if err := tx.Create(&zone).Error; err != nil {
			return err
		}
		for position, original := range v.Networks {
			interval, err := bindingRange(original)
			if err != nil {
				return err
			}
			family := 6
			if interval.First.Is4() {
				family = 4
			}
			network := models.NetworkBinding{ZoneBindingID: zone.ID, InstanceID: id, AddressFamily: family, Start: addressKey(interval.First), End: addressKey(interval.Last), OriginalText: original, Position: position, Version: settings.Version}
			if err := tx.Create(&network).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
