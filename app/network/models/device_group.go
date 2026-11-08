package models

import (
	"time"

	common "github.com/go-admin-team/go-admin-core/v2/sdk/contract/models"
)

// Groups are shared taxonomy. Membership/counts still use device data scope.
// Empty groups are physically deleted, allowing explicit recreation of a code.
type DeviceGroup struct {
	common.Model
	ParentID    *int   `json:"parentId" gorm:"column:parent_id;index"`
	Code        string `json:"code" gorm:"size:100;not null;uniqueIndex:ux_network_device_group_code"`
	Name        string `json:"name" gorm:"size:128;not null"`
	Description string `json:"description" gorm:"type:text"`
	Enabled     bool   `json:"enabled" gorm:"not null"`
	Position    int    `json:"position" gorm:"not null;default:0"`
	Version     int64  `json:"version" gorm:"not null;default:1"`
	common.ControlBy
	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"-"`
}

func (DeviceGroup) TableName() string { return "sys_network_device_group" }

// One write lock per tenant database serializes changes to the group graph and
// its memberships across processes and dialects; it is held until commit.
type DeviceGroupLock struct {
	ID      int   `gorm:"primaryKey;autoIncrement:false"`
	Version int64 `gorm:"not null;default:0"`
}

func (DeviceGroupLock) TableName() string { return "sys_network_device_group_lock" }
