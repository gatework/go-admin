package inventory

import (
	"gorm.io/plugin/soft_delete"

	common "github.com/gatework/motor-core/sdk/contract/models"
)

// CircuitBinding contains only identifiers and readable snapshots, never
// upstream connection details. Snapshot names are filled by the service.
type CircuitBinding struct {
	HostID      string `json:"hostId" gorm:"column:host_id;size:32"`
	HostName    string `json:"hostName" gorm:"size:512"`
	InItemID    string `json:"inItemId" gorm:"column:in_item_id;size:32"`
	InItemName  string `json:"inItemName" gorm:"size:512"`
	OutItemID   string `json:"outItemId" gorm:"column:out_item_id;size:32"`
	OutItemName string `json:"outItemName" gorm:"size:512"`
}

// CircuitEndpoint records circuit endpoint state for network persistence.
type CircuitEndpoint struct {
	DeviceID     *int           `json:"deviceId" gorm:"column:device_id;index"`
	Name         string         `json:"name" gorm:"size:160"`
	Address      string         `json:"address" gorm:"size:512"`
	Interface    string         `json:"interface" gorm:"size:160"`
	IP           string         `json:"ip" gorm:"column:ip;size:45"`
	ContactName  string         `json:"contactName" gorm:"size:128"`
	ContactPhone string         `json:"contactPhone" gorm:"size:64"`
	Monitoring   CircuitBinding `json:"monitoring" gorm:"embedded;embeddedPrefix:monitoring_"`
}

// CircuitFields is the complete editable allowlist shared by the DTO and model.
// Ownership, primary key, timestamps and deletion marker are deliberately absent.
type CircuitFields struct {
	Version       int64           `json:"version" gorm:"not null;default:1"`
	CircuitNo     string          `json:"circuitNo" gorm:"size:120;not null;uniqueIndex:ux_network_circuit_no"`
	Name          string          `json:"name" gorm:"size:160;not null"`
	Provider      string          `json:"provider" gorm:"size:160;not null;index"`
	Category      string          `json:"category" gorm:"size:32;not null;index"`
	Purpose       string          `json:"purpose" gorm:"size:32;not null"`
	Status        string          `json:"status" gorm:"size:20;not null;index"`
	BandwidthMbps *int            `json:"bandwidthMbps"`
	Region        string          `json:"region" gorm:"size:120;index"`
	Organization  string          `json:"organization" gorm:"size:160;index"`
	ContractNo    string          `json:"contractNo" gorm:"size:160"`
	InstalledOn   string          `json:"installedOn" gorm:"size:10"`
	ExpiresOn     string          `json:"expiresOn" gorm:"size:10"`
	Payer         string          `json:"payer" gorm:"size:16;not null"`
	Fee           string          `json:"fee" gorm:"size:13"`
	Currency      string          `json:"currency" gorm:"size:3;not null"`
	BillingCycle  string          `json:"billingCycle" gorm:"size:16"`
	RouteNotes    string          `json:"routeNotes" gorm:"type:text"`
	Notes         string          `json:"notes" gorm:"type:text"`
	AEndpoint     CircuitEndpoint `json:"aEndpoint" gorm:"embedded;embeddedPrefix:a_"`
	ZEndpoint     CircuitEndpoint `json:"zEndpoint" gorm:"embedded;embeddedPrefix:z_"`
}

// Circuit records circuit state for network persistence.
type Circuit struct {
	common.Model
	CircuitFields    `gorm:"embedded"`
	common.ControlBy `json:"-"`
	common.ModelTime
	DeletedAt soft_delete.DeletedAt `json:"-" gorm:"softDelete:milli;not null;default:0;index"`
}

// TableName returns the stable database table for Circuit.
func (Circuit) TableName() string { return "sys_network_circuit" }

// GetID returns the record's persistent identity.
func (c Circuit) GetID() any { return c.ID }
