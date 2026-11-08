package dto

import "time"

// IssueDevice is an assessment outcome, never inferred from absent findings.
type IssueDevice struct {
	DeviceID       int        `json:"deviceId"`
	Status         string     `json:"status"`
	Reason         string     `json:"reason"`
	Value          string     `json:"value"`
	ObservedAt     *time.Time `json:"observedAt,omitempty"`
	EvidenceIDs    []int      `json:"evidenceIds"`
	FindingIndices []int      `json:"findingIndices"`
}
type IssueCounts struct {
	Affected      int `json:"affected"`
	Healthy       int `json:"healthy"`
	Unknown       int `json:"unknown"`
	NotApplicable int `json:"notApplicable"`
}

// IssueGroup scopes one check to a business context. Keys are opaque and stable;
// finding indices always address the containing Detail response only.
type IssueGroup struct {
	Key            string        `json:"key"`
	Scope          string        `json:"scope,omitempty"`
	Check          string        `json:"check"`
	Category       string        `json:"category"`
	Title          string        `json:"title"`
	Severity       string        `json:"severity"`
	VNI            int           `json:"vni,omitempty"`
	VRF            string        `json:"vrf,omitempty"`
	Direction      string        `json:"direction,omitempty"`
	CompleteScope  bool          `json:"completeScope"`
	Counts         IssueCounts   `json:"counts"`
	Devices        []IssueDevice `json:"devices"`
	FindingIndices []int         `json:"findingIndices"`
	Remediations   []Remediation `json:"remediations"`
}
