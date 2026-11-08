package dto

import (
	common "github.com/go-admin-team/go-admin-core/v2/sdk/contract/dto"
	"go-admin/app/fabric/models"
	"time"
)

type PageReq struct {
	common.Pagination
	Keyword  string `form:"keyword"`
	Status   string `form:"status"`
	FabricID int    `form:"fabricId"`
	VRF      string `form:"vrf"`
}
type RulesReq struct {
	Version int64         `json:"version"`
	Rules   []models.Rule `json:"rules"`
}
type MemberReq struct {
	FabricID *int   `json:"fabricId"`
	Mode     string `json:"mode"`
}
type VersionReq struct {
	Version int64 `json:"version"`
}
type SettingsReq struct {
	Version           int64 `json:"version"`
	CollectionEnabled bool  `json:"collectionEnabled"`
	RuntimeMinutes    int   `json:"runtimeMinutes"`
	ConfigMinutes     int   `json:"configMinutes"`
}
type Evidence struct {
	Latest       *models.Observation `json:"latest"`
	LastComplete *models.Observation `json:"lastComplete"`
	State        string              `json:"state"`
	Stale        bool                `json:"stale"`
}
type Member struct {
	models.Membership
	Name       string `json:"name"`
	IP         string `json:"ip"`
	Role       string `json:"role"`
	Candidates []int  `json:"candidates"`
}
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
type SegmentCell struct {
	DeviceID      int            `json:"deviceId"`
	State         string         `json:"state"`
	Configuration map[string]any `json:"configuration"`
	ACLIn         string         `json:"aclIn"`
	ACLOut        string         `json:"aclOut"`
}
type Segment struct {
	VNI   int           `json:"vni"`
	VRF   string        `json:"vrf"`
	State string        `json:"state"`
	Cells []SegmentCell `json:"cells"`
}
type VPCGroup struct {
	Key       string `json:"key"`
	Domain    string `json:"domain"`
	DeviceIDs []int  `json:"deviceIds"`
	State     string `json:"state"`
}
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
type Detail struct {
	Summary
	Nodes       []Node       `json:"nodes"`
	Segments    []Segment    `json:"segments"`
	VPCGroups   []VPCGroup   `json:"vpcGroups"`
	Findings    []Finding    `json:"findings"`
	EvidenceKey string       `json:"evidenceKey"`
	IssueGroups []IssueGroup `json:"issueGroups"`
}
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
