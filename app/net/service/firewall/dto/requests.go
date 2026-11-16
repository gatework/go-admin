// Package dto holds the firewall workbench's request types: what handlers bind
// and the service accepts.
package dto

import (
	"time"

	"github.com/gatework/motor/app/net/models/firewall/types"
	"github.com/gatework/motor/common/paging"
)

// Page is a list query: page, keyword, and the device group and network
// domain to narrow to (nil group: every group; 0: ungrouped).
type Page struct {
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	Keyword   string `form:"keyword"`
	GroupID   *int   `form:"groupId"`
	DomainID  int    `form:"domainId"`
}

// Validate fills page defaults and rejects out-of-range values.
func (p *Page) Validate() error {
	if !paging.Normalize(&p.PageIndex, &p.PageSize) || len(p.Keyword) > 512 || p.DomainID < 0 || p.GroupID != nil && *p.GroupID < 0 {
		return types.InvalidErr
	}
	return nil
}

// InstanceInput registers a firewall device as a workbench instance. An empty
// Vendor is inferred from inventory.
type InstanceInput struct {
	DeviceID      int    `json:"deviceId"`
	DomainID      int    `json:"domainId"`
	Vendor        string `json:"vendor"`
	NativeContext string `json:"nativeContext"`
	Mode          string `json:"mode"`
}

// DomainInput creates a network domain, or renames one at Version.
type DomainInput struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Version int64  `json:"version"`
}

// CollectionInput queues a configuration collection from the given
// instances. RequestKey makes a retried submission idempotent.
type CollectionInput struct {
	RequestKey  string `json:"requestKey"`
	Kind        string `json:"kind"`
	InstanceIDs []int  `json:"instanceIds"`
}

// ObjectQuery filters a snapshot's objects. Address, protocol and port find
// objects that match a flow; zone and state narrow by placement and status.
type ObjectQuery struct {
	Page
	Address  string `form:"address"`
	Protocol string `form:"protocol"`
	Port     string `form:"port"`
	Zone     string `form:"zone"`
	State    string `form:"state"`
}

// ImportInput is a configuration a user uploads in place of collecting it.
// ExpectedRevision guards against importing over a newer snapshot.
type ImportInput struct {
	RequestKey       string     `json:"requestKey"`
	ExpectedRevision string     `json:"expectedRevision"`
	Text             string     `json:"text"`
	Filename         string     `json:"filename"`
	Complete         bool       `json:"complete"`
	CapturedAt       *time.Time `json:"capturedAt,omitempty"`
}

// DraftInput asks for a policy draft from an analysis, optionally limited to
// some of its rows.
type DraftInput struct {
	RequestKey string   `json:"requestKey"`
	AnalysisID int      `json:"analysisId"`
	Title      string   `json:"title"`
	RowKeys    []string `json:"rowKeys,omitempty"`
}

// DraftEdit changes a draft's title and notes at Version.
type DraftEdit struct {
	Version int64  `json:"version"`
	Title   string `json:"title"`
	Notes   string `json:"notes"`
}

// RequestInput saves an access request: the flows to check, at Version.
type RequestInput struct {
	Version  int64       `json:"version"`
	Title    string      `json:"title"`
	DomainID int         `json:"domainId"`
	Input    types.Input `json:"input"`
}

// JobInput starts a job (an analysis) for a request at its Version.
type JobInput struct {
	RequestKey string `json:"requestKey"`
	Version    int64  `json:"version"`
}
