// Package firewall owns manual workflow state and rebuildable native projections.
package firewall

import (
	"time"

	fm "github.com/gatework/firemon"
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

// ScopeBinding stores scope binding data in the database.
type ScopeBinding struct {
	ID           int    `gorm:"primaryKey" json:"id"`
	InstanceID   int    `gorm:"index" json:"instanceId"`
	RoutingScope string `gorm:"size:255" json:"routingScope"`
	DomainID     int    `gorm:"index" json:"domainId"`
	Version      int64  `json:"version"`
}

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

// Route stores route data in the database.
type Route struct {
	Native `gorm:"embedded"`
	Record fm.RouteRecord `gorm:"serializer:json;type:text" json:"record"`
}

// Zone stores zone data in the database.
type Zone struct {
	Native `gorm:"embedded"`
	Record fm.ZoneRecord `gorm:"serializer:json;type:text" json:"record"`
}

// Address stores address data in the database.
type Address struct {
	Native `gorm:"embedded"`
	Record fm.AddressRecord `gorm:"serializer:json;type:text" json:"record"`
}

// AddressGroup stores address group data in the database.
type AddressGroup struct {
	Native `gorm:"embedded"`
	Record fm.AddressGroupRecord `gorm:"serializer:json;type:text" json:"record"`
}

// Service stores service data in the database.
type Service struct {
	Native `gorm:"embedded"`
	Record fm.ServiceRecord `gorm:"serializer:json;type:text" json:"record"`
}

// ServiceGroup stores service group data in the database.
type ServiceGroup struct {
	Native `gorm:"embedded"`
	Record fm.ServiceGroupRecord `gorm:"serializer:json;type:text" json:"record"`
}

// Schedule stores schedule data in the database.
type Schedule struct {
	Native `gorm:"embedded"`
	Record fm.ScheduleRecord `gorm:"serializer:json;type:text" json:"record"`
}

// Rule stores rule data in the database.
type Rule struct {
	Native `gorm:"embedded"`
	Record fm.RuleRecord `gorm:"serializer:json;type:text" json:"record"`
}

// RuleBinding stores rule binding data in the database.
type RuleBinding struct {
	Native `gorm:"embedded"`
	Record fm.RuleBindingRecord `gorm:"serializer:json;type:text" json:"record"`
}

// StaticNAT stores static nat data in the database.
type StaticNAT struct {
	Native `gorm:"embedded"`
	Record fm.StaticNATRecord `gorm:"serializer:json;type:text" json:"record"`
}

// DynamicNAT stores dynamic nat data in the database.
type DynamicNAT struct {
	Native `gorm:"embedded"`
	Record fm.DynamicNATRecord `gorm:"serializer:json;type:text" json:"record"`
}

// NATPool stores natpool data in the database.
type NATPool struct {
	Native `gorm:"embedded"`
	Record fm.NATPoolRecord `gorm:"serializer:json;type:text" json:"record"`
}

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

// Request stores request data in the database.
type Request struct {
	ActorID    int              `gorm:"index" json:"-"`
	ID         int              `gorm:"primaryKey" json:"id"`
	Title      string           `gorm:"size:255" json:"title"`
	DomainID   int              `gorm:"index" json:"domainId"`
	Version    int64            `json:"version"`
	Input      types.Input      `gorm:"serializer:json;type:text" json:"input"`
	Normalized types.Normalized `gorm:"serializer:json;type:text" json:"normalized"`
	CreatedAt  time.Time        `json:"createdAt"`
	UpdatedAt  time.Time        `json:"updatedAt"`
}

// RequestRow stores request row data in the database.
type RequestRow struct {
	Key            string              `gorm:"size:128" json:"key"`
	Line           int                 `json:"line"`
	Raw            types.InputRow      `gorm:"serializer:json;type:text" json:"raw"`
	ID             int                 `gorm:"primaryKey" json:"id"`
	RequestID      int                 `gorm:"index" json:"requestId"`
	RequestVersion int64               `json:"requestVersion"`
	Position       int                 `json:"position"`
	Normalized     types.NormalizedRow `gorm:"serializer:json;type:text" json:"normalized"`
}

// Analysis stores analysis data in the database.
type Analysis struct {
	ActorID        int                  `gorm:"index" json:"-"`
	DomainID       int                  `gorm:"index" json:"domainId"`
	PayloadHash    string               `gorm:"size:64" json:"-"`
	ClaimToken     string               `gorm:"size:64" json:"-"`
	ClaimUntil     time.Time            `json:"-"`
	FinishedAt     time.Time            `json:"-"`
	Input          types.Normalized     `gorm:"serializer:json;type:text" json:"-"`
	Targets        []types.TargetRef    `gorm:"serializer:json;type:text" json:"-"`
	ID             int                  `gorm:"primaryKey" json:"id"`
	RequestID      int                  `gorm:"index;uniqueIndex:ux_firewall_analysis,priority:1" json:"requestId"`
	RequestKey     string               `gorm:"size:128;uniqueIndex:ux_firewall_analysis,priority:2" json:"-"`
	RequestVersion int64                `json:"requestVersion"`
	Status         string               `gorm:"size:32;index" json:"status"`
	At             time.Time            `json:"at"`
	Result         types.AnalysisResult `gorm:"serializer:json;type:text" json:"result"`
	ErrorCode      string               `gorm:"size:64" json:"errorCode"`
	Stale          bool                 `json:"stale"`
	CreatedAt      time.Time            `json:"createdAt"`
	UpdatedAt      time.Time            `json:"updatedAt"`
}

// AnalysisTarget stores analysis target data in the database.
type AnalysisTarget struct {
	ID         int `gorm:"primaryKey" json:"-"`
	AnalysisID int `gorm:"index;uniqueIndex:ux_firewall_analysis_target,priority:1" json:"-"`
	InstanceID int `gorm:"index;uniqueIndex:ux_firewall_analysis_target,priority:2" json:"-"`
}

// TableName returns the database table used for this record.
func (AnalysisTarget) TableName() string { return "sys_firewall_analysis_target" }

// AnalysisResult stores analysis result data in the database.
type AnalysisResult struct {
	ID         int                `gorm:"primaryKey" json:"id"`
	AnalysisID int                `gorm:"index" json:"analysisId"`
	Position   int                `json:"position"`
	Item       types.AnalysisItem `gorm:"serializer:json;type:text" json:"item"`
}

// Draft stores draft data in the database.
type Draft struct {
	ActorID     int               `gorm:"index" json:"-"`
	PayloadHash string            `gorm:"size:64" json:"-"`
	ClaimToken  string            `gorm:"size:64" json:"-"`
	ClaimUntil  time.Time         `json:"-"`
	FinishedAt  time.Time         `json:"-"`
	ID          int               `gorm:"primaryKey" json:"id"`
	RequestID   int               `gorm:"index" json:"requestId"`
	AnalysisID  int               `gorm:"index;uniqueIndex:ux_firewall_draft,priority:1" json:"analysisId"`
	RequestKey  string            `gorm:"size:128;uniqueIndex:ux_firewall_draft,priority:2" json:"-"`
	Title       string            `gorm:"size:255" json:"title"`
	Version     int64             `json:"version"`
	Status      string            `gorm:"size:32;index" json:"status"`
	Result      types.DraftResult `gorm:"serializer:json;type:text" json:"result"`
	RowKeys     []string          `gorm:"serializer:json;type:text" json:"rowKeys"`
	Notes       string            `gorm:"type:text" json:"notes"`
	Stale       bool              `json:"stale"`
	ErrorCode   string            `gorm:"size:64" json:"errorCode"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

// TableName returns the database table used for this record.
func (Domain) TableName() string { return "sys_firewall_domain" }

// TableName returns the database table used for this record.
func (Instance) TableName() string { return "sys_firewall_instance" }

// TableName returns the database table used for this record.
func (ScopeBinding) TableName() string { return "sys_firewall_scope_binding" }

// TableName returns the database table used for this record.
func (ZoneBinding) TableName() string { return "sys_firewall_zone_binding" }

// TableName returns the database table used for this record.
func (NetworkBinding) TableName() string { return "sys_firewall_network_binding" }

// TableName returns the database table used for this record.
func (Import) TableName() string { return "sys_firewall_import" }

// TableName returns the database table used for this record.
func (Snapshot) TableName() string { return "sys_firewall_snapshot" }

// TableName returns the database table used for this record.
func (Coverage) TableName() string { return "sys_firewall_coverage" }

// TableName returns the database table used for this record.
func (Interface) TableName() string { return "sys_firewall_interface" }

// TableName returns the database table used for this record.
func (Route) TableName() string { return "sys_firewall_route" }

// TableName returns the database table used for this record.
func (Zone) TableName() string { return "sys_firewall_zone" }

// TableName returns the database table used for this record.
func (Address) TableName() string { return "sys_firewall_address" }

// TableName returns the database table used for this record.
func (AddressGroup) TableName() string { return "sys_firewall_address_group" }

// TableName returns the database table used for this record.
func (Service) TableName() string { return "sys_firewall_service" }

// TableName returns the database table used for this record.
func (ServiceGroup) TableName() string { return "sys_firewall_service_group" }

// TableName returns the database table used for this record.
func (Schedule) TableName() string { return "sys_firewall_schedule" }

// TableName returns the database table used for this record.
func (Rule) TableName() string { return "sys_firewall_rule" }

// TableName returns the database table used for this record.
func (RuleBinding) TableName() string { return "sys_firewall_rule_binding" }

// TableName returns the database table used for this record.
func (StaticNAT) TableName() string { return "sys_firewall_static_nat" }

// TableName returns the database table used for this record.
func (DynamicNAT) TableName() string { return "sys_firewall_dynamic_nat" }

// TableName returns the database table used for this record.
func (NATPool) TableName() string { return "sys_firewall_nat_pool" }

// TableName returns the database table used for this record.
func (Reference) TableName() string { return "sys_firewall_reference" }

// TableName returns the database table used for this record.
func (AddressRange) TableName() string { return "sys_firewall_address_range" }

// TableName returns the database table used for this record.
func (ServiceTerm) TableName() string { return "sys_firewall_service_term" }

// TableName returns the database table used for this record.
func (Request) TableName() string { return "sys_firewall_request" }

// TableName returns the database table used for this record.
func (RequestRow) TableName() string { return "sys_firewall_request_row" }

// TableName returns the database table used for this record.
func (Analysis) TableName() string { return "sys_firewall_analysis" }

// TableName returns the database table used for this record.
func (AnalysisResult) TableName() string { return "sys_firewall_analysis_result" }

// TableName returns the database table used for this record.
func (Draft) TableName() string { return "sys_firewall_draft" }

// Schema stores schema data in the database.
func Schema() []any {
	return []any{&Domain{}, &Instance{}, &ScopeBinding{}, &ZoneBinding{}, &NetworkBinding{}, &Import{}, &Snapshot{}, &Coverage{}, &Interface{}, &Route{}, &Zone{}, &Address{}, &AddressGroup{}, &Service{}, &ServiceGroup{}, &Schedule{}, &Rule{}, &RuleBinding{}, &StaticNAT{}, &DynamicNAT{}, &NATPool{}, &Reference{}, &AddressRange{}, &ServiceTerm{}, &Request{}, &RequestRow{}, &Analysis{}, &AnalysisResult{}, &AnalysisTarget{}, &Draft{}, &CollectionBatch{}, &CollectionTarget{}}
}
