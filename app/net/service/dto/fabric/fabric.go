package fabric

import (
	"time"

	common "github.com/gatework/motor-core/sdk/contract/dto"
	models "github.com/gatework/motor/app/net/models/fabric"
)

// PageReq carries the page req fields exchanged by fabric operations.
type PageReq struct {
	common.Pagination
	Keyword  string `form:"keyword"`
	Status   string `form:"status"`
	FabricID int    `form:"fabricId"`
	VRF      string `form:"vrf"`
}

// RulesReq carries the rules req fields exchanged by fabric operations.
type RulesReq struct {
	Version int64         `json:"version"`
	Rules   []models.Rule `json:"rules"`
}

// MemberReq carries the member req fields exchanged by fabric operations.
type MemberReq struct {
	FabricID *int   `json:"fabricId"`
	Mode     string `json:"mode"`
}

// VersionReq carries the version req fields exchanged by fabric operations.
type VersionReq struct {
	Version int64 `json:"version"`
}

// SettingsReq carries the settings req fields exchanged by fabric operations.
type SettingsReq struct {
	Version           int64 `json:"version"`
	CollectionEnabled bool  `json:"collectionEnabled"`
	RuntimeMinutes    int   `json:"runtimeMinutes"`
	ConfigMinutes     int   `json:"configMinutes"`
}

// Evidence carries the evidence fields exchanged by fabric operations.
type Evidence struct {
	Latest       *models.Observation `json:"latest"`
	LastComplete *models.Observation `json:"lastComplete"`
	State        string              `json:"state"`
	Stale        bool                `json:"stale"`
}

// Member carries the member fields exchanged by fabric operations.
type Member struct {
	models.Membership
	Name       string `json:"name"`
	IP         string `json:"ip"`
	Role       string `json:"role"`
	Candidates []int  `json:"candidates"`
}

// Node carries the node fields exchanged by fabric operations.
type Node struct {
	DeviceID          int                 `json:"deviceId"`
	Name              string              `json:"name"`
	IP                string              `json:"ip"`
	Role              string              `json:"role"`
	Version           int64               `json:"version"`
	MembershipVersion int64               `json:"membershipVersion"`
	State             string              `json:"state"`
	ConfigState       string              `json:"configState"`
	Dimensions        map[string]Evidence `json:"dimensions"`
	VPCDomain         string              `json:"vpcDomain"`
	VPCPeer           string              `json:"vpcPeer"`
	VPCSource         string              `json:"vpcSource"`
	Model             string              `json:"model"`
	Platform          string              `json:"platform"`
	Supported         bool                `json:"supported"`
	Features          []string            `json:"features"`
	VTEP              string              `json:"vtep"`
}

// SegmentCell carries the segment cell fields exchanged by fabric operations.
type SegmentCell struct {
	DeviceID      int            `json:"deviceId"`
	State         string         `json:"state"`
	Configuration map[string]any `json:"configuration"`
	ACLIn         string         `json:"aclIn"`
	ACLOut        string         `json:"aclOut"`
}

// Segment carries the segment fields exchanged by fabric operations.
type Segment struct {
	VNI   int           `json:"vni"`
	VRF   string        `json:"vrf"`
	State string        `json:"state"`
	Cells []SegmentCell `json:"cells"`
}

// VPCGroup carries the v p c group fields exchanged by fabric operations.
type VPCGroup struct {
	Key       string `json:"key"`
	Domain    string `json:"domain"`
	DeviceIDs []int  `json:"deviceIds"`
	State     string `json:"state"`
}

// Finding carries the finding fields exchanged by fabric operations.
type Finding struct {
	// Scope is business only when VRF/VNI attribution is resolved; otherwise fabric.
	Scope       string       `json:"scope,omitempty"`
	VRF         string       `json:"vrf,omitempty"`
	Check       string       `json:"check,omitempty"`
	Direction   string       `json:"direction,omitempty"`
	Remediation *Remediation `json:"remediation,omitempty"`
	DeviceID    int          `json:"deviceId,omitempty"`
	VNI         int          `json:"vni,omitempty"`
	Code        string       `json:"code"`
	Role        string       `json:"role,omitempty"`
	Severity    string       `json:"severity"`
	Message     string       `json:"message"`
	Evidence    any          `json:"evidence"`
}

// Summary carries the summary fields exchanged by fabric operations.
type Summary struct {
	models.Fabric
	MemberCount      int        `json:"memberCount"`
	LeafCount        int        `json:"leafCount"`
	State            string     `json:"state"`
	CompleteScope    bool       `json:"completeScope"`
	AssessmentID     int        `json:"assessmentId"`
	FindingCount     int        `json:"findingCount"`
	IssueCount       int        `json:"issueCount"`
	UnknownCount     int        `json:"unknownCount"`
	ObservedAt       *time.Time `json:"observedAt"`
	EvidenceCoverage int        `json:"evidenceCoverage"`
	CheckedCount     int        `json:"checkedCount"`
	ApplicableCount  int        `json:"applicableCount"`
}

// Detail carries the detail fields exchanged by fabric operations.
type Detail struct {
	Summary
	Nodes       []Node       `json:"nodes"`
	Segments    []Segment    `json:"segments"`
	VPCGroups   []VPCGroup   `json:"vpcGroups"`
	Findings    []Finding    `json:"findings"`
	EvidenceKey string       `json:"evidenceKey"`
	IssueGroups []IssueGroup `json:"issueGroups"`
}

// Endpoint carries the endpoint fields exchanged by fabric operations.
type Endpoint struct {
	FabricID       int       `json:"fabricId"`
	FabricCode     string    `json:"fabricCode"`
	DeviceID       int       `json:"deviceId"`
	DeviceName     string    `json:"deviceName"`
	VRF            string    `json:"vrf"`
	IP             string    `json:"ip"`
	MAC            string    `json:"mac"`
	VLAN           int       `json:"vlan"`
	VNI            int       `json:"vni"`
	Interface      string    `json:"interface"`
	RemoteVTEP     string    `json:"remoteVtep"`
	Kind           string    `json:"kind"`
	State          string    `json:"state"`
	ObservedAt     time.Time `json:"observedAt"`
	EvidenceID     int       `json:"evidenceId"`
	VPCID          string    `json:"vpcId,omitempty"`
	VPCGroupKey    string    `json:"vpcGroupKey,omitempty"`
	CandidateCount int       `json:"candidateCount"`
}
