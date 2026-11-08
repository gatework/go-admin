package dto

// CandidatesReq checks actual device evidence without creating an executable order.
// A nonempty target set narrows an issue's scope; omission evaluates current Leaves.
// An explicitly empty set is invalid, so a resolved issue cannot broaden its scope.
type CandidatesReq struct {
	SourceDeviceID  int    `json:"sourceDeviceId"`
	Kind            string `json:"kind"`
	VNI             int    `json:"vni"`
	Direction       string `json:"direction,omitempty"`
	TargetDeviceIDs []int  `json:"targetDeviceIds,omitempty"`
}
type Candidate struct {
	DeviceID int    `json:"deviceId"`
	Name     string `json:"name"`
	IP       string `json:"ip"`
	Reason   string `json:"reason"`
}
type Candidates struct {
	SourceDeviceID int         `json:"sourceDeviceId"`
	Kind           string      `json:"kind"`
	VNI            int         `json:"vni"`
	Direction      string      `json:"direction,omitempty"`
	Ready          []Candidate `json:"ready"`
	Unchanged      []Candidate `json:"unchanged"`
	Blocked        []Candidate `json:"blocked"`
}
type Remediation struct {
	Kind            string `json:"kind"`
	VNI             int    `json:"vni"`
	Direction       string `json:"direction,omitempty"`
	TargetDeviceIDs []int  `json:"targetDeviceIds"`
}
