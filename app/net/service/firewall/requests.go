package firewall

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/contract/actions"
	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/firewall/storage"
	"github.com/gatework/motor/app/net/models/firewall/types"
	"github.com/gatework/motor/app/net/service/firewall/engine"
)

// RequestInput carries request input state for this module.
type RequestInput struct {
	Version  int64       `json:"version"`
	Title    string      `json:"title"`
	DomainID int         `json:"domainId"`
	Input    types.Input `json:"input"`
}

// Request provides request for this module.
func (s *Firewall) Request(ctx context.Context, id int, p *actions.DataPermission) (models.Request, error) {
	var out models.Request
	if id <= 0 {
		return out, InvalidErr
	}
	e := s.requests(ctx, p).First(&out, id).Error
	return out, safeError(e)
}

// Requests provides requests for this module.
func (s *Firewall) Requests(ctx context.Context, page *Page, p *actions.DataPermission) ([]models.Request, int64, error) {
	out := []models.Request{}
	var count int64
	if e := page.Validate(); e != nil {
		return out, 0, e
	}
	q := keyword(s.requests(ctx, p), "title", page.Keyword)
	if page.DomainID > 0 {
		q = q.Where("domain_id = ?", page.DomainID)
	}
	if e := q.Count(&count).Error; e != nil {
		return out, 0, e
	}
	e := pageQuery(q, *page).Order("id DESC").Find(&out).Error
	return out, count, e
}

