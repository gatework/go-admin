// Package types contains the framework-independent firewall workbench contract.
package types

import (
	"encoding/json"
	"time"

	fm "github.com/gatework/firemon"
)

// InputRow describes input row in the firewall request and evidence protocol.
type InputRow struct {
	Key         string `json:"key"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Service     string `json:"service"`
	Notes       string `json:"notes"`
}

// Columns describes columns in the firewall request and evidence protocol.
type Columns struct {
	Source      int  `json:"source"`
	Destination int  `json:"destination"`
	Service     int  `json:"service"`
	Notes       *int `json:"notes,omitempty"`
}

// Input describes input in the firewall request and evidence protocol.
type Input struct {
	Rows            []InputRow `json:"rows"`
	Text            string     `json:"text,omitempty"`
	Columns         *Columns   `json:"columns,omitempty"`
	DefaultProtocol string     `json:"defaultProtocol"`
}

// Notice describes notice in the firewall request and evidence protocol.
type Notice struct {
	Field string `json:"field"`
	Token string `json:"token"`
	Code  string `json:"code"`
	Value string `json:"value,omitempty"`
}

// Term describes term in the firewall request and evidence protocol.
type Term struct {
	Value string `json:"value"`
	Kind  string `json:"kind"`
}

// NormalizedRow describes normalized row in the firewall request and evidence protocol.
type NormalizedRow struct {
	InputRow
	Line         int      `json:"line"`
	Sources      []Term   `json:"sources"`
	Destinations []Term   `json:"destinations"`
	Services     []Term   `json:"services"`
	Corrections  []Notice `json:"corrections"`
	Issues       []Notice `json:"issues"`
	Status       string   `json:"status"`
}

// Normalized describes normalized in the firewall request and evidence protocol.
type Normalized struct {
	Rows            []NormalizedRow `json:"rows"`
	DefaultProtocol string          `json:"defaultProtocol"`
	Issues          []Notice        `json:"issues"`
}

// Binding describes binding in the firewall request and evidence protocol.
type Binding struct {
	Zone         string   `json:"zone"`
	Scope        string   `json:"scope"`
	RoutingScope string   `json:"routingScope"`
	DomainID     int      `json:"domainId"`
	Protected    bool     `json:"protected"`
	Networks     []string `json:"networks"`
}

// ScopeBinding describes scope binding in the firewall request and evidence protocol.
type ScopeBinding struct {
	RoutingScope string `json:"routingScope"`
	DomainID     int    `json:"domainId"`
}

// Settings describes settings in the firewall request and evidence protocol.
type Settings struct {
	Version int64          `json:"version"`
	Zones   []Binding      `json:"zones"`
	Scopes  []ScopeBinding `json:"scopes"`
}

// Target has already passed the host's inventory authorization check.
type Target struct {
	InstanceID      int
	Name            string
	Revision        string
	DomainID        int
	Mode            string
	BindingsVersion int64
	Settings        Settings
	Firewall        *fm.Firewall
}

// TargetRef is the durable, serializable input of an analysis task.
type TargetRef struct {
	InstanceID      int      `json:"instanceId"`
	Name            string   `json:"name"`
	Revision        string   `json:"revision"`
	DomainID        int      `json:"domainId"`
	Mode            string   `json:"mode"`
	BindingsVersion int64    `json:"bindingsVersion"`
	Settings        Settings `json:"settings"`
}

// Coverage describes coverage in the firewall request and evidence protocol.
type Coverage struct {
	InstanceID      int    `json:"instanceId"`
	DomainID        int    `json:"domainId"`
	Revision        string `json:"revision"`
	BindingsVersion int64  `json:"bindingsVersion"`
	Zone            string `json:"zone"`
	Scope           string `json:"scope"`
	RoutingScope    string `json:"routingScope"`
	Network         string `json:"network"`
	Protected       bool   `json:"protected"`
	Origin          string `json:"origin"`
	ObjectID        string `json:"objectId,omitempty"`
	Known           bool   `json:"known"`
}

// TrafficPart describes traffic part in the firewall request and evidence protocol.
type TrafficPart struct {
	Source          string `json:"source"`
	Destination     string `json:"destination"`
	Protocol        string `json:"protocol"`
	SourcePort      string `json:"sourcePort"`
	DestinationPort string `json:"destinationPort"`
	ICMPType        string `json:"icmpType,omitempty"`
	ICMPCode        string `json:"icmpCode,omitempty"`
}

// RuleTrace describes rule trace in the firewall request and evidence protocol.
type RuleTrace struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Action  string        `json:"action"`
	Certain bool          `json:"certain"`
	Traffic []TrafficPart `json:"traffic"`
}

// NATStep describes natstep in the firewall request and evidence protocol.
type NATStep struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Reverse bool   `json:"reverse"`
}

// NATTrace describes nattrace in the firewall request and evidence protocol.
type NATTrace struct {
	ID                 string        `json:"id"`
	Kind               string        `json:"kind"`
	Exact              bool          `json:"exact"`
	PossibleAllocation bool          `json:"possibleAllocation"`
	Original           []TrafficPart `json:"original"`
	Translated         []TrafficPart `json:"translated"`
	Steps              []NATStep     `json:"steps"`
}

// AnalysisItem describes analysis item in the firewall request and evidence protocol.
type AnalysisItem struct {
	RowKey        string        `json:"rowKey"`
	InstanceID    int           `json:"instanceId"`
	InstanceName  string        `json:"instanceName"`
	Revision      string        `json:"revision"`
	Scope         string        `json:"scope"`
	RoutingScope  string        `json:"routingScope"`
	FromZone      string        `json:"fromZone"`
	ToZone        string        `json:"toZone"`
	HitSide       string        `json:"hitSide"`
	Basis         string        `json:"basis"`
	Status        string        `json:"status"`
	Original      []TrafficPart `json:"original"`
	PolicyView    string        `json:"policyView"`
	PacketStatus  string        `json:"packetStatus"`
	PacketUnknown []TrafficPart `json:"packetUnknown"`
	Allowed       []TrafficPart `json:"allowed"`
	Denied        []TrafficPart `json:"denied"`
	Unmatched     []TrafficPart `json:"unmatched"`
	Unknown       []TrafficPart `json:"unknown"`
	Trace         []RuleTrace   `json:"trace"`
	NAT           []NATTrace    `json:"nat"`
	Issues        []Notice      `json:"issues"`
}

// AnalysisResult describes analysis result in the firewall request and evidence protocol.
type AnalysisResult struct {
	Items    []AnalysisItem `json:"items"`
	Issues   []Notice       `json:"issues"`
	Complete bool           `json:"complete"`
}

// DraftChange describes draft change in the firewall request and evidence protocol.
type DraftChange struct {
	Kind        string          `json:"kind"`
	RuleID      string          `json:"ruleId"`
	Description string          `json:"description"`
	Data        json.RawMessage `json:"data,omitempty"`
}

// DraftItem describes draft item in the firewall request and evidence protocol.
type DraftItem struct {
	InstanceID       int           `json:"instanceId"`
	Revision         string        `json:"revision"`
	RowKeys          []string      `json:"rowKeys"`
	SettingsVersion  int64         `json:"settingsVersion"`
	Status           string        `json:"status"`
	BaseDigest       string        `json:"baseDigest"`
	ResultDigest     string        `json:"resultDigest"`
	Changes          []DraftChange `json:"changes"`
	Issues           []Notice      `json:"issues"`
	TargetSatisfied  bool          `json:"targetSatisfied"`
	OutsidePreserved bool          `json:"outsidePreserved"`
	NATUnchanged     bool          `json:"natUnchanged"`
}

// DraftResult describes draft result in the firewall request and evidence protocol.
type DraftResult struct {
	Items    []DraftItem `json:"items"`
	Complete bool        `json:"complete"`
	At       time.Time   `json:"at"`
}
