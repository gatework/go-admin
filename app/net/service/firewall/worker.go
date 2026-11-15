package firewall

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/gatework/motor/common/worker"

	"github.com/google/uuid"
	"gorm.io/gorm"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/config"
	"github.com/gatework/firemon/snapshot"
	"github.com/gatework/firemon/vendors"
	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/firewall/storage"
	"github.com/gatework/motor/app/net/models/firewall/types"
	network "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/service/firewall/engine"
)

// Worker performs only local parsing, projection and analysis. It has no device transport.
type Worker struct {
	loop worker.Loop
	s    *Firewall
}

// NewWorker provides new worker for this module.
func NewWorker(s *Firewall) *Worker {
	return &Worker{s: s, loop: worker.Loop{Name: "firewall offline worker", Logger: s.Logger}}
}

// Run provides run for this module.
func (w *Worker) Run(ctx context.Context) error {
	return w.loop.Run(ctx, 2, w.Tick)
}

func runnable(db *gorm.DB) *gorm.DB {
	return db.Where("status = ? OR (status = ? AND claim_until < ?)", "queued", "running", time.Now().UTC())
}

// Tick provides tick for this module.
func (w *Worker) Tick(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	store, e := w.s.store()
	if e != nil {
		return e
	}
	if e = store.Drain(ctx); e != nil {
		return e
	}
	var imports []models.Import
	if e = runnable(w.s.db(ctx)).Order("id").Limit(2).Find(&imports).Error; e != nil {
		return e
	}
	for _, job := range imports {
		token, claimed, e := w.claim(ctx, &models.Import{}, job.ID)
		if e != nil {
			return e
		}
		if claimed {
			return w.importJob(ctx, job, token)
		}
	}
	var analyses []models.Analysis
	if e = runnable(w.s.db(ctx)).Order("id").Limit(2).Find(&analyses).Error; e != nil {
		return e
	}
	for _, job := range analyses {
		token, claimed, e := w.claim(ctx, &models.Analysis{}, job.ID)
		if e != nil {
			return e
		}
		if claimed {
			return w.analysisJob(ctx, job, token)
		}
	}
	var drafts []models.Draft
	if e = runnable(w.s.db(ctx)).Where("analysis_id IN (?)", w.s.db(ctx).Model(&models.Analysis{}).Select("id").Where("status NOT IN ?", []string{"queued", "running"})).Order("id").Limit(2).Find(&drafts).Error; e != nil {
		return e
	}
	for _, job := range drafts {
		token, claimed, e := w.claim(ctx, &models.Draft{}, job.ID)
		if e != nil {
			return e
		}
		if claimed {
			return w.draftJob(ctx, job, token)
		}
	}
	return nil
}

