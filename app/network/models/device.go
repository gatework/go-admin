package models

import (
	"time"

	common "github.com/go-admin-team/go-admin-core/v2/sdk/contract/models"
	"gorm.io/plugin/soft_delete"
)

// SourceFacts is the explicit allowlist of Netdisco inventory facts. Never
// persist the original response: it may contain SNMP credentials and logs.
type SourceFacts struct {
	IP               string   `json:"ip" gorm:"column:ip;size:45;not null;uniqueIndex:ux_network_device_ip"`
	Name             string   `json:"name" gorm:"size:512"`
	Hostname         string   `json:"hostname" gorm:"size:512"`
	Serial           string   `json:"serial" gorm:"size:512"`
	Vendor           string   `json:"vendor" gorm:"size:255;index"`
	HardwareModel    string   `json:"model" gorm:"column:model;size:255;index"`
	OS               string   `json:"os" gorm:"column:os;size:255"`
	OSVersion        string   `json:"osVersion" gorm:"column:os_version;size:512"`
	Location         string   `json:"location" gorm:"type:text"`
	Contact          string   `json:"contact" gorm:"type:text"`
	Description      string   `json:"description" gorm:"type:text"`
	MACAddress       string   `json:"macAddress" gorm:"column:mac_address;size:255"`
	SourceTags       []string `json:"sourceTags" gorm:"serializer:json;type:text;not null"`
	SourceCreatedAt  string   `json:"sourceCreatedAt" gorm:"size:40"`
	LastDiscoveredAt string   `json:"lastDiscoveredAt" gorm:"size:40"`
}

// Device keeps manually maintained fields outside the source-fact allowlist.
type Device struct {
	common.Model
	SourceFacts  `gorm:"embedded"`
	LastSyncedAt time.Time `json:"lastSyncedAt"`
	Version      int64     `json:"version" gorm:"not null;default:1"`
	LegacyGroup  string    `json:"-" gorm:"column:group_name;size:128;index"`
	Group        string    `json:"group" gorm:"-"`
	GroupID      *int      `json:"groupId" gorm:"column:group_id;index"`
	// Empty defaults are durable migration markers. The discovery writer always
	// sets both modes explicitly, so an interrupted backfill can safely resume.
	GroupSource          string   `json:"groupSource" gorm:"size:10;not null;default:''"`
	ClassificationSource string   `json:"classificationSource" gorm:"size:10;not null;default:''"`
	Status               string   `json:"status" gorm:"size:20;not null;default:pending;index"`
	DeviceType           string   `json:"deviceType" gorm:"size:64;index"`
	DeviceRole           string   `json:"deviceRole" gorm:"size:64;index"`
	Owner                string   `json:"owner" gorm:"size:128"`
	ContactEmail         string   `json:"contactEmail" gorm:"size:254"`
	ContactPhone         string   `json:"contactPhone" gorm:"size:64"`
	Tags                 []string `json:"tags" gorm:"serializer:json;type:text;not null"`
	Notes                string   `json:"notes" gorm:"type:text"`
	common.ControlBy
	common.ModelTime
	DeletedAt soft_delete.DeletedAt `json:"-" gorm:"softDelete:milli;not null;default:0;index"`
}

func (Device) TableName() string     { return "sys_network_device" }
func (d *Device) GetId() interface{} { return d.Id }