// SaveRequest provides save request for this module.
func (s *Firewall) SaveRequest(ctx context.Context, id int, in RequestInput, p *actions.DataPermission) (models.Request, error) {
	var out models.Request
	if s.Actor <= 0 || !validTitle(in.Title) {
		return out, InvalidErr
	}
	if e := s.domain(ctx, in.DomainID); e != nil {
		return out, e
	}
	normal, e := engine.Normalize(in.Input)
	if e != nil {
		return out, InvalidErr
	}
	if len(normal.Rows) == 0 {
		return out, InvalidErr
	}
	in.Input.DefaultProtocol = normal.DefaultProtocol
	e = s.db(ctx).Transaction(func(tx *gorm.DB) error {
		child := *s
		child.DB = tx
		if id > 0 {
			var err error
			out, err = child.Request(ctx, id, p)
			if err != nil {
				return err
			}
			if out.Version != in.Version {
				return ConflictErr
			}
		}
		out.Title = strings.TrimSpace(in.Title)
		out.DomainID = in.DomainID
		out.Input = in.Input
		out.Normalized = normal
		out.ActorID = s.Actor
		out.Version++
		if id == 0 {
			if err := tx.Create(&out).Error; err != nil {
				return err
			}
		} else {
			q := tx.Model(&models.Request{}).Where("id = ? AND version = ?", id, in.Version).Select("title", "domain_id", "input", "normalized", "version", "updated_at").Updates(&out)
			if q.Error != nil {
				return q.Error
			}
			if q.RowsAffected != 1 {
				return ConflictErr
			}
		}
		for i, row := range normal.Rows {
			r := models.RequestRow{RequestID: out.ID, RequestVersion: out.Version, Key: row.Key, Line: row.Line, Position: i, Raw: row.InputRow, Normalized: row}
			if err := tx.Create(&r).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return out, e
}

// EnqueueAnalysis provides enqueue analysis for this module.
func (s *Firewall) EnqueueAnalysis(ctx context.Context, id int, in JobInput, p *actions.DataPermission) (models.Analysis, error) {
	var out models.Analysis
	if !validKey(in.RequestKey) {
		return out, InvalidErr
	}
	r, e := s.Request(ctx, id, p)
	if e != nil {
		return out, e
	}
	digest := hash(struct {
		Request int
		Version int64
	}{id, in.Version})
	e = s.db(ctx).Where("request_id = ? AND request_key = ?", id, in.RequestKey).First(&out).Error
	if e == nil {
		if out.PayloadHash != digest || out.ActorID != s.Actor {
			return out, ConflictErr
		}
		return s.Analysis(ctx, out.ID, p)
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, e
	}
	if r.Version != in.Version {
		return out, ConflictErr
	}
	targets, e := s.pinTargets(ctx, r.DomainID, p)
	if e != nil {
		return out, e
	}
	out = models.Analysis{RequestID: id, RequestVersion: r.Version, ActorID: s.Actor, DomainID: r.DomainID, RequestKey: in.RequestKey, PayloadHash: digest, Status: "queued", At: time.Now().UTC(), Input: r.Normalized, Targets: targets, Result: types.AnalysisResult{Items: []types.AnalysisItem{}, Issues: []types.Notice{}}}
	e = s.db(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.Request
		if err := tx.First(&current, id).Error; err != nil {
			return err
		}
		if current.Version != r.Version {
			return ConflictErr
		}
		if err := tx.Create(&out).Error; err != nil {
			return err
		}
		for _, ref := range targets {
			if err := tx.Create(&models.AnalysisTarget{AnalysisID: out.ID, InstanceID: ref.InstanceID}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if e != nil {
		var existing models.Analysis
		if x := s.db(ctx).Where("request_id = ? AND request_key = ?", id, in.RequestKey).First(&existing).Error; x == nil && existing.PayloadHash == digest && existing.ActorID == s.Actor {
			return s.Analysis(ctx, existing.ID, p)
		}
	}
	return out, e
}

// Analysis provides analysis for this module.
func (s *Firewall) Analysis(ctx context.Context, id int, p *actions.DataPermission) (models.Analysis, error) {
	var out models.Analysis
	if id <= 0 {
		return out, InvalidErr
	}
	if e := s.db(ctx).First(&out, id).Error; e != nil {
		return out, safeError(e)
	}
	if _, e := s.Request(ctx, out.RequestID, p); e != nil {
		return models.Analysis{}, e
	}
	if e := s.authorizeTargets(ctx, out.Targets, p); e != nil {
		return models.Analysis{}, e
	}
	stale, e := s.stale(ctx, out, p)
	out.Stale = stale
	return out, e
}

// Analyses provides analyses for this module.
func (s *Firewall) Analyses(ctx context.Context, id int, page *Page, p *actions.DataPermission) ([]models.Analysis, int64, error) {
	out := []models.Analysis{}
	var count int64
	if e := page.Validate(); e != nil {
		return out, 0, e
	}
	if _, e := s.Request(ctx, id, p); e != nil {
		return out, 0, e
	}
	q := s.db(ctx).Model(&models.Analysis{}).Where("request_id = ?", id)
	if e := q.Count(&count).Error; e != nil {
		return out, 0, e
	}
	if e := pageQuery(q, *page).Order("id DESC").Find(&out).Error; e != nil {
		return out, 0, e
	}
	for i := range out {
		v, e := s.Analysis(ctx, out[i].ID, p)
		if e != nil {
			return nil, 0, e
		}
		out[i] = v
	}
	return out, count, nil
}

// CancelAnalysis cancels a queued or running analysis and returns it as it
// now stands.
func (s *Firewall) CancelAnalysis(ctx context.Context, id int, p *actions.DataPermission) (models.Analysis, error) {
	if _, e := s.Analysis(ctx, id, p); e != nil {
		return models.Analysis{}, e
	}
	q := s.db(ctx).Model(&models.Analysis{}).Where("id = ? AND status IN ?", id, []string{"queued", "running"}).Updates(map[string]any{"status": "cancelled", "finished_at": time.Now().UTC(), "claim_token": ""})
	if q.Error != nil {
		return models.Analysis{}, q.Error
	}
	if q.RowsAffected != 1 {
		return models.Analysis{}, ConflictErr
	}
	return s.Analysis(ctx, id, p)
}

// JobInput carries job input state for this module.
type JobInput struct {
	RequestKey string `json:"requestKey"`
	Version    int64  `json:"version"`
}

func (s *Firewall) requests(ctx context.Context, p *actions.DataPermission) *gorm.DB {
	visible := scopedInstances(s.db(ctx), p).Select("id")
	blocked := s.db(ctx).Table("sys_firewall_analysis_target AS target").Select("1").Joins("JOIN sys_firewall_analysis analysis ON analysis.id = target.analysis_id").Where("analysis.request_id = sys_firewall_request.id").Where("target.instance_id NOT IN (?)", visible)
	return s.db(ctx).Model(&models.Request{}).Where("actor_id = ?", s.Actor).Where("NOT EXISTS (?)", blocked)
}

func (s *Firewall) pinTargets(ctx context.Context, domainID int, p *actions.DataPermission) ([]types.TargetRef, error) {
	rows := []models.Instance{}
	q := inDomain(scopedInstances(s.db(ctx), p), s.db(ctx), domainID)
	if e := q.Order("id").Find(&rows).Error; e != nil {
		return nil, e
	}
	store, e := s.store()
	if e != nil {
		return nil, e
	}
	manifest, e := store.Manifest(ctx)
	if e != nil {
		return nil, e
	}
	out := []types.TargetRef{}
	for _, row := range rows {
		head := manifest.Devices[storage.DeviceKey(row.ID)]
		if head.Revision != row.CurrentRevision {
			return nil, UnavailableErr
		}
		if _, reserved := manifest.Reservations[storage.DeviceKey(row.ID)]; reserved {
			return nil, UnavailableErr
		}
		out = append(out, types.TargetRef{InstanceID: row.ID, Name: row.Name, Revision: head.Revision, DomainID: row.DomainID, Mode: row.Mode, BindingsVersion: row.SettingsVersion, Settings: row.Settings})
	}
	return out, nil
}

func (s *Firewall) authorizeTargets(ctx context.Context, targets []types.TargetRef, p *actions.DataPermission) error {
	for _, ref := range targets {
		if _, e := s.Instance(ctx, ref.InstanceID, p); e != nil {
			return e
		}
	}
	return nil
}

func (s *Firewall) stale(ctx context.Context, a models.Analysis, p *actions.DataPermission) (bool, error) {
	var r models.Request
	if e := s.db(ctx).First(&r, a.RequestID).Error; e != nil {
		return true, e
	}
	if r.Version != a.RequestVersion {
		return true, nil
	}
	currentTargets, err := s.pinTargets(ctx, a.DomainID, p)
	if errors.Is(err, UnavailableErr) {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	if hash(currentTargets) != hash(a.Targets) {
		return true, nil
	}
	store, e := s.store()
	if e != nil {
		return true, e
	}
	manifest, e := store.Manifest(ctx)
	if e != nil {
		return true, e
	}
	for _, ref := range a.Targets {
		var current models.Instance
		if e := s.db(ctx).First(&current, ref.InstanceID).Error; e != nil {
			return true, e
		}
		head := manifest.Devices[storage.DeviceKey(ref.InstanceID)]
		if current.SettingsVersion != ref.BindingsVersion || current.CurrentRevision != ref.Revision || head.Revision != ref.Revision {
			return true, nil
		}
	}
	return false, nil
}
