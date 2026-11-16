package firewall

import (
	"time"

	"github.com/gatework/motor/app/net/models/firewall/types"
)

// Domain stores domain data in the database.
type Domain struct {
	ID        int       `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128" json:"name"`
	Code      string    `gorm:"size:64;uniqueIndex" json:"code"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName returns the database table used for this record.
func (Domain) TableName() string { return "sys_firewall_domain" }

// Instance stores instance data in the database.
type Instance struct {
	ID              int            `gorm:"primaryKey" json:"id"`
	DeviceID        int            `gorm:"index;uniqueIndex:ux_firewall_instance,priority:1" json:"deviceId"`
	NativeContext   string         `gorm:"size:128;uniqueIndex:ux_firewall_instance,priority:2" json:"nativeContext"`
	Name            string         `gorm:"size:128" json:"name"`
	Vendor          string         `gorm:"size:32" json:"vendor"`
	Mode            string         `gorm:"size:32" json:"mode"`
	DomainID        int            `gorm:"index" json:"domainId"`
	Version         int64          `json:"version"`
	SettingsVersion int64          `json:"settingsVersion"`
	Settings        types.Settings `gorm:"serializer:json;type:text" json:"-"`
	CurrentRevision string         `gorm:"size:64" json:"currentRevision"`
	Generation      uint64         `json:"-"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

// TableName returns the database table used for this record.
func (Instance) TableName() string { return "sys_firewall_instance" }

// ScopeBinding stores scope binding data in the database.
type ScopeBinding struct {
	ID           int    `gorm:"primaryKey" json:"id"`
	InstanceID   int    `gorm:"index" json:"instanceId"`
	RoutingScope string `gorm:"size:255" json:"routingScope"`
	DomainID     int    `gorm:"index" json:"domainId"`
	Version      int64  `json:"version"`
}

// TableName returns the database table used for this record.
func (ScopeBinding) TableName() string { return "sys_firewall_scope_binding" }

// ZoneBinding stores zone binding data in the database.
type ZoneBinding struct {
	ID           int      `gorm:"primaryKey" json:"id"`
	InstanceID   int      `gorm:"index" json:"instanceId"`
	Zone         string   `gorm:"size:255" json:"zone"`
	Scope        string   `gorm:"type:text" json:"scope"`
	RoutingScope string   `gorm:"type:text" json:"routingScope"`
	DomainID     int      `gorm:"index" json:"domainId"`
	Protected    bool     `json:"protected"`
	Networks     []string `gorm:"serializer:json;type:text" json:"networks"`
	Version      int64    `json:"version"`
}

// TableName returns the database table used for this record.
func (ZoneBinding) TableName() string { return "sys_firewall_zone_binding" }

// NetworkBinding stores network binding data in the database.
type NetworkBinding struct {
	ID            int    `gorm:"primaryKey" json:"id"`
	ZoneBindingID int    `gorm:"index;uniqueIndex:ux_firewall_network_position,priority:1" json:"zoneBindingId"`
	InstanceID    int    `gorm:"index" json:"instanceId"`
	AddressFamily int    `gorm:"index" json:"addressFamily"`
	Start         string `gorm:"size:32;index" json:"start"`
	End           string `gorm:"size:32;index" json:"end"`
	OriginalText  string `gorm:"type:text" json:"originalText"`
	Position      int    `gorm:"uniqueIndex:ux_firewall_network_position,priority:2" json:"position"`
	Version       int64  `json:"version"`
}

// TableName returns the database table used for this record.
func (NetworkBinding) TableName() string { return "sys_firewall_network_binding" }

// Coverage stores coverage data in the database.
type Coverage struct {
	ID              int    `gorm:"primaryKey" json:"id"`
	InstanceID      int    `gorm:"index" json:"instanceId"`
	DomainID        int    `gorm:"index" json:"domainId"`
	Revision        string `gorm:"size:64" json:"revision"`
	BindingsVersion int64  `json:"bindingsVersion"`
	Zone            string `gorm:"type:text" json:"zone"`
	Scope           string `gorm:"type:text" json:"scope"`
	RoutingScope    string `gorm:"type:text" json:"routingScope"`
	Network         string `gorm:"size:128" json:"network"`
	Protected       bool   `json:"protected"`
	Origin          string `gorm:"size:32" json:"origin"`
	ObjectID        string `gorm:"type:text" json:"objectId"`
	Known           bool   `json:"known"`
}

// TableName returns the database table used for this record.
func (Coverage) TableName() string { return "sys_firewall_coverage" }
