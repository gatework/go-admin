package inventory

import (
	"time"

	common "github.com/gatework/motor-core/sdk/contract/models"
)

// DeviceGroup stores shared taxonomy independently of scoped device counts.
// Empty groups are physically deleted, allowing explicit recreation of a code.
// DeviceGroup records device group state for network persistence.
type DeviceGroup struct {
	common.ControlBy
	common.Model
	ParentID    *int      `json:"parentId" gorm:"column:parent_id;index"`
	Code        string    `json:"code" gorm:"size:100;not null;uniqueIndex:ux_network_device_group_code"`
	Name        string    `json:"name" gorm:"size:128;not null"`
	Description string    `json:"description" gorm:"type:text"`
	Enabled     bool      `json:"enabled" gorm:"not null"`
	Position    int       `json:"position" gorm:"not null;default:0"`
	Version     int64     `json:"version" gorm:"not null;default:1"`
	CreatedAt   time.Time `json:"-"`
	UpdatedAt   time.Time `json:"-"`
}

// TableName returns the stable database table for DeviceGroup.
func (DeviceGroup) TableName() string { return "sys_network_device_group" }

// DeviceGroupLock serializes hierarchy and membership changes per tenant.
// its memberships across processes and dialects; it is held until commit.
// DeviceGroupLock records device group lock state for network persistence.
type DeviceGroupLock struct {
	ID      int   `gorm:"primaryKey;autoIncrement:false"`
	Version int64 `gorm:"not null;default:0"`
}

// TableName returns the stable database table for DeviceGroupLock.
func (DeviceGroupLock) TableName() string { return "sys_network_device_group_lock" }
