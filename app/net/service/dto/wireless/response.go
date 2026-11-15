package wireless

import (
	"time"

	network "github.com/gatework/motor/app/net/models/inventory"
	models "github.com/gatework/motor/app/net/models/wireless"
)

// Coverage carries the wireless coverage fields exchanged by wireless operations.
type Coverage struct {
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	State          string     `json:"state"`
	Reason         string     `json:"reason"`
	Version        int64      `json:"version"`
	ObservedAt     *time.Time `json:"observedAt"`
	LastCompleteAt *time.Time `json:"lastCompleteAt"`
}

// Fields carries the wireless fields fields exchanged by wireless operations.
type Fields struct {
	Name        string   `json:"name"`
	APID        string   `json:"apId"`
	APName      string   `json:"apName"`
	RadioID     string   `json:"radioId"`
	BSSID       string   `json:"bssid"`
	SSID        string   `json:"ssid"`
	VLAN        string   `json:"vlan"`
	Band        string   `json:"band"`
	MAC         string   `json:"mac"`
	IP          string   `json:"ip"`
	IPv6        string   `json:"ipv6"`
	Model       string   `json:"model"`
	Serial      string   `json:"serial"`
	Status      string   `json:"status"`
	AdminStatus string   `json:"adminStatus"`
	Protocol    string   `json:"protocol"`
	Channel     string   `json:"channel"`
	ClientCount *float64 `json:"clientCount"`
	Signal      *float64 `json:"signal"`
	SNR         *float64 `json:"snr"`
}

// Resource carries the wireless resource fields exchanged by wireless operations.
type Resource struct {
	models.Resource
	Fields         Fields   `json:"fields"`
	Coverage       Coverage `json:"coverage"`
	Health         string   `json:"health"`
	ControllerName string   `json:"controllerName"`
}

// Controller carries the wireless controller fields exchanged by wireless operations.
type Controller struct {
	Device              network.Device      `json:"device"`
	Platform            string              `json:"platform"`
	SupportedDimensions []string            `json:"supportedDimensions"`
	DefaultDimensions   []string            `json:"defaultDimensions"`
	Coverage            []Coverage          `json:"coverage"`
	Counts              map[string]int      `json:"counts"`
	Candidates          map[string][]Fields `json:"candidates"`
}

// Suggestion carries the wireless suggestion fields exchanged by wireless operations.
type Suggestion struct {
	Dimensions []string `json:"dimensions"`
	APNames    []string `json:"apNames"`
}

// Outcome carries the wireless outcome fields exchanged by wireless operations.
type Outcome struct {
	ResourceID int    `json:"resourceId"`
	Kind       string `json:"kind"`
	APName     string `json:"apName"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
}

// IssueGroup carries the wireless issue group fields exchanged by wireless operations.
type IssueGroup struct {
	Key           string     `json:"key"`
	Check         string     `json:"check"`
	SSID          string     `json:"ssid"`
	Band          string     `json:"band"`
	Affected      int        `json:"affected"`
	Healthy       int        `json:"healthy"`
	Unknown       int        `json:"unknown"`
	NotApplicable int        `json:"notApplicable"`
	Outcomes      []Outcome  `json:"outcomes"`
	Suggestion    Suggestion `json:"suggestion"`
}

// Distribution carries the wireless distribution fields exchanged by wireless operations.
type Distribution struct {
	SSID  string `json:"ssid"`
	Count int    `json:"count"`
}

// Analysis carries the wireless analysis fields exchanged by wireless operations.
type Analysis struct {
	ControllerID int            `json:"controllerId"`
	Groups       []IssueGroup   `json:"groups"`
	SSIDs        []Distribution `json:"ssids"`
	Coverage     []Coverage     `json:"coverage"`
}

// Neighbor carries the wireless neighbor fields exchanged by wireless operations.
type Neighbor struct {
	EvidenceID     string   `json:"evidenceId"`
	APName         string   `json:"apName"`
	SystemName     string   `json:"systemName"`
	ManagementIP   string   `json:"managementIp"`
	LocalInterface string   `json:"localInterface"`
	PortID         string   `json:"portId"`
	ChassisID      string   `json:"chassisId"`
	Candidates     []Switch `json:"candidates"`
}

// Switch carries the wireless switch fields exchanged by wireless operations.
type Switch struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	IP   string `json:"ip"`
}

// APDetail carries the wireless a p detail fields exchanged by wireless operations.
type APDetail struct {
	Resource         Resource         `json:"resource"`
	Neighbors        []Neighbor       `json:"neighbors"`
	NeighborCoverage Coverage         `json:"neighborCoverage"`
	Radios           []Resource       `json:"radios"`
	BSSes            []Resource       `json:"bsses"`
	Clients          []Resource       `json:"clients"`
	Counts           map[string]int64 `json:"counts"`
}
