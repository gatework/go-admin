package service

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gatework/firemon/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"github.com/google/uuid"
	"go-admin/app/firewall/engine"
	"go-admin/app/firewall/models"
	"go-admin/app/firewall/storage"
	"go-admin/app/firewall/types"
	network "go-admin/app/network/models"
	"gorm.io/gorm"
)

func (s *Firewall) Settings(ctx context.Context, id int, p *actions.DataPermission) (types.Settings, error) {
	i, e := s.Instance(ctx, id, p)
	if e != nil {
		return types.Settings{}, e
	}
	settings := i.Settings
	settings.Version = i.SettingsVersion
	if settings.Zones == nil {
		settings.Zones = []types.Binding{}
	}
	if settings.Scopes == nil {
		settings.Scopes = []types.ScopeBinding{}
	}
	return settings, nil
}
func (s *Firewall) SaveSettings(ctx context.Context, id int, in types.Settings, p *actions.DataPermission) (types.Settings, error) {
	i, e := s.Instance(ctx, id, p)
	if e != nil {
		return in, e
	}
	if in.Version != i.SettingsVersion {
		return in, ErrConflict
	}
	if len(in.Zones) > 512 || len(in.Scopes) > 128 {
		return in, ErrInvalid
	}
	store, e := s.store()
	if e != nil {
		return in, e
	}
	if i.CurrentRevision == "" {
		return in, ErrUnavailable
	}
	fw, e := store.Load(ctx, id, i.CurrentRevision)
	if e != nil {
		return in, e
	}
	seen := map[string]bool{}
	for _, b := range in.Scopes {
		if len(b.RoutingScope) > 255 || seen["scope:"+b.RoutingScope] {
			return in, ErrInvalid
		}
		seen["scope:"+b.RoutingScope] = true
		if e = s.domain(ctx, b.DomainID); e != nil {
			return in, e
		}
	}
	for _, b := range in.Zones {
		// Zero is inheritance: the engine resolves the VRF mapping first, then
		// the instance default. Persist it so later mapping edits still apply.
		domainID := b.DomainID
		if domainID == 0 {
			domainID = i.DomainID
			for _, scope := range in.Scopes {
				if scope.RoutingScope == b.RoutingScope {
					domainID = scope.DomainID
				}
			}
		}
		if e = s.domain(ctx, domainID); e != nil {
			return in, e
		}
		key := "zone:" + hash([]string{b.Zone, b.Scope, b.RoutingScope})
		if seen[key] || b.Zone == "" || len(b.Networks) > 2048 {
			return in, ErrInvalid
		}
		seen[key] = true
		found := 0
		for _, zone := range fw.Zones() {
			if (string(zone.ID()) == b.Zone || zone.Name() == b.Zone) && zone.Scope() == b.Scope {
				found++
			}
		}
		if found != 1 {
			return in, ErrInvalid
		}
		for _, v := range b.Networks {
			if _, e := netip.ParsePrefix(strings.TrimSpace(v)); e != nil {
				if _, e = netip.ParseAddr(strings.TrimSpace(v)); e != nil {
					return in, ErrInvalid
				}
			}
		}
	}
	// Validate the derived workload before committing manual state. Otherwise a
	// bounded-engine rejection would return an error after advancing the version.
	_, e = engine.Coverage(ctx, types.Target{InstanceID: id, Name: i.Name, Revision: i.CurrentRevision, DomainID: i.DomainID, Mode: i.Mode, BindingsVersion: i.SettingsVersion + 1, Settings: in, Firewall: fw})
	if e != nil {
		return in, e
	}
	out, e := store.SaveSettings(ctx, id, in)
	if errors.Is(e, storage.ErrVersion) {
		return out, ErrConflict
	}
	if e != nil {
		return out, e
	}
	// Rebuild is idempotent; concurrent publication/settings edits reject old coverage.
	_, e = s.Coverage(ctx, id, p)
	if errors.Is(e, storage.ErrVersion) {
		return out, ErrConflict
	}
	return out, e
}
func (s *Firewall) Coverage(ctx context.Context, id int, p *actions.DataPermission) ([]types.Coverage, error) {
	i, e := s.Instance(ctx, id, p)
	if e != nil {
		return nil, e
	}
	if i.CurrentRevision == "" {
		return []types.Coverage{}, nil
	}
	store, e := s.store()
	if e != nil {
		return nil, e
	}
	manifest, e := store.Manifest(ctx)
	if e != nil {
		return nil, e
	}
	if manifest.Devices[storage.DeviceKey(id)].Revision != i.CurrentRevision {
		return nil, ErrUnavailable
	}
	fw, e := store.Load(ctx, id, i.CurrentRevision)
	if e != nil {
		return nil, e
	}
	rows, e := engine.Coverage(ctx, types.Target{InstanceID: id, Name: i.Name, Revision: i.CurrentRevision, DomainID: i.DomainID, Mode: i.Mode, BindingsVersion: i.SettingsVersion, Settings: i.Settings, Firewall: fw})
	if e != nil {
		return nil, e
	}
	e = store.SaveCoverage(ctx, id, i.CurrentRevision, i.SettingsVersion, rows)
	return rows, e
}
func (s *Firewall) Snapshot(ctx context.Context, id int, p *actions.DataPermission) (models.Snapshot, error) {
	var out models.Snapshot
	if id <= 0 {
		return out, ErrInvalid
	}
	if e := s.db(ctx).First(&out, id).Error; e != nil {
		return out, safeError(e)
	}
	if _, e := s.Instance(ctx, out.InstanceID, p); e != nil {
		return models.Snapshot{}, e
	}
	return out, nil
}
func (s *Firewall) Snapshots(ctx context.Context, id int, page *Page, p *actions.DataPermission) ([]models.Snapshot, int64, error) {
	out := []models.Snapshot{}
	var count int64
	if e := page.Validate(); e != nil {
		return out, 0, e
	}
	if _, e := s.Instance(ctx, id, p); e != nil {
		return out, 0, e
	}
	q := s.db(ctx).Model(&models.Snapshot{}).Where("instance_id = ?", id)
	if e := q.Count(&count).Error; e != nil {
		return out, 0, e
	}
	e := pageQuery(q, *page).Order("id DESC").Find(&out).Error
	return out, count, e
}
func (s *Firewall) Objects(ctx context.Context, id int, kind string, f storage.Filter, p *actions.DataPermission) (storage.Page, error) {
	if _, e := s.Snapshot(ctx, id, p); e != nil {
		return storage.Page{}, e
	}
	st, e := s.store()
	if e != nil {
		return storage.Page{}, e
	}
	return st.Objects(ctx, id, kind, f)
}
func (s *Firewall) Object(ctx context.Context, id int, kind string, objectID int, p *actions.DataPermission) (storage.Object, error) {
	if _, e := s.Snapshot(ctx, id, p); e != nil {
		return storage.Object{}, e
	}
	st, e := s.store()
	if e != nil {
		return storage.Object{}, e
	}
	return st.Object(ctx, id, kind, objectID)
}
func (s *Firewall) References(ctx context.Context, id int, kind string, objectID int, p *actions.DataPermission) (storage.References, error) {
	if _, e := s.Snapshot(ctx, id, p); e != nil {
		return storage.References{}, e
	}
	st, e := s.store()
	if e != nil {
		return storage.References{}, e
	}
	return st.References(ctx, id, kind, objectID)
}

