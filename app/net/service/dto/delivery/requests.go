package delivery

import "github.com/gatework/motor/app/net/models/connection"

// InputTarget carries the input target fields exchanged by delivery operations.
type InputTarget struct {
	DeviceID           int     `json:"deviceId"`
	Script             string  `json:"script"`
	ExpectedProfile    *string `json:"expectedProfile,omitempty"`
	ExpectedDeviceRole *string `json:"expectedDeviceRole,omitempty"`
	ExpectedDeviceType *string `json:"expectedDeviceType,omitempty"`
}

// Create carries the create fields exchanged by delivery operations.
type Create struct {
	RequestKey string        `json:"requestKey"`
	Title      string        `json:"title"`
	Targets    []InputTarget `json:"targets"`
}

// Edit carries the edit fields exchanged by delivery operations.
type Edit struct {
	Version int64         `json:"version"`
	Title   string        `json:"title"`
	Targets []InputTarget `json:"targets"`
}

// Action carries the action fields exchanged by delivery operations.
type Action struct {
	Version    int64  `json:"version"`
	RequestKey string `json:"requestKey,omitempty"`
}

// Connection carries the connection fields exchanged by delivery operations.
type Connection = connection.Input

// Page carries the page fields exchanged by delivery operations.
type Page struct {
	Keyword   string `form:"keyword"`
	Status    string `form:"status"`
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
}
