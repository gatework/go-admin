// Package diagnosticrun keeps the audit trail of network diagnostics run from
// the backend: who ran which tool against which target, and when. The jobs
// themselves live only in the memory of the process that runs them.
package diagnosticrun

import "time"

// Record is one diagnostic started by a user.
type Record struct {
	ID        int    `gorm:"primaryKey"`
	UserID    int    `gorm:"index"`
	JobID     string `gorm:"size:32"`
	Tool      string `gorm:"size:32"`
	Target    string `gorm:"size:255;index"`
	Family    string `gorm:"size:8"`
	Mode      string `gorm:"size:16"`
	Port      int
	Command   string    `gorm:"type:text"`
	StartedAt time.Time `gorm:"index"`
}

// TableName returns the stable database table for Record.
func (Record) TableName() string { return "sys_diagnostic_run" }
