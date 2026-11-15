package wireless

import (
	"time"

	wireless "github.com/gatework/go-wireless"
)

// Dimension retains the latest attempt separately from authoritative complete facts.
// Neither observations nor internal source identities are HTTP response models.
type Dimension struct {
	ID                 int                   `gorm:"primaryKey" json:"-"`
	ControllerID       int                   `gorm:"uniqueIndex:ux_wireless_dimension,priority:1" json:"controllerId"`
	Name               string                `gorm:"size:40;uniqueIndex:ux_wireless_dimension,priority:2" json:"name"`
	Version            int64                 `json:"version"`
	SourceHash         string                `gorm:"size:64" json:"-"`
	CompleteSourceHash string                `gorm:"size:64" json:"-"`
	Latest             wireless.Observation  `gorm:"serializer:json;type:text" json:"-"`
	LastComplete       *wireless.Observation `gorm:"serializer:json;type:text" json:"-"`
	ObservedAt         time.Time             `json:"observedAt"`
	CompleteAt         *time.Time            `json:"lastCompleteAt"`
	ProjectionStatus   string                `gorm:"size:20" json:"projectionStatus"`
	ProjectionIssues   []string              `gorm:"serializer:json;type:text" json:"projectionIssues"`
}

// TableName returns the stable database table for Dimension.
func (Dimension) TableName() string { return "sys_wireless_dimension" }

// Resource is scoped by controller and dimension. Missing APs remain durable
// inventory objects so rediscovery never discards human management fields.
type Resource struct {
	ID                      int             `gorm:"primaryKey" json:"id"`
	ControllerID            int             `gorm:"uniqueIndex:ux_wireless_resource,priority:1;index" json:"controllerId"`
	Kind                    string          `gorm:"size:40;uniqueIndex:ux_wireless_resource,priority:2" json:"kind"`
	Identity                string          `gorm:"size:64;uniqueIndex:ux_wireless_resource,priority:3" json:"-"`
	Facts                   wireless.Record `gorm:"serializer:json;type:text" json:"-"`
	SourceHash              string          `gorm:"size:64" json:"-"`
	ObservationVersion      int64           `json:"observationVersion"`
	ObservedAt              time.Time       `json:"observedAt"`
	Missing                 bool            `json:"missing"`
	ParentStatus            string          `gorm:"size:24" json:"parentStatus"`
	ParentResourceID        *int            `gorm:"index" json:"-"`
	Version                 int64           `json:"version"`
	LocalAlias              string          `gorm:"size:255" json:"localAlias"`
	InstallationLocation    string          `gorm:"size:255" json:"installationLocation"`
	Owner                   string          `gorm:"size:255" json:"owner"`
	Notes                   string          `gorm:"type:text" json:"notes"`
	NeighborObservedAt      *time.Time      `json:"neighborObservedAt"`
	NeighborStatus          string          `gorm:"size:24" json:"neighborStatus"`
	NeighborEvidenceVersion int64           `json:"neighborEvidenceVersion"`
	NeighborDeviceID        *int            `json:"neighborDeviceId"`
	NeighborInterface       string          `gorm:"size:255" json:"neighborInterface"`
	NeighborEvidenceID      string          `gorm:"size:64" json:"neighborEvidenceId"`
	NeighborVersion         int64           `json:"neighborVersion"`
	UpdatedAt               time.Time       `json:"updatedAt"`
}

// TableName returns the stable database table for Resource.
func (Resource) TableName() string { return "sys_wireless_resource" }

// Collection records collection state for wireless persistence.
type Collection struct {
	Results      []DimensionResult    `gorm:"serializer:json;type:text" json:"results"`
	ID           int                  `gorm:"primaryKey" json:"id"`
	ControllerID int                  `gorm:"index" json:"controllerId"`
	RequestKey   string               `gorm:"size:100;uniqueIndex:ux_wireless_request,priority:2" json:"requestKey"`
	CreateBy     int                  `gorm:"uniqueIndex:ux_wireless_request,priority:1" json:"createdBy"`
	PayloadHash  string               `gorm:"size:64" json:"-"`
	SourceHash   string               `gorm:"size:64" json:"-"`
	Dimensions   []wireless.Dimension `gorm:"serializer:json;type:text" json:"dimensions"`
	APNames      []string             `gorm:"serializer:json;type:text" json:"apNames"`
	Status       string               `gorm:"size:24;index" json:"status"`
	ErrorCode    string               `gorm:"size:64" json:"errorCode,omitempty"`
	Version      int64                `json:"version"`
	LeaseToken   string               `gorm:"size:40" json:"-"`
	LeaseUntil   *time.Time           `gorm:"index" json:"-"`
	CreatedAt    time.Time            `json:"createdAt"`
	StartedAt    *time.Time           `json:"startedAt"`
	FinishedAt   *time.Time           `json:"finishedAt"`
}

// TableName returns the stable database table for Collection.
func (Collection) TableName() string { return "sys_wireless_collection" }

// NeighborState records completeness for each explicitly selected AP.
type NeighborState struct {
	ID                 int    `gorm:"primaryKey"`
	ControllerID       int    `gorm:"uniqueIndex:ux_wireless_neighbor,priority:1"`
	APResourceID       int    `gorm:"uniqueIndex:ux_wireless_neighbor,priority:2"`
	APName             string `gorm:"size:255;index"`
	Version            int64
	SourceHash         string `gorm:"size:64"`
	CompleteSourceHash string `gorm:"size:64"`
	Status             string `gorm:"size:24"`
	ObservedAt         time.Time
	CompleteAt         *time.Time
	Records            []wireless.Record `gorm:"serializer:json;type:text"`
}

// TableName returns the stable database table for NeighborState.
func (NeighborState) TableName() string { return "sys_wireless_neighbor" }

// DimensionResult records dimension result state for wireless persistence.
type DimensionResult struct {
	Name   string   `json:"name"`
	Status string   `json:"status"`
	Count  int      `json:"count"`
	Issues []string `json:"issues"`
}

// Audit stores identifiers and versions, never uploaded CSV or device output.
type Audit struct {
	ID           int `gorm:"primaryKey"`
	ActorID      int
	ControllerID int
	ResourceID   int
	Action       string `gorm:"size:32"`
	Version      int64
	CreatedAt    time.Time
}

// TableName returns the stable database table for Audit.
func (Audit) TableName() string { return "sys_wireless_audit" }
