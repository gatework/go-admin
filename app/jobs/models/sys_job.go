package models

import (
	"gorm.io/gorm"

	"github.com/gatework/motor/common/models"
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
	EntryID        int    `json:"entry_id" gorm:"size:11;"`              // job启动时返回的id

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

// GetList stores get list data in the database.
func (e *SysJob) GetList(tx *gorm.DB, list any) (err error) {
	return tx.Table(e.TableName()).Where("status = ?", 2).Find(list).Error
}

// Update 更新SysJob
func (e *SysJob) Update(tx *gorm.DB, id any) (err error) {
	return tx.Table(e.TableName()).Where(id).Updates(&e).Error
}

// RemoveAllEntryID stores remove all entry id data in the database.
func (e *SysJob) RemoveAllEntryID(tx *gorm.DB) (update SysJob, err error) {
	if err = tx.Table(e.TableName()).Where("entry_id > ?", 0).Update("entry_id", 0).Error; err != nil {
		return
	}
	return
}
