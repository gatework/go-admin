package models

import "time"

// SyncState is a singleton per tenant database. The lease and ownership token
// recover abandoned runs and fence their late writes after another run starts.
type SyncState struct {
	ID             int        `json:"-" gorm:"primaryKey;autoIncrement:false"`
	Status         string     `json:"status" gorm:"size:20;not null"`
	Token          string     `json:"-" gorm:"size:36"`
	LeaseExpiresAt *time.Time `json:"-"`
	LastSuccessAt  *time.Time `json:"lastSuccessAt,omitempty"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	FinishedAt     *time.Time `json:"finishedAt,omitempty"`
	DeviceCount    int        `json:"deviceCount" gorm:"not null;default:0"`
	Message        string     `json:"message,omitempty" gorm:"size:255"`
}

func (SyncState) TableName() string { return "sys_network_device_sync_state" }
