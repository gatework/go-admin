package models

import "time"

type CollectionBatch struct {
	ID              int                `gorm:"primaryKey" json:"id"`
	ActorID         int                `gorm:"uniqueIndex:ux_firewall_collection_request,priority:1" json:"-"`
	RequestKey      string             `gorm:"size:128;uniqueIndex:ux_firewall_collection_request,priority:2" json:"-"`
	PayloadHash     string             `gorm:"size:64" json:"-"`
	Kind            string             `gorm:"size:32" json:"kind"`
	Status          string             `gorm:"size:32;index" json:"status"`
	CancelRequested bool               `json:"cancelRequested"`
	CreatedAt       time.Time          `json:"createdAt"`
	UpdatedAt       time.Time          `json:"updatedAt"`
	FinishedAt      *time.Time         `json:"finishedAt"`
	Targets         []CollectionTarget `gorm:"foreignKey:BatchID" json:"targets"`
}

func (CollectionBatch) TableName() string { return "sys_firewall_collection_batch" }

type CollectionTarget struct {
	ID                int        `gorm:"primaryKey" json:"id"`
	BatchID           int        `gorm:"index;uniqueIndex:ux_firewall_collection_instance,priority:1" json:"batchId"`
	InstanceID        int        `gorm:"index;uniqueIndex:ux_firewall_collection_instance,priority:2" json:"instanceId"`
	DeviceID          int        `gorm:"index" json:"deviceId"`
	Name              string     `gorm:"size:255" json:"name"`
	IP                string     `gorm:"size:64" json:"ip"`
	Vendor            string     `gorm:"size:32" json:"vendor"`
	NativeContext     string     `gorm:"size:128" json:"nativeContext"`
	SourceHash        string     `gorm:"size:64" json:"-"`
	ExpectedRevision  string     `gorm:"size:64" json:"-"`
	InstanceVersion   int64      `json:"-"`
	ConnectionVersion int64      `json:"-"`
	LeaseToken        string     `gorm:"size:40" json:"-"`
	LeaseUntil        *time.Time `gorm:"index" json:"-"`
	Status            string     `gorm:"size:32;index" json:"status"`
	Stage             string     `gorm:"size:32" json:"stage"`
	Revision          string     `gorm:"size:64" json:"revision"`
	SnapshotID        int        `gorm:"-" json:"snapshotId"`
	Complete          bool       `json:"complete"`
	ErrorCode         string     `gorm:"size:64" json:"errorCode"`
	SessionClosed     bool       `json:"sessionClosed"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
	StartedAt         *time.Time `json:"startedAt"`
	FinishedAt        *time.Time `json:"finishedAt"`
}

func (CollectionTarget) TableName() string { return "sys_firewall_collection_target" }