func (w *Worker) claim(ctx context.Context, model any, id int) (string, bool, error) {
	token := uuid.NewString()
	q := runnable(w.s.db(ctx).Model(model).Where("id = ?", id)).Updates(map[string]any{"status": "running", "claim_token": token, "claim_until": time.Now().UTC().Add(6 * time.Minute)})
	return token, q.RowsAffected == 1, q.Error
}
func owned(db *gorm.DB, id int, token string) *gorm.DB {
	return db.Where("id = ? AND status = ? AND claim_token = ? AND claim_until > ?", id, "running", token, time.Now().UTC())
}
func errorCode(e error) string {
	switch {
	case e == nil:
		return ""
	case errors.Is(e, snapshot.ErrConflict), errors.Is(e, storage.VersionErr):
		return "version_conflict"
	case errors.Is(e, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(e, context.Canceled):
		return "cancelled"
	case errors.Is(e, engine.BudgetErr):
		return "budget_exceeded"
	case errors.Is(e, config.ErrSource):
		return "invalid_source"
	default:
		return "operation_failed"
	}
}
func (w *Worker) importJob(parent context.Context, job models.Import, token string) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	status, revision, code := "failed", "", ""
	run := func() error {
		var instance models.Instance
		if e := w.s.db(ctx).First(&instance, job.InstanceID).Error; e != nil {
			return e
		}
		var device network.Device
		if e := w.s.db(ctx).First(&device, instance.DeviceID).Error; e != nil {
			return e
		}
		root, e := w.s.root()
		if e != nil {
			return e
		}
		if job.SourcePath == "" || filepath.Base(job.SourcePath) != job.SourcePath {
			return InvalidErr
		}
		file, e := os.Open(filepath.Join(root, "incoming", job.SourcePath))
		if e != nil {
			return e
		}
		data, e := io.ReadAll(io.LimitReader(file, config.DefaultMaxBytes+1))
		closeErr := file.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		if len(data) > config.DefaultMaxBytes {
			return InvalidErr
		}
		fw, e := (vendors.Parser{}).Parse(ctx, config.Source{Text: string(data), Complete: job.Complete}, fm.Metadata{Device: storage.DeviceKey(instance.ID), Vendor: instance.Vendor, Context: instance.NativeContext, CapturedAt: job.CapturedAt, Software: job.Software})
		if e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		st, e := w.s.store()
		if e != nil {
			return e
		}
		snap, e := st.Import(ctx, instance.ID, fw, storage.ImportOptions{ExpectedRevision: job.ExpectedRevision, PublicationID: "import:" + strconv.Itoa(job.ID) + ":" + job.PayloadHash})
		if e != nil {
			return e
		}
		revision = snap.Revision
		status = "succeeded"
		if !snap.Complete {
			status = "partial"
		}
		return nil
	}
	if e := run(); e != nil {
		code = errorCode(e)
	}
	finish, finishCancel := worker.Cleanup(parent, 10*time.Second)
	defer finishCancel()
	q := owned(w.s.db(finish).Model(&models.Import{}), job.ID, token).Updates(map[string]any{"status": status, "revision": revision, "error_code": code, "finished_at": time.Now().UTC(), "claim_token": ""})
	return q.Error
}
func (w *Worker) loadTargets(ctx context.Context, refs []types.TargetRef) ([]types.Target, error) {
	st, e := w.s.store()
	if e != nil {
		return nil, e
	}
	out := []types.Target{}
	for _, ref := range refs {
		t := types.Target{InstanceID: ref.InstanceID, Name: ref.Name, Revision: ref.Revision, DomainID: ref.DomainID, Mode: ref.Mode, BindingsVersion: ref.BindingsVersion, Settings: ref.Settings}
		if ref.Revision != "" {
			t.Firewall, e = st.Load(ctx, ref.InstanceID, ref.Revision)
			if e != nil {
				return nil, e
			}
		}
		out = append(out, t)
	}
	return out, nil
}
func (w *Worker) analysisJob(parent context.Context, job models.Analysis, token string) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	// Stop computation promptly when the persisted cancel transition wins.
	defer worker.WatchCancel(ctx, cancel, func(query context.Context) (bool, error) {
		var count int64
		err := owned(w.s.db(query).Model(&models.Analysis{}), job.ID, token).Count(&count).Error
		return count == 0, err
	})()
	result := types.AnalysisResult{Items: []types.AnalysisItem{}, Issues: []types.Notice{}}
	targets, e := w.loadTargets(ctx, job.Targets)
	if e == nil {
		result, e = engine.Analyze(ctx, job.Input, targets, job.DomainID, job.At)
	}
	status := "succeeded"
	if !result.Complete {
		status = "partial"
	}
	if e != nil {
		status = "failed"
		result = types.AnalysisResult{Items: []types.AnalysisItem{}, Issues: []types.Notice{{Code: errorCode(e)}}}
	}
	finish, finishCancel := worker.Cleanup(parent, 10*time.Second)
	defer finishCancel()
	return w.s.db(finish).Transaction(func(tx *gorm.DB) error {
		out := models.Analysis{Status: status, Result: result, ErrorCode: errorCode(e), FinishedAt: time.Now().UTC(), ClaimToken: ""}
		q := owned(tx.Model(&models.Analysis{}), job.ID, token).Select("status", "result", "error_code", "finished_at", "claim_token", "updated_at").Updates(&out)
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected == 0 {
			return nil
		}
		if err := tx.Where("analysis_id = ?", job.ID).Delete(&models.AnalysisResult{}).Error; err != nil {
			return err
		}
		for i, item := range result.Items {
			if err := tx.Create(&models.AnalysisResult{AnalysisID: job.ID, Position: i, Item: item}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (w *Worker) draftJob(parent context.Context, job models.Draft, token string) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Minute)
	defer cancel()
	result := types.DraftResult{Items: []types.DraftItem{}}
	var a models.Analysis
	e := w.s.db(ctx).First(&a, job.AnalysisID).Error
	if e == nil && (a.Status != "succeeded" && a.Status != "partial") {
		e = UnavailableErr
	}
	if e == nil {
		var targets []types.Target
		targets, e = w.loadTargets(ctx, a.Targets)
		if e == nil {
			in := a.Input
			in.Rows = slices.DeleteFunc(slices.Clone(in.Rows), func(r types.NormalizedRow) bool { return !slices.Contains(job.RowKeys, r.Key) })
			result, e = engine.Design(ctx, in, targets, a.DomainID, a.At)
		}
	}
	status := "verified"
	if !result.Complete {
		status = "needs_review"
	}
	if e != nil {
		status = "failed"
	}
	finish, finishCancel := worker.Cleanup(parent, 10*time.Second)
	defer finishCancel()
	out := models.Draft{Status: status, Result: result, ErrorCode: errorCode(e), FinishedAt: time.Now().UTC(), ClaimToken: ""}
	return owned(w.s.db(finish).Model(&models.Draft{}), job.ID, token).Select("status", "result", "error_code", "finished_at", "claim_token", "updated_at").Updates(&out).Error
}
