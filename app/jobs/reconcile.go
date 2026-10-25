package jobs

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"go-admin/app/jobs/models"
	"gorm.io/gorm"
)

// Standalone callers retain the public service Cron field. Managed tenants
// always go through their supervisor and never this unsupervised path.
// One coordinator preserves standalone serialization without permanently
// retaining every caller-owned scheduler. It captures a cron only while the
// current operation is running; timed-out callers cannot add blocked workers.
var standaloneControl cronControl

func ReconcileCron(ctx context.Context, db *gorm.DB, c *cron.Cron) error {
	var managed *supervisor
	supervisors.Range(func(_, value any) bool {
		s := value.(*supervisor)
		if s.cron == c {
			managed = s
			return false
		}
		return true
	})
	if managed != nil {
		return managed.reconcile(ctx)
	}
	if c == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return standaloneControl.run(ctx, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Explicit standalone Cron callers retain the legacy entry-ID removal
		// contract for custom jobs; managed schedulers use stable job IDs instead.
		var stopped []models.SysJob
		if err := db.WithContext(ctx).Where("status <> ? AND entry_id <> ?", 2, 0).Find(&stopped).Error; err != nil {
			return err
		}
		for _, job := range stopped {
			if err := ctx.Err(); err != nil {
				return err
			}
			c.Remove(cron.EntryID(job.EntryId))
		}
		list, err := loadJobs(ctx, db)
		if err != nil {
			return err
		}
		if err = applyJobs(ctx, c, list, nil); err != nil {
			return err
		}
		return saveEntryIDs(ctx, db, list)
	})
}

func loadJobs(ctx context.Context, db *gorm.DB) ([]models.SysJob, error) {
	var list []models.SysJob
	err := db.WithContext(ctx).Where("status = ?", 2).Find(&list).Error
	return list, err
}

func jobCore(job models.SysJob) JobCore {
	return JobCore{JobId: job.JobId, Name: job.JobName, InvokeTarget: job.InvokeTarget,
		CronExpression: job.CronExpression, Args: job.Args}
}

func entryCore(job cron.Job) (JobCore, int) {
	switch j := job.(type) {
	case *HttpJob:
		return j.JobCore, 1
	case *ExecJob:
		return j.JobCore, 2
	default:
		return JobCore{}, 0
	}
}

// applyJobs compares stable database IDs and definitions. Persisted entry_id
// is merely a display hint; cron IDs belong to one process and are never used
// to decide whether a job is registered or which job to remove.
func applyJobs(ctx context.Context, c *cron.Cron, list []models.SysJob, allowed func() bool) error {
	desired := make(map[int]*models.SysJob, len(list))
	for i := range list {
		desired[list[i].JobId] = &list[i]
	}
	retained := make(map[int]bool, len(list))
	for _, entry := range c.Entries() {
		if err := ctx.Err(); err != nil {
			return err
		}
		core, kind := entryCore(entry.Job)
		if kind == 0 {
			continue
		}
		job := desired[core.JobId]
		core.EntryId = 0
		if job != nil && job.JobType == kind && core == jobCore(*job) && !retained[core.JobId] {
			retained[core.JobId] = true
			continue
		}
		c.Remove(entry.ID)
	}
	// Read back retained IDs; caller-provided database entry IDs are untrusted.
	for i := range list {
		list[i].EntryId = 0
	}
	for _, entry := range c.Entries() {
		if err := ctx.Err(); err != nil {
			return err
		}
		core, kind := entryCore(entry.Job)
		if job := desired[core.JobId]; kind != 0 && job != nil {
			job.EntryId = int(entry.ID)
		}
	}
	for i := range list {
		if err := ctx.Err(); err != nil {
			return err
		}
		job := &list[i]
		if job.EntryId != 0 {
			continue
		}
		var runnable Job
		switch job.JobType {
		case 1:
			runnable = &HttpJob{JobCore: jobCore(*job), allowed: allowed}
		case 2:
			runnable = &ExecJob{JobCore: jobCore(*job), allowed: allowed}
		default:
			return fmt.Errorf("unsupported job type %d for job %d", job.JobType, job.JobId)
		}
		id, err := AddJob(c, runnable)
		if err != nil {
			return fmt.Errorf("registering job %d: %w", job.JobId, err)
		}
		job.EntryId = id
	}
	return nil
}

func saveEntryIDs(ctx context.Context, db *gorm.DB, list []models.SysJob) error {
	for _, job := range list {
		if err := db.WithContext(ctx).Model(&models.SysJob{}).
			Where("job_id = ? AND status = ? AND entry_id <> ?", job.JobId, 2, job.EntryId).
			UpdateColumn("entry_id", job.EntryId).Error; err != nil {
			return err
		}
	}
	return db.WithContext(ctx).Model(&models.SysJob{}).Where("status <> ? AND entry_id <> ?", 2, 0).UpdateColumn("entry_id", 0).Error
}
