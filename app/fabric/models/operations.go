package models

import (
	vxlan "github.com/gatework/go-vxlan"
	delivery "go-admin/app/delivery/models"
	"time"
)

type Collection struct {
	ID           int                `gorm:"primaryKey" json:"id"`
	FabricID     int                `gorm:"index" json:"fabricId"`
	RequestKey   string             `gorm:"size:100;uniqueIndex:ux_fabric_collect,priority:2" json:"requestKey"`
	PayloadHash  string             `gorm:"size:64" json:"-"`
	CreateBy     int                `gorm:"uniqueIndex:ux_fabric_collect,priority:1" json:"createdBy"`
	Mode         string             `gorm:"size:20" json:"mode"`
	Status       string             `gorm:"size:24;index" json:"status"`
	Version      int64              `json:"version"`
	GroupingHash string             `gorm:"size:64" json:"groupingHash"`
	Targets      []CollectionTarget `gorm:"serializer:fabric_operations;type:text" json:"targets"`
	CreatedAt    time.Time          `json:"createdAt"`
	StartedAt    *time.Time         `json:"startedAt,omitempty"`
	FinishedAt   *time.Time         `json:"finishedAt,omitempty"`
	LeaseUntil   *time.Time         `gorm:"index" json:"-"`
}

func (Collection) TableName() string { return "sys_fabric_collection" }

type CollectionTarget struct {
	DeviceID   int                     `json:"deviceId"`
	Status     string                  `json:"status"`
	ErrorCode  string                  `json:"errorCode,omitempty"`
	SourceHash string                  `json:"-"`
	Device     delivery.DeviceSnapshot `json:"-"`
	Connection delivery.Connection     `json:"-"`
}

type Change struct {
	ID                  int            `gorm:"primaryKey" json:"id"`
	FabricID            int            `gorm:"index" json:"fabricId"`
	RequestKey          string         `gorm:"size:100;uniqueIndex:ux_fabric_change,priority:2" json:"requestKey"`
	PayloadHash         string         `gorm:"size:64" json:"-"`
	CreateBy            int            `gorm:"uniqueIndex:ux_fabric_change,priority:1" json:"createdBy"`
	ExecuteBy           int            `json:"-"`
	SourceDeviceID      int            `json:"sourceDeviceId"`
	Kind                string         `gorm:"size:24" json:"kind"`
	VNI                 int            `json:"vni"`
	Direction           string         `gorm:"size:3" json:"direction,omitempty"`
	Status              string         `gorm:"size:24;index" json:"status"`
	Version             int64          `json:"version"`
	DeliveryOrderID     int            `json:"deliveryOrderId"`
	GroupingHash        string         `gorm:"size:64" json:"groupingHash"`
	PlanHash            string         `gorm:"size:64" json:"planHash"`
	SourceObservationID int            `json:"sourceObservationId"`
	SourceHash          string         `gorm:"size:64" json:"-"`
	SourceConfigHash    string         `gorm:"size:64" json:"-"`
	Plan                vxlan.Record   `gorm:"serializer:json;type:text" json:"plan"`
	Targets             []ChangeTarget `gorm:"serializer:fabric_operations;type:text" json:"targets"`
	Authorization       []int          `gorm:"serializer:json;type:text" json:"-"`
	CreatedAt           time.Time      `json:"createdAt"`
	StartedAt           *time.Time     `json:"startedAt,omitempty"`
	FinishedAt          *time.Time     `json:"finishedAt,omitempty"`
	LeaseUntil          *time.Time     `gorm:"index" json:"-"`
}

func (Change) TableName() string { return "sys_fabric_change" }

type ChangeTarget struct {
	DeviceID              int                     `json:"deviceId"`
	DeliveryTargetID      int                     `json:"deliveryTargetId"`
	Status                string                  `json:"status"`
	ErrorCode             string                  `json:"errorCode,omitempty"`
	CompletedSteps        int                     `json:"completedSteps"`
	ConfigurationVerified bool                    `json:"configurationVerified"`
	RuntimeState          string                  `json:"runtimeState"`
	Attempted             bool                    `json:"attempted"`
	Commands              []string                `json:"commands"`
	SourceHash            string                  `json:"-"`
	ConfigHash            string                  `json:"-"`
	ObservationID         int                     `json:"observationId"`
	Device                delivery.DeviceSnapshot `json:"-"`
	Connection            delivery.Connection     `json:"-"`
	PreEvidence           vxlan.Snapshot          `gorm:"-" json:"-"`
	PostEvidence          vxlan.Snapshot          `gorm:"-" json:"-"`
	Receipts              []ChangeReceipt         `json:"receipts"`
}
type ChangeReceipt struct {
	Step    int       `json:"step"`
	Command string    `json:"command"`
	Status  string    `json:"status"`
	At      time.Time `json:"at"`
}
