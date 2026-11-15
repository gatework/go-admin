package inventory

import (
	"net/mail"
	"slices"
	"strings"
	"unicode/utf8"

	common "github.com/gatework/motor-core/sdk/contract/dto"
	"github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/common/paging"
)

// DevicePageReq carries the device page req fields exchanged by network operations.
type DevicePageReq struct {
	common.Pagination
	FirewallOnly    bool   `form:"-"`
	Keyword         string `form:"keyword"`
	Vendor          string `form:"vendor"`
	Model           string `form:"model"`
	Status          string `form:"status"`
	DeviceType      string `form:"deviceType"`
	DeviceRole      string `form:"deviceRole"`
	VendorEmpty     bool   `form:"vendorEmpty"`
	DeviceTypeEmpty bool   `form:"deviceTypeEmpty"`
	DeviceRoleEmpty bool   `form:"deviceRoleEmpty"`
	GroupID         *int   `form:"groupId"`
	Tag             string `form:"tag"`
}

// Validate normalizes the request and rejects invalid or conflicting fields.
func (r *DevicePageReq) Validate() error {
	if !paging.Normalize(&r.PageIndex, &r.PageSize) || (r.GroupID != nil && *r.GroupID < 0) {
		return inventory.InvalidErr
	}
	for _, value := range []string{r.Keyword, r.Vendor, r.Model, r.Status, r.DeviceType, r.DeviceRole, r.Tag} {
		if utf8.RuneCountInString(value) > 512 {
			return inventory.InvalidErr
		}
	}
	return nil
}

// DeviceUpdateReq carries the device update req fields exchanged by network operations.
type DeviceUpdateReq struct {
	Version              int64    `json:"version"`
	GroupID              *int     `json:"groupId"`
	GroupSource          string   `json:"groupSource"`
	ClassificationSource string   `json:"classificationSource"`
	Status               string   `json:"status"`
	DeviceType           string   `json:"deviceType"`
	DeviceRole           string   `json:"deviceRole"`
	Owner                string   `json:"owner"`
	ContactEmail         string   `json:"contactEmail"`
	ContactPhone         string   `json:"contactPhone"`
	Tags                 []string `json:"tags"`
	Notes                string   `json:"notes"`
}

// Validate normalizes the request and rejects invalid or conflicting fields.
func (r *DeviceUpdateReq) Validate() error {
	if !ValidSource(r.GroupSource) || !ValidSource(r.ClassificationSource) || (r.GroupID != nil && *r.GroupID <= 0) {
		return inventory.InvalidErr
	}
	if r.Version <= 0 || !slices.Contains([]string{"pending", "active", "maintenance", "disabled", "retired"}, r.Status) {
		return inventory.InvalidErr
	}
	if !slices.Contains([]string{"", "switch", "router", "firewall", "load_balancer", "wireless_controller", "wireless_ap", "network_management_server", "other"}, r.DeviceType) {
		return inventory.InvalidErr
	}
	if !slices.Contains([]string{"", "core_switch", "aggregate_switch", "poe_switch", "access_switch", "leaf_switch", "border_switch", "spine_switch", "firewall", "load_balancer", "wireless_controller", "wireless_ap", "network_management_server", "other"}, r.DeviceRole) {
		return inventory.InvalidErr
	}
	for _, field := range []struct {
		value *string
		max   int
	}{{&r.Owner, 128}, {&r.ContactEmail, 254}, {&r.ContactPhone, 64}, {&r.Notes, 10000}} {
		*field.value = strings.TrimSpace(*field.value)
		if utf8.RuneCountInString(*field.value) > field.max || strings.ContainsRune(*field.value, 0) {
			return inventory.InvalidErr
		}
	}
	if r.ContactEmail != "" {
		address, err := mail.ParseAddress(r.ContactEmail)
		if err != nil || address.Address != r.ContactEmail {
			return inventory.InvalidErr
		}
	}
	if len(r.Tags) > 50 {
		return inventory.InvalidErr
	}
	tags := make([]string, 0, len(r.Tags))
	for _, tag := range r.Tags {
		tag = strings.TrimSpace(tag)
		if utf8.RuneCountInString(tag) > 64 || strings.ContainsRune(tag, 0) {
			return inventory.InvalidErr
		}
		if tag != "" && !slices.Contains(tags, tag) {
			tags = append(tags, tag)
		}
	}
	r.Tags = tags
	return nil
}

// DeviceOptions carries the device options fields exchanged by network operations.
type DeviceOptions struct {
	Vendors []string `json:"vendors"`
	Models  []string `json:"models"`
}
