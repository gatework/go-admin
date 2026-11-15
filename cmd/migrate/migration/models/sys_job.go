package models

// SysJob defines the sysjob contract.
//
//nolint:embeddedstructfieldcheck // Frozen migration field order defines historical positional SQL seed columns.
type SysJob struct {
	JobID          int    `json:"jobId" gorm:"primaryKey;autoIncrement"`
	JobName        string `json:"jobName" gorm:"size:255;"`
	JobGroup       string `json:"jobGroup" gorm:"size:255;"`
	JobType        int    `json:"jobType" gorm:"size:1;"`
	CronExpression string `json:"cronExpression" gorm:"size:255;"`
	InvokeTarget   string `json:"invokeTarget" gorm:"size:255;"`
	Args           string `json:"args" gorm:"size:255;"`
	MisfirePolicy  int    `json:"misfirePolicy" gorm:"size:255;"`
	Concurrent     int    `json:"concurrent" gorm:"size:1;"`
	Status         int    `json:"status" gorm:"size:1;"`
	EntryID        int    `json:"entry_id" gorm:"size:11;"`
	ModelTime
	ControlBy
}

// TableName returns the explicit persistence table name.
func (SysJob) TableName() string {
	return "sys_job"
}
