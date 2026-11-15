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
	"github.com/gatework/motor/common/dto"
)

// SysJob carries sys job state for this module.
type SysJob struct {
	service.Service
	Cron       *cron.Cron
	Tenant     string
	Permission *actions.DataPermission
}

// RemoveJob persists stopped intent so a restart or lease takeover cannot
// resurrect it. Running invocations are allowed to finish.
func (e *SysJob) RemoveJob(c *dto.DeleteRequest) error {
	return e.setStatus(c.ID, models.StatusStopped)
}

// StartJob is idempotent and enables the persisted job. Standby requests
// persist intent; only the lease holder dispatches it after reconciliation.
func (e *SysJob) StartJob(c *dto.GetRequest) error {
	return e.setStatus(c.ID, models.StatusEnabled)
}

func (e *SysJob) setStatus(id int, status int) error {
	var job models.SysJob
	db := e.DB.Scopes(actions.Permission(job.TableName(), e.Permission))
	if err := db.First(&job, id).Error; err != nil {
		return err
	}
	if status == models.StatusEnabled {
		if job.JobType != models.TypeHTTP && job.JobType != models.TypeTask {
			return fmt.Errorf("unsupported job type %d", job.JobType)
		}
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.DowOptional | cron.Descriptor)
		if _, err := parser.Parse(job.CronExpression); err != nil {
			return err
		}
	}
	result := e.DB.Model(&models.SysJob{}).Scopes(actions.Permission(job.TableName(), e.Permission)).
		Where("job_id = ?", id).Updates(map[string]any{"status": status})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	ctx := e.DB.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}
	var err error
	if e.Tenant != "" {
		err = jobs.Reconcile(ctx, e.Tenant)
	} else {
		err = jobs.ReconcileCron(ctx, e.DB, e.Cron)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("操作超时！")
	}
	return err
}
