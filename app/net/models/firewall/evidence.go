package firewall

import (
	"time"

	fm "github.com/gatework/firemon"
)

// Import stores import data in the database.
type Import struct {
	Software         string    `gorm:"type:text" json:"-"`
	ActorID          int       `gorm:"index" json:"-"`
	PayloadHash      string    `gorm:"size:64" json:"-"`
	ClaimToken       string    `gorm:"size:64" json:"-"`
	ClaimUntil       time.Time `json:"-"`
	FinishedAt       time.Time `json:"-"`
	ID               int       `gorm:"primaryKey" json:"id"`
	InstanceID       int       `gorm:"index;uniqueIndex:ux_firewall_import,priority:1" json:"instanceId"`
	RequestKey       string    `gorm:"size:128;uniqueIndex:ux_firewall_import,priority:2" json:"-"`
	ExpectedRevision string    `gorm:"size:64" json:"-"`
	Status           string    `gorm:"size:32;index" json:"status"`
	Revision         string    `gorm:"size:64" json:"revision"`
	ErrorCode        string    `gorm:"size:64" json:"errorCode"`
	Filename         string    `gorm:"size:255" json:"filename"`
	Complete         bool      `json:"complete"`
	CapturedAt       time.Time `json:"capturedAt"`
	SourcePath       string    `gorm:"size:1024" json:"-"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// TableName returns the database table used for this record.
func (Import) TableName() string { return "sys_firewall_import" }

// Diagnostic stores diagnostic data in the database.
type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Object  string `json:"object"`
}

// Snapshot stores snapshot data in the database.
type Snapshot struct {
	SourceDigest    string       `gorm:"size:64" json:"sourceDigest"`
	SemanticDigest  string       `gorm:"size:64" json:"semanticDigest"`
	ParserVersion   string       `gorm:"type:text" json:"parserVersion"`
	CapturedAt      time.Time    `json:"capturedAt"`
	Software        string       `gorm:"type:text" json:"software"`
	ID              int          `gorm:"primaryKey" json:"id"`
	InstanceID      int          `gorm:"index;uniqueIndex:ux_firewall_snapshot,priority:1" json:"instanceId"`
	Revision        string       `gorm:"size:64;uniqueIndex:ux_firewall_snapshot,priority:2" json:"revision"`
	Generation      uint64       `json:"-"`
	Complete        bool         `json:"complete"`
	Current         bool         `json:"current"`
	Diagnostics     []Diagnostic `gorm:"serializer:json;type:text" json:"diagnostics"`
	ProjectionState string       `gorm:"size:32" json:"projectionState"`
	CreatedAt       time.Time    `json:"createdAt"`
}

// TableName returns the database table used for this record.
func (Snapshot) TableName() string { return "sys_firewall_snapshot" }

// Native stores native data in the database.
// NativeKey hashes the complete native identity rather than indexing unbounded IDs.
// The containing typed table supplies kind isolation; SnapshotID supplies revision isolation.
type Native struct {
	ID          int          `gorm:"primaryKey" json:"id"`
	SnapshotID  int          `gorm:"index;uniqueIndex:,composite:native,priority:1" json:"-"`
	NativeKey   string       `gorm:"size:64;uniqueIndex:,composite:native,priority:2" json:"-"`
	NativeID    string       `gorm:"type:text" json:"nativeId"`
	Name        string       `gorm:"type:text" json:"name"`
	Scope       string       `gorm:"type:text" json:"scope"`
	Position    int          `gorm:"index" json:"position"`
	State       string       `gorm:"size:32;index" json:"state"`
	Diagnostics []Diagnostic `gorm:"serializer:json;type:text" json:"diagnostics"`
}

// Interface stores interface data in the database.
type Interface struct {
	Native `gorm:"embedded"`
	Record fm.InterfaceRecord `gorm:"serializer:json;type:text" json:"record"`
}

// TableName returns the database table used for this record.
func (Interface) TableName() string { return "sys_firewall_interface" }

// Route stores route data in the database.
type Route struct {
	Native `gorm:"embedded"`
	Record fm.RouteRecord `gorm:"serializer:json;type:text" json:"record"`
}

// TableName returns the database table used for this record.
func (Route) TableName() string { return "sys_firewall_route" }

// Zone stores zone data in the database.
type Zone struct {
	Native `gorm:"embedded"`
	Record fm.ZoneRecord `gorm:"serializer:json;type:text" json:"record"`
}

// TableName returns the database table used for this record.
func (Zone) TableName() string { return "sys_firewall_zone" }

// Address stores address data in the database.
type Address struct {
	Native `gorm:"embedded"`
	Record fm.AddressRecord `gorm:"serializer:json;type:text" json:"record"`
}

// TableName returns the database table used for this record.
func (Address) TableName() string { return "sys_firewall_address" }

// AddressGroup stores address group data in the database.
type AddressGroup struct {
	Native `gorm:"embedded"`
	Record fm.AddressGroupRecord `gorm:"serializer:json;type:text" json:"record"`
}

// TableName returns the database table used for this record.
func (AddressGroup) TableName() string { return "sys_firewall_address_group" }

// Service stores service data in the database.
type Service struct {
	Native `gorm:"embedded"`
	Record fm.ServiceRecord `gorm:"serializer:json;type:text" json:"record"`
}

// TableName returns the database table used for this record.
func (Service) TableName() string { return "sys_firewall_service" }

// ServiceGroup stores service group data in the database.
type ServiceGroup struct {
	Native `gorm:"embedded"`
	Record fm.ServiceGroupRecord `gorm:"serializer:json;type:text" json:"record"`
}

// TableName returns the database table used for this record.
func (ServiceGroup) TableName() string { return "sys_firewall_service_group" }

// Schedule stores schedule data in the database.
type Schedule struct {
	Native `gorm:"embedded"`
	Record fm.ScheduleRecord `gorm:"serializer:json;type:text" json:"record"`
}

// TableName returns the database table used for this record.
func (Schedule) TableName() string { return "sys_firewall_schedule" }

// Rule stores rule data in the database.
type Rule struct {
	Native `gorm:"embedded"`
	Record fm.RuleRecord `gorm:"serializer:json;type:text" json:"record"`
}

// TableName returns the database table used for this record.
func (Rule) TableName() string { return "sys_firewall_rule" }

// RuleBinding stores rule binding data in the database.
type RuleBinding struct {
	Native `gorm:"embedded"`
	Record fm.RuleBindingRecord `gorm:"serializer:json;type:text" json:"record"`
}

// TableName returns the database table used for this record.
func (RuleBinding) TableName() string { return "sys_firewall_rule_binding" }

// StaticNAT stores static nat data in the database.
type StaticNAT struct {
	Native `gorm:"embedded"`
	Record fm.StaticNATRecord `gorm:"serializer:json;type:text" json:"record"`
}

// TableName returns the database table used for this record.
func (StaticNAT) TableName() string { return "sys_firewall_static_nat" }

// DynamicNAT stores dynamic nat data in the database.
type DynamicNAT struct {
	Native `gorm:"embedded"`
	Record fm.DynamicNATRecord `gorm:"serializer:json;type:text" json:"record"`
}

// TableName returns the database table used for this record.
func (DynamicNAT) TableName() string { return "sys_firewall_dynamic_nat" }

// NATPool stores natpool data in the database.
type NATPool struct {
	Native `gorm:"embedded"`
	Record fm.NATPoolRecord `gorm:"serializer:json;type:text" json:"record"`
}

// TableName returns the database table used for this record.
func (NATPool) TableName() string { return "sys_firewall_nat_pool" }

// Reference stores reference data in the database.
type Reference struct {
	ID             int    `gorm:"primaryKey" json:"-"`
	SnapshotID     int    `gorm:"index" json:"-"`
	SourceKind     string `gorm:"size:32;index" json:"sourceKind"`
	SourceID       int    `gorm:"index" json:"sourceId"`
	TargetKind     string `gorm:"size:32;index" json:"targetKind"`
	TargetID       int    `gorm:"index" json:"targetId"`
	TargetNativeID string `gorm:"type:text" json:"-"`
	Role           string `gorm:"size:64" json:"role"`
	Position       int    `json:"position"`
	Resolved       bool   `json:"resolved"`
}

// TableName returns the database table used for this record.
func (Reference) TableName() string { return "sys_firewall_reference" }

// AddressRange stores address range data in the database.
type AddressRange struct {
	ID         int    `gorm:"primaryKey"`
	SnapshotID int    `gorm:"index"`
	ObjectKind string `gorm:"size:32"`
	ObjectID   int    `gorm:"index"`
	Position   int
	Family     int
	First      string `gorm:"size:32;index"`
	Last       string `gorm:"size:32;index"`
}

// TableName returns the database table used for this record.
func (AddressRange) TableName() string { return "sys_firewall_address_range" }

// ServiceTerm stores service term data in the database.
type ServiceTerm struct {
	ID              int `gorm:"primaryKey"`
	SnapshotID      int `gorm:"index"`
	ObjectID        int `gorm:"index"`
	Position        int
	ProtocolLow     uint16
	ProtocolHigh    uint16
	SourceLow       uint16
	SourceHigh      uint16
	DestinationLow  uint16
	DestinationHigh uint16
	ICMPTypeLow     uint16
	ICMPTypeHigh    uint16
	ICMPCodeLow     uint16
	ICMPCodeHigh    uint16
}

// TableName returns the database table used for this record.
func (ServiceTerm) TableName() string { return "sys_firewall_service_term" }
