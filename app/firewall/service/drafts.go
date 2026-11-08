package service

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"go-admin/app/firewall/models"
	"go-admin/app/firewall/types"
	"gorm.io/gorm"
)

type DraftInput struct {
	RequestKey string   `json:"requestKey"`
	AnalysisID int      `json:"analysisId"`
	Title      string   `json:"title"`
	RowKeys    []string `json:"rowKeys,omitempty"`
}
type DraftEdit struct {
	Version int64  `json:"version"`
	Title   string `json:"title"`
	Notes   string `json:"notes"`
}

func (s *Firewall) EnqueueDraft(ctx context.Context, in DraftInput, p *actions.DataPermission) (models.Draft, error) {
	var out models.Draft
	if !validKey(in.RequestKey) || !validTitle(in.Title) {
		return out, ErrInvalid
	}
	a, e := s.Analysis(ctx, in.AnalysisID, p)
	if e != nil {
		return out, e
	}
	return s.enqueueDraft(ctx, in, a, false)
}
func (s *Firewall) enqueueDraft(ctx context.Context, in DraftInput, a models.Analysis, awaitAnalysis bool) (models.Draft, error) {
	var out models.Draft
	if len(in.RowKeys) == 0 {
		for _, r := range a.Input.Rows {
			in.RowKeys = append(in.RowKeys, r.Key)
		}
	}
	if len(in.RowKeys) == 0 || len(in.RowKeys) > 100 {
		return out, ErrInvalid
	}
	slices.Sort(in.RowKeys)
	in.RowKeys = slices.Compact(in.RowKeys)
	for _, key := range in.RowKeys {
		if !slices.ContainsFunc(a.Input.Rows, func(r types.NormalizedRow) bool { return r.Key == key }) {
			return out, ErrInvalid
		}
	}
	digest := hash(in)
	e := s.db(ctx).Where("analysis_id = ? AND request_key = ?", a.ID, in.RequestKey).First(&out).Error
	if e == nil {
		if out.PayloadHash != digest || out.ActorID != s.Actor {
			return out, ErrConflict
		}
		out.Stale = a.Stale
		return out, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, e
	}
	// An accepted idempotent retry retains its original receipt even when its
	// evidence is now stale. Only a new draft needs a current, completed basis.
	if a.Stale || !awaitAnalysis && a.Status != "succeeded" && a.Status != "partial" {
		return out, ErrConflict
	}
	out = models.Draft{RequestID: a.RequestID, AnalysisID: a.ID, ActorID: s.Actor, RequestKey: in.RequestKey, PayloadHash: digest, Title: strings.TrimSpace(in.Title), Version: 1, Status: "queued", RowKeys: in.RowKeys, Result: types.DraftResult{Items: []types.DraftItem{}, At: a.At}}
	if e = s.db(ctx).Create(&out).Error; e != nil {
		var prior models.Draft
		if x := s.db(ctx).Where("analysis_id = ? AND request_key = ?", a.ID, in.RequestKey).First(&prior).Error; x == nil && prior.PayloadHash == digest && prior.ActorID == s.Actor {
			prior.Stale = a.Stale
			return prior, nil
		}
		return out, ErrConflict
	}
	return out, nil
}
func (s *Firewall) Draft(ctx context.Context, id int, p *actions.DataPermission) (models.Draft, error) {
	var out models.Draft
	if id <= 0 {
		return out, ErrInvalid
	}
	if e := s.db(ctx).First(&out, id).Error; e != nil {
		return out, safeError(e)
	}
	if out.ActorID != s.Actor {
		return models.Draft{}, ErrNotFound
	}
	a, e := s.Analysis(ctx, out.AnalysisID, p)
	if e != nil {
		return models.Draft{}, e
	}
	out.Stale = a.Stale
	return out, nil
}
func (s *Firewall) Drafts(ctx context.Context, page *Page, p *actions.DataPermission) ([]models.Draft, int64, error) {
	out := []models.Draft{}
	var count int64
	if e := page.Validate(); e != nil {
		return out, 0, e
	}
	q := s.db(ctx).Model(&models.Draft{}).Where("actor_id = ? AND request_id IN (?)", s.Actor, s.requests(ctx, p).Select("id"))
	q = keyword(q, "title", page.Keyword)
	if e := q.Count(&count).Error; e != nil {
		return out, 0, e
	}
	if e := pageQuery(q, *page).Order("id DESC").Find(&out).Error; e != nil {
		return out, 0, e
	}
	for i := range out {
		d, e := s.Draft(ctx, out[i].ID, p)
		if e != nil {
			return nil, 0, e
		}
		out[i] = d
	}
	return out, count, nil
}
func (s *Firewall) EditDraft(ctx context.Context, id int, in DraftEdit, p *actions.DataPermission) (models.Draft, error) {
	out, e := s.Draft(ctx, id, p)
	if e != nil {
		return out, e
	}
	if !validTitle(in.Title) || len(in.Notes) > 8192 {
		return out, ErrInvalid
	}
	if out.Version != in.Version {
		return out, ErrConflict
	}
	q := s.db(ctx).Model(&models.Draft{}).Where("id = ? AND version = ?", id, in.Version).Updates(map[string]any{"title": strings.TrimSpace(in.Title), "notes": in.Notes, "version": gorm.Expr("version + 1")})
	if q.Error != nil {
		return out, q.Error
	}
	if q.RowsAffected != 1 {
		return out, ErrConflict
	}
	return s.Draft(ctx, id, p)
}
func (s *Firewall) ReanalyzeDraft(ctx context.Context, id int, in JobInput, p *actions.DataPermission) (models.Draft, error) {
	d, e := s.Draft(ctx, id, p)
	if e != nil {
		return models.Draft{}, e
	}
	if in.Version != d.Version || !validKey(in.RequestKey) {
		return models.Draft{}, ErrConflict
	}
	r, e := s.Request(ctx, d.RequestID, p)
	if e != nil {
		return models.Draft{}, e
	}
	a, e := s.EnqueueAnalysis(ctx, r.ID, JobInput{RequestKey: "draft-" + hash(struct {
		ID  int
		Key string
	}{id, in.RequestKey}), Version: r.Version}, p)
	if e != nil {
		return models.Draft{}, e
	}
	return s.enqueueDraft(ctx, DraftInput{RequestKey: in.RequestKey, AnalysisID: a.ID, Title: d.Title, RowKeys: d.RowKeys}, a, true)
}
