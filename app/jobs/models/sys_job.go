package models

import (
	"github.com/gatework/motor/common/models"
)

// Job types (sys_job.job_type).
const (
	// TypeHTTP requests InvokeTarget as a URL.
	TypeHTTP = 1
	// TypeTask runs the registered task named by InvokeTarget.
	TypeTask = 2
)

// Job states (sys_job.status), as the sys_job_status dictionary labels them.
const (
	StatusStopped = 1
	StatusEnabled = 2
)

// SysJob stores sys job data in the database.
type SysJob struct {
	models.ControlBy
	models.ModelTime
	JobID          int    `json:"jobId" gorm:"primaryKey;autoIncrement"` // 编码
	JobName        string `json:"jobName" gorm:"size:255;"`              // 名称
	JobGroup       string `json:"jobGroup" gorm:"size:255;"`             // 任务分组
	JobType        int    `json:"jobType" gorm:"size:1;"`                // 任务类型
	CronExpression string `json:"cronExpression" gorm:"size:255;"`       // cron表达式
	InvokeTarget   string `json:"invokeTarget" gorm:"size:255;"`         // 调用目标
	Args           string `json:"args" gorm:"size:255;"`                 // 目标参数
	MisfirePolicy  int    `json:"misfirePolicy" gorm:"size:255;"`        // 执行策略
	Concurrent     int    `json:"concurrent" gorm:"size:1;"`             // 是否并发
	Status         int    `json:"status" gorm:"size:1;"`                 // 状态
	EntryID        int    `json:"entryId" gorm:"size:11;"`               // job启动时返回的id

	DataScope string `json:"dataScope" gorm:"-"`
}

// TableName returns the database table used for this record.
func (*SysJob) TableName() string {
	return "sys_job"
}

// GetID returns the resource identity.
func (e *SysJob) GetID() any {
	return e.JobID
}

// SetCreateBy stores set create by data in the database.
func (e *SysJob) SetCreateBy(createBy int) {
	e.CreateBy = createBy
}

// SetUpdateBy stores set update by data in the database.
func (e *SysJob) SetUpdateBy(updateBy int) {
	e.UpdateBy = updateBy
}