type ImportInput struct {
	RequestKey       string     `json:"requestKey"`
	ExpectedRevision string     `json:"expectedRevision"`
	Text             string     `json:"text"`
	Filename         string     `json:"filename"`
	Complete         bool       `json:"complete"`
	CapturedAt       *time.Time `json:"capturedAt,omitempty"`
}

func (s *Firewall) EnqueueImport(ctx context.Context, id int, in ImportInput, p *actions.DataPermission) (models.Import, error) {
	var out models.Import
	if !validKey(in.RequestKey) || len(in.Text) == 0 || len(in.Text) > config.DefaultMaxBytes || !utf8.ValidString(in.Text) || strings.TrimSpace(in.Text) == "" || len(in.Filename) > 255 {
		return out, ErrInvalid
	}
	i, e := s.Instance(ctx, id, p)
	if e != nil {
		return out, e
	}
	if in.CapturedAt != nil && in.CapturedAt.After(time.Now().Add(5*time.Minute)) {
		return out, ErrInvalid
	}
	digest := hash(in)
	e = s.db(ctx).Where("instance_id = ? AND request_key = ?", id, in.RequestKey).First(&out).Error
	if e == nil {
		if out.PayloadHash != digest || out.ActorID != s.Actor {
			return out, ErrConflict
		}
		return out, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, e
	}
	if i.CurrentRevision != in.ExpectedRevision {
		return out, ErrConflict
	}
	root, e := s.root()
	if e != nil {
		return out, e
	}
	dir := filepath.Join(root, "incoming")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return out, e
	}
	name := uuid.NewString() + ".cfg"
	path := filepath.Join(dir, name)
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return out, e
	}
	_, e = f.WriteString(in.Text)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		_ = os.Remove(path)
		return out, e
	}
	var device network.Device
	if e = deviceScope(s.db(ctx), p).First(&device, i.DeviceID).Error; e != nil {
		_ = os.Remove(path)
		return out, safeError(e)
	}
	out = models.Import{InstanceID: id, ActorID: s.Actor, RequestKey: in.RequestKey, ExpectedRevision: in.ExpectedRevision, PayloadHash: digest, SourcePath: name, Filename: filepath.Base(in.Filename), Complete: in.Complete, Software: device.OSVersion, Status: "queued"}
	if in.CapturedAt != nil {
		out.CapturedAt = in.CapturedAt.UTC()
	}
	if e = s.db(ctx).Create(&out).Error; e != nil {
		_ = os.Remove(path)
		var previous models.Import
		if x := s.db(ctx).Where("instance_id = ? AND request_key = ?", id, in.RequestKey).First(&previous).Error; x == nil && previous.ActorID == s.Actor && previous.PayloadHash == digest {
			return previous, nil
		}
		return out, ErrConflict
	}
	return out, nil
}
func (s *Firewall) Import(ctx context.Context, id int, p *actions.DataPermission) (models.Import, error) {
	var out models.Import
	if id <= 0 {
		return out, ErrInvalid
	}
	if e := s.db(ctx).First(&out, id).Error; e != nil {
		return out, safeError(e)
	}
	if _, e := s.Instance(ctx, out.InstanceID, p); e != nil {
		return models.Import{}, e
	}
	return out, nil
}
func (s *Firewall) Imports(ctx context.Context, id int, page *Page, p *actions.DataPermission) ([]models.Import, int64, error) {
	out := []models.Import{}
	var count int64
	if e := page.Validate(); e != nil {
		return out, 0, e
	}
	if _, e := s.Instance(ctx, id, p); e != nil {
		return out, 0, e
	}
	q := s.db(ctx).Model(&models.Import{}).Where("instance_id = ?", id)
	if e := q.Count(&count).Error; e != nil {
		return out, 0, e
	}
	e := pageQuery(q, *page).Order("id DESC").Find(&out).Error
	return out, count, e
}
func (s *Firewall) CancelImport(ctx context.Context, id int, p *actions.DataPermission) error {
	if _, e := s.Import(ctx, id, p); e != nil {
		return e
	}
	q := s.db(ctx).Model(&models.Import{}).Where("id = ? AND status = ?", id, "queued").Updates(map[string]any{"status": "cancelled", "finished_at": time.Now().UTC()})
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return ErrConflict
	}
	return nil
}
