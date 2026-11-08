package dto

import (
	network "go-admin/app/network/models"
	"go-admin/app/wireless/models"
	"time"
)

type WirelessCoverage struct {
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	State          string     `json:"state"`
	Reason         string     `json:"reason"`
	Version        int64      `json:"version"`
	ObservedAt     *time.Time `json:"observedAt"`
	LastCompleteAt *time.Time `json:"lastCompleteAt"`
}
type WirelessFields struct {
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
type WirelessResource struct {
	models.Resource
	Fields         WirelessFields   `json:"fields"`
	Coverage       WirelessCoverage `json:"coverage"`
	Health         string           `json:"health"`
	ControllerName string           `json:"controllerName"`
}
type WirelessController struct {
	Device              network.Device              `json:"device"`
	Platform            string                      `json:"platform"`
	SupportedDimensions []string                    `json:"supportedDimensions"`
	DefaultDimensions   []string                    `json:"defaultDimensions"`
	Coverage            []WirelessCoverage          `json:"coverage"`
	Counts              map[string]int              `json:"counts"`
	Candidates          map[string][]WirelessFields `json:"candidates"`
}
type WirelessSuggestion struct {
	Dimensions []string `json:"dimensions"`
	APNames    []string `json:"apNames"`
}
type WirelessOutcome struct {
	ResourceID int    `json:"resourceId"`
	Kind       string `json:"kind"`
	APName     string `json:"apName"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
}
type WirelessIssueGroup struct {
	Key           string             `json:"key"`
	Check         string             `json:"check"`
	SSID          string             `json:"ssid"`
	Band          string             `json:"band"`
	Affected      int                `json:"affected"`
	Healthy       int                `json:"healthy"`
	Unknown       int                `json:"unknown"`
	NotApplicable int                `json:"notApplicable"`
	Outcomes      []WirelessOutcome  `json:"outcomes"`
	Suggestion    WirelessSuggestion `json:"suggestion"`
}
type WirelessDistribution struct {
	SSID  string `json:"ssid"`
	Count int    `json:"count"`
}
type WirelessAnalysis struct {
	ControllerID int                    `json:"controllerId"`
	Groups       []WirelessIssueGroup   `json:"groups"`
	SSIDs        []WirelessDistribution `json:"ssids"`
	Coverage     []WirelessCoverage     `json:"coverage"`
}
type WirelessNeighbor struct {
	EvidenceID     string           `json:"evidenceId"`
	APName         string           `json:"apName"`
	SystemName     string           `json:"systemName"`
	ManagementIP   string           `json:"managementIp"`
	LocalInterface string           `json:"localInterface"`
	PortID         string           `json:"portId"`
	ChassisID      string           `json:"chassisId"`
	Candidates     []WirelessSwitch `json:"candidates"`
}
type WirelessSwitch struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	IP   string `json:"ip"`
}
type WirelessAPDetail struct {
	Resource         WirelessResource   `json:"resource"`
	Neighbors        []WirelessNeighbor `json:"neighbors"`
	NeighborCoverage WirelessCoverage   `json:"neighborCoverage"`
	Radios           []WirelessResource `json:"radios"`
	BSSes            []WirelessResource `json:"bsses"`
	Clients          []WirelessResource `json:"clients"`
	Counts           map[string]int64   `json:"counts"`
}
