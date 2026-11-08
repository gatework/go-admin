package models

import (
	"go-admin/app/network/connection"
	"go-admin/app/network/lease"
	"time"
)

type Connection = connection.Settings

type DeviceSnapshot struct {
	ID         int     `json:"id"`
	Name       string  `json:"name"`
	Hostname   string  `json:"hostname"`
	IP         string  `json:"ip"`
	Vendor     string  `json:"vendor"`
	Model      string  `json:"model"`
	DeviceType *string `json:"deviceType,omitempty"`
	DeviceRole *string `json:"deviceRole,omitempty"`
	Profile    string  `json:"profile,omitempty"`
}
type Order struct {
	Kind            string     `gorm:"size:24;not null;default:script;index" json:"kind"`
	FabricID        int        `gorm:"index" json:"fabricId,omitempty"`
	ID              int        `gorm:"primaryKey" json:"id"`
	RequestNo       string     `gorm:"size:64;uniqueIndex" json:"requestNo"`
	RequestKey      string     `gorm:"size:100;uniqueIndex:ux_delivery_request,priority:2" json:"-"`
	PayloadHash     string     `gorm:"size:64" json:"-"`
	Title           string     `gorm:"size:200" json:"title"`
	Status          string     `gorm:"size:24;index" json:"status"`
	Version         int64      `json:"version"`
	DeviceCount     int        `json:"deviceCount"`
	SucceededCount  int        `json:"succeededCount"`
	FailedCount     int        `json:"failedCount"`
	UncertainCount  int        `json:"uncertainCount"`
	CancelledCount  int        `json:"cancelledCount"`
	CreateBy        int        `gorm:"column:create_by;index;uniqueIndex:ux_delivery_request,priority:1" json:"createdBy"`
	CreatedByName   string     `gorm:"size:128" json:"createdByName"`
	RetryOfID       *int       `json:"retryOfId,omitempty"`
	ScheduledAt     *time.Time `gorm:"index" json:"scheduledAt,omitempty"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	CancelRequested bool       `json:"-"`
	Targets         []Target   `gorm:"foreignKey:OrderID" json:"targets,omitempty"`
	Editable        bool       `gorm:"-" json:"editable"`
	Executable      bool       `gorm:"-" json:"executable"`
	Cancellable     bool       `gorm:"-" json:"cancellable"`
	Retryable       bool       `gorm:"-" json:"retryable"`
}

func (Order) TableName() string { return "sys_delivery_order" }

type Target struct {
	ID                int            `gorm:"primaryKey" json:"id"`
	OrderID           int            `gorm:"index;not null" json:"-"`
	DeviceID          int            `gorm:"index;not null" json:"deviceId"`
	Device            DeviceSnapshot `gorm:"serializer:json;type:text" json:"device"`
	Connection        Connection     `gorm:"serializer:json;type:text" json:"-"`
	ConnectionVersion int64          `json:"-"`
	Script            string         `gorm:"size:65536" json:"script"`
	ScriptSHA256      string         `gorm:"size:64" json:"scriptSha256"`
	ScriptLineCount   int            `json:"scriptLineCount"`
	Status            string         `gorm:"size:24;index" json:"status"`
	CompletedSteps    int            `json:"completedSteps"`
	Attempted         bool           `json:"-"`
	LeaseToken        string         `gorm:"size:40" json:"-"`
	LeaseUntil        *time.Time     `gorm:"index" json:"-"`
	StartedAt         *time.Time     `json:"startedAt,omitempty"`
	FinishedAt        *time.Time     `json:"finishedAt,omitempty"`
	ErrorCode         string         `gorm:"size:64" json:"errorCode,omitempty"`
	Output            string         `gorm:"size:4194304" json:"output,omitempty"`
	Retryable         bool           `gorm:"-" json:"retryable"`
}

func (Target) TableName() string { return "sys_delivery_target" }

// DeviceLease is the device lease row, owned by app/network/lease. The alias
// keeps the migration that created the table, and its readers, unchanged.
type DeviceLease = lease.Record

type Receipt struct {
	ID        int    `gorm:"primaryKey"`
	TargetID  int    `gorm:"uniqueIndex:ux_delivery_receipt"`
	Step      int    `gorm:"uniqueIndex:ux_delivery_receipt"`
	Output    string `gorm:"size:4194304"`
	CreatedAt time.Time
}

func (Receipt) TableName() string { return "sys_delivery_receipt" }

type Audit struct {
	ID        int `gorm:"primaryKey"`
	OrderID   int `gorm:"index"`
	DeviceID  int
	ActorID   int
	Action    string `gorm:"size:40"`
	CreatedAt time.Time
}

func (Audit) TableName() string { return "sys_delivery_audit" }
