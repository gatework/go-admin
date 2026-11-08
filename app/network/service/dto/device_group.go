package dto

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"go-admin/app/network"
	"go-admin/app/network/models"
)

type DeviceGroupNode struct {
	models.DeviceGroup
	DirectDeviceCount int64             `json:"directDeviceCount"`
	DeviceCount       int64             `json:"deviceCount"`
	Children          []DeviceGroupNode `json:"children"`
}
type DeviceGroupTree struct {
	Items                []DeviceGroupNode `json:"items"`
	TotalDeviceCount     int64             `json:"totalDeviceCount"`
	UngroupedDeviceCount int64             `json:"ungroupedDeviceCount"`
}
type DeviceGroupReq struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	ParentID    *int   `json:"parentId"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Position    int    `json:"position"`
	Version     int64  `json:"version"`
}

var groupCode = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)

func (r *DeviceGroupReq) Validate(update bool) error {
	r.Code = strings.ToLower(strings.TrimSpace(r.Code))
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	if (update && r.Version <= 0) || r.Position < 0 || (r.ParentID != nil && *r.ParentID <= 0) || len(r.Code) > 100 || !groupCode.MatchString(r.Code) || r.Name == "" {
		return network.ErrInvalidInput
	}
	for _, field := range []struct {
		value string
		max   int
	}{{r.Name, 128}, {r.Description, 4000}} {
		if utf8.RuneCountInString(field.value) > field.max || strings.ContainsRune(field.value, 0) {
			return network.ErrInvalidInput
		}
	}
	return nil
}

type DeviceVersion struct {
	ID      int   `json:"id"`
	Version int64 `json:"version"`
}
type DeviceGroupDeleteReq struct {
	Version int64 `json:"version"`
}
type DeviceGroupAssignReq struct {
	Devices     []DeviceVersion `json:"devices"`
	GroupID     *int            `json:"groupId"`
	GroupSource string          `json:"groupSource"`
}

func ValidSource(source string) bool { return source == "auto" || source == "manual" }
func (r *DeviceGroupAssignReq) Validate() error {
	if len(r.Devices) == 0 || len(r.Devices) > 200 || !ValidSource(r.GroupSource) || (r.GroupID != nil && *r.GroupID <= 0) {
		return network.ErrInvalidInput
	}
	seen := map[int]bool{}
	for _, row := range r.Devices {
		if row.ID <= 0 || row.Version <= 0 || seen[row.ID] {
			return network.ErrInvalidInput
		}
		seen[row.ID] = true
	}
	return nil
}
