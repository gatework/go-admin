package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/service"

	"github.com/gatework/motor/app/jobs"
	"github.com/gatework/motor/app/jobs/models"
	"github.com/gatework/motor/common/actions"
)

// Job carries sys job state for this module.
type Job struct {
	service.Service
	Cron       *cron.Cron
	Tenant     string
	Permission *actions.DataPermission
}

// Stop persists stopped intent so a restart or lease takeover cannot resurrect
// the job. Running invocations are allowed to finish.
func (e *Job) Stop(ctx context.Context, id int) error {
	return e.setStatus(ctx, id, models.StatusStopped)
}

// Start is idempotent and enables the persisted job. A standby instance only
// persists the intent; the lease holder dispatches it when it reconciles.
func (e *Job) Start(ctx context.Context, id int) error {
	return e.setStatus(ctx, id, models.StatusEnabled)
}

func (e *Job) setStatus(ctx context.Context, id int, status int) error {
	db := e.DB.WithContext(ctx)
	var job models.SysJob
	if err := db.Scopes(actions.Permission(job.TableName(), e.Permission)).First(&job, id).Error; err != nil {
		return err
	}
	if status == models.StatusEnabled {
		if job.JobType != models.TypeHTTP && job.JobType != models.TypeTask {
			return fmt.Errorf("unsupported job type %d", job.JobType)
		}
		if _, err := jobs.ParseSchedule(job.CronExpression); err != nil {
			return err
		}
	}
	result := db.Model(&models.SysJob{}).Scopes(actions.Permission(job.TableName(), e.Permission)).
		Where("job_id = ?", id).Updates(map[string]any{"status": status})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	var err error
	if e.Tenant != "" {
		err = jobs.Reconcile(ctx, e.Tenant)
	} else {
		err = jobs.ReconcileCron(ctx, db, e.Cron)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("操作超时！")
	}
	return err
}
