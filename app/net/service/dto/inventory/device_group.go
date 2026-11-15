package inventory

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gatework/motor/app/net/models/inventory"
)

// DeviceGroupNode carries the device group node fields exchanged by network operations.
type DeviceGroupNode struct {
	inventory.DeviceGroup
	DirectDeviceCount int64             `json:"directDeviceCount"`
	DeviceCount       int64             `json:"deviceCount"`
	Children          []DeviceGroupNode `json:"children"`
}

// DeviceGroupTree carries the device group tree fields exchanged by network operations.
type DeviceGroupTree struct {
	Items                []DeviceGroupNode `json:"items"`
	TotalDeviceCount     int64             `json:"totalDeviceCount"`
	UngroupedDeviceCount int64             `json:"ungroupedDeviceCount"`
}

// DeviceGroupReq carries the device group req fields exchanged by network operations.
type DeviceGroupReq struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	ParentID    *int   `json:"parentId"`
	Description string `json:"description"`
	Marker      string `json:"marker"`
	Enabled     bool   `json:"enabled"`
	Position    int    `json:"position"`
	Version     int64  `json:"version"`
}

var groupCode = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)

// Validate normalizes the request and rejects invalid or conflicting fields.
func (r *DeviceGroupReq) Validate(update bool) error {
	r.Code = strings.ToLower(strings.TrimSpace(r.Code))
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	r.Marker = strings.ToLower(strings.TrimSpace(r.Marker))
	if utf8.RuneCountInString(r.Marker) > 32 || strings.ContainsFunc(r.Marker, unicode.IsSpace) || strings.ContainsRune(r.Marker, 0) {
		return inventory.InvalidErr
	}
	if (update && r.Version <= 0) || r.Position < 0 || (r.ParentID != nil && *r.ParentID <= 0) || len(r.Code) > 100 || !groupCode.MatchString(r.Code) || r.Name == "" {
		return inventory.InvalidErr
	}
	for _, field := range []struct {
		value string
		max   int
	}{{r.Name, 128}, {r.Description, 4000}} {
		if utf8.RuneCountInString(field.value) > field.max || strings.ContainsRune(field.value, 0) {
			return inventory.InvalidErr
		}
	}
	return nil
}

// DeviceVersion carries the device version fields exchanged by network operations.
type DeviceVersion struct {
	ID      int   `json:"id"`
	Version int64 `json:"version"`
}

// DeviceGroupDeleteReq carries the device group delete req fields exchanged by network operations.
type DeviceGroupDeleteReq struct {
	Version int64 `json:"version"`
}

// DeviceGroupAssignReq carries the device group assign req fields exchanged by network operations.
type DeviceGroupAssignReq struct {
	Devices     []DeviceVersion `json:"devices"`
	GroupID     *int            `json:"groupId"`
	GroupSource string          `json:"groupSource"`
}

// ValidSource provides the valid source operation within the caller's authorized network scope.
func ValidSource(source string) bool { return source == "auto" || source == "manual" }

// Validate normalizes the request and rejects invalid or conflicting fields.
func (r *DeviceGroupAssignReq) Validate() error {
	if len(r.Devices) == 0 || len(r.Devices) > 200 || !ValidSource(r.GroupSource) || (r.GroupID != nil && *r.GroupID <= 0) {
		return inventory.InvalidErr
	}
	seen := map[int]bool{}
	for _, row := range r.Devices {
		if row.ID <= 0 || row.Version <= 0 || seen[row.ID] {
			return inventory.InvalidErr
		}
		seen[row.ID] = true
	}
	return nil
}
