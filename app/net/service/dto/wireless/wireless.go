package wireless

import wireless "github.com/gatework/go-wireless"

// PageReq carries the page req fields exchanged by wireless operations.
type PageReq struct {
	ExactAP   bool   `form:"exactAp"`
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	Keyword   string `form:"keyword"`
	IP        string `form:"ip"`
	MAC       string `form:"mac"`
	AP        string `form:"ap"`
	SSID      string `form:"ssid"`
	VLAN      string `form:"vlan"`
	Band      string `form:"band"`
	Status    string `form:"status"`
	Sort      string `form:"sort"`
	Direction string `form:"direction"`
	GroupID   *int   `form:"groupId"`
}

// CollectionReq carries the collection req fields exchanged by wireless operations.
type CollectionReq struct {
	RequestKey string               `json:"requestKey"`
	Dimensions []wireless.Dimension `json:"dimensions"`
	APNames    []string             `json:"apNames"`
}

// ManagementReq carries the management req fields exchanged by wireless operations.
type ManagementReq struct {
	Version              int64  `json:"version"`
	LocalAlias           string `json:"localAlias"`
	InstallationLocation string `json:"installationLocation"`
	Owner                string `json:"owner"`
	Notes                string `json:"notes"`
}

// NeighborReq carries the neighbor req fields exchanged by wireless operations.
type NeighborReq struct {
	Version            int64  `json:"version"`
	ObservationVersion int64  `json:"observationVersion"`
	EvidenceID         string `json:"evidenceId"`
	DeviceID           *int   `json:"deviceId"`
	Interface          string `json:"interface"`
}

// ImportReq carries the import req fields exchanged by wireless operations.
type ImportReq struct {
	Content string `json:"content"`
	Digest  string `json:"digest,omitempty"`
}
