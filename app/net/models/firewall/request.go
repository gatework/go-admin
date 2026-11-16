package firewall

import (
	"time"

	"github.com/gatework/motor/app/net/models/firewall/types"
)

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

// TableName returns the database table used for this record.
func (Request) TableName() string { return "sys_firewall_request" }

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

// TableName returns the database table used for this record.
func (RequestRow) TableName() string { return "sys_firewall_request_row" }

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

// TableName returns the database table used for this record.
func (Analysis) TableName() string { return "sys_firewall_analysis" }

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

// TableName returns the database table used for this record.
func (AnalysisResult) TableName() string { return "sys_firewall_analysis_result" }

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
func (Draft) TableName() string { return "sys_firewall_draft" }
