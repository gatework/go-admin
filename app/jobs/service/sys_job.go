package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"go-admin/app/jobs"
	"go-admin/app/jobs/models"
	"go-admin/common/actions"
	"go-admin/common/dto"
)

type SysJob struct {
	service.Service
	Cron       *cron.Cron
	Tenant     string
	Permission *actions.DataPermission
}

// RemoveJob persists stopped intent so a restart or lease takeover cannot
// resurrect it. Running invocations are allowed to finish.
func (e *SysJob) RemoveJob(c *dto.GeneralDelDto) error {
	return e.setStatus(c.Id, 1)
}

// StartJob is idempotent and enables the persisted job. Standby requests
// persist intent; only the lease holder dispatches it after reconciliation.
func (e *SysJob) StartJob(c *dto.GeneralGetDto) error {
	return e.setStatus(c.Id, 2)
}

func (e *SysJob) setStatus(id int, status int) error {
	var job models.SysJob
	db := e.Orm.Scopes(actions.Permission(job.TableName(), e.Permission))
	if err := db.First(&job, id).Error; err != nil {
		return err
	}
	if status == 2 {
		if job.JobType != 1 && job.JobType != 2 {
			return fmt.Errorf("unsupported job type %d", job.JobType)
		}
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.DowOptional | cron.Descriptor)
		if _, err := parser.Parse(job.CronExpression); err != nil {
			return err
		}
	}
	result := e.Orm.Model(&models.SysJob{}).Scopes(actions.Permission(job.TableName(), e.Permission)).
		Where("job_id = ?", id).Updates(map[string]any{"status": status})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	ctx := e.Orm.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}
	var err error
	if e.Tenant != "" {
		err = jobs.Reconcile(ctx, e.Tenant)
	} else {
		err = jobs.ReconcileCron(ctx, e.Orm, e.Cron)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		e.Msg = "操作超时！"
		return errors.New(e.Msg)
	}
	return err
}
