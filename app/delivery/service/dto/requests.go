package dto

import "go-admin/app/network/connection"

type InputTarget struct {
	DeviceID           int     `json:"deviceId"`
	Script             string  `json:"script"`
	ExpectedProfile    *string `json:"expectedProfile,omitempty"`
	ExpectedDeviceRole *string `json:"expectedDeviceRole,omitempty"`
	ExpectedDeviceType *string `json:"expectedDeviceType,omitempty"`
}
type Create struct {
	RequestKey string        `json:"requestKey"`
	Title      string        `json:"title"`
	Targets    []InputTarget `json:"targets"`
}
type Edit struct {
	Version int64         `json:"version"`
	Title   string        `json:"title"`
	Targets []InputTarget `json:"targets"`
}
type Action struct {
	Version    int64  `json:"version"`
	RequestKey string `json:"requestKey,omitempty"`
}
type Connection = connection.Input
type Page struct {
	Keyword   string `form:"keyword"`
	Status    string `form:"status"`
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
}
