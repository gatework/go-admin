package inventory

import (
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	common "github.com/gatework/motor-core/sdk/contract/dto"
	"github.com/gatework/motor/app/net/models/inventory"
)

// CircuitControlReq carries the circuit control req fields exchanged by network operations.
type CircuitControlReq inventory.CircuitFields

var circuitNumber = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._/-]{0,119}$`)
var circuitMoney = regexp.MustCompile(`^[0-9]{1,10}(\.[0-9]{1,2})?$`)
var circuitCurrency = regexp.MustCompile(`^[A-Z]{3}$`)
var zabbixID = regexp.MustCompile(`^[1-9][0-9]{0,31}$`)
var circuitStatuses = []string{"planned", "active", "suspended", "terminated"}
var circuitCategories = []string{"mpls", "internet_dedicated", "sdwan", "dark_fiber", "p2p", "other"}

func circuitText(value *string, limit int) bool {
	*value = strings.TrimSpace(*value)
	return utf8.ValidString(*value) && utf8.RuneCountInString(*value) <= limit && !strings.ContainsRune(*value, 0)
}

// Validate normalizes the request and rejects invalid or conflicting fields.
func (r *CircuitControlReq) Validate(edit bool) error {
	if (edit && r.Version <= 0) || (!edit && r.Version != 0) {
		return inventory.InvalidErr
	}
	r.CircuitNo = strings.ToUpper(strings.TrimSpace(r.CircuitNo))
	if !circuitNumber.MatchString(r.CircuitNo) {
		return inventory.InvalidErr
	}
	for _, field := range []struct {
		value *string
		max   int
	}{
		{&r.Name, 160}, {&r.Provider, 160}, {&r.Region, 120}, {&r.Organization, 160}, {&r.ContractNo, 160},
		{&r.Notes, 10000}, {&r.RouteNotes, 10000}, {&r.Currency, 3}, {&r.Fee, 13}, {&r.InstalledOn, 10}, {&r.ExpiresOn, 10},
	} {
		if !circuitText(field.value, field.max) {
			return inventory.InvalidErr
		}
	}
	if r.Name == "" || r.Provider == "" {
		return inventory.InvalidErr
	}
	if r.Status == "" {
		r.Status = "planned"
	}
	if r.Category == "" {
		r.Category = "other"
	}
	if r.Purpose == "" {
		r.Purpose = "other"
	}
	if r.Payer == "" {
		r.Payer = "us"
	}
	if r.Currency == "" {
		r.Currency = "CNY"
	}
	if !slices.Contains(circuitStatuses, r.Status) || !slices.Contains(circuitCategories, r.Category) ||
		!slices.Contains([]string{"production_primary", "backup", "office_internet", "site_interconnect", "cloud_access", "partner_access", "management", "other"}, r.Purpose) ||
		!slices.Contains([]string{"us", "vendor", "shared"}, r.Payer) ||
		!slices.Contains([]string{"", "monthly", "quarterly", "yearly", "one_time"}, r.BillingCycle) || !circuitCurrency.MatchString(r.Currency) {
		return inventory.InvalidErr
	}
	if r.BandwidthMbps != nil && (*r.BandwidthMbps < 1 || *r.BandwidthMbps > 1000000) {
		return inventory.InvalidErr
	}
	if err := r.validateBilling(); err != nil {
		return err
	}
	for _, endpoint := range []*inventory.CircuitEndpoint{&r.AEndpoint, &r.ZEndpoint} {
		if err := validateCircuitEndpoint(endpoint); err != nil {
			return err
		}
	}
	if r.AEndpoint.DeviceID != nil && r.ZEndpoint.DeviceID != nil && *r.AEndpoint.DeviceID == *r.ZEndpoint.DeviceID {
		return inventory.InvalidErr
	}
	return nil
}

func validateCircuitEndpoint(e *inventory.CircuitEndpoint) error {
	if e.DeviceID != nil && *e.DeviceID <= 0 {
		return inventory.InvalidErr
	}
	for _, field := range []struct {
		value *string
		max   int
	}{
		{&e.Name, 160}, {&e.Address, 512}, {&e.Interface, 160}, {&e.IP, 45}, {&e.ContactName, 128}, {&e.ContactPhone, 64},
		{&e.Monitoring.HostID, 32}, {&e.Monitoring.InItemID, 32}, {&e.Monitoring.OutItemID, 32},
		{&e.Monitoring.HostName, 512}, {&e.Monitoring.InItemName, 512}, {&e.Monitoring.OutItemName, 512},
	} {
		if !circuitText(field.value, field.max) {
			return inventory.InvalidErr
		}
	}
	if e.IP != "" {
		ip, err := netip.ParseAddr(e.IP)
		if err != nil || ip.Zone() != "" {
			return inventory.InvalidErr
		}
		e.IP = ip.Unmap().String()
	}
	b := &e.Monitoring
	for _, id := range []string{b.HostID, b.InItemID, b.OutItemID} {
		if id != "" && !zabbixID.MatchString(id) {
			return inventory.InvalidErr
		}
	}
	if b.HostID == "" && (b.InItemID != "" || b.OutItemID != "") {
		return inventory.CircuitBindingErr
	}
	if b.InItemID != "" && b.InItemID == b.OutItemID {
		return inventory.CircuitBindingErr
	}
	return nil
}

// CircuitPageReq carries the circuit page req fields exchanged by network operations.
type CircuitPageReq struct {
	common.Pagination
	Keyword      string `form:"keyword"`
	Status       string `form:"status"`
	Category     string `form:"category"`
	Provider     string `form:"provider"`
	Region       string `form:"region"`
	Organization string `form:"organization"`
	Monitored    string `form:"monitored"`
}

// Validate normalizes the request and rejects invalid or conflicting fields.
func (r *CircuitPageReq) Validate() error {
	if r.PageIndex < 0 || r.PageIndex > 1000000 || r.PageSize < 0 || r.PageSize > 200 {
		return inventory.InvalidErr
	}
	for _, field := range []struct {
		value *string
		max   int
	}{{&r.Keyword, 512}, {&r.Status, 20}, {&r.Category, 32}, {&r.Provider, 160}, {&r.Region, 120}, {&r.Organization, 160}, {&r.Monitored, 5}} {
		if !circuitText(field.value, field.max) {
			return inventory.InvalidErr
		}
	}
	if (r.Status != "" && !slices.Contains(circuitStatuses, r.Status)) || (r.Category != "" && !slices.Contains(circuitCategories, r.Category)) || !slices.Contains([]string{"", "true", "false"}, r.Monitored) {
		return inventory.InvalidErr
	}
	r.GetPageIndex()
	r.GetPageSize()
	return nil
}

// CircuitOptions carries the circuit options fields exchanged by network operations.
type CircuitOptions struct {
	Providers        []string `json:"providers"`
	Regions          []string `json:"regions"`
	Organizations    []string `json:"organizations"`
	ZabbixConfigured bool     `json:"zabbixConfigured"`
}

// CircuitDeleteReq carries the circuit delete req fields exchanged by network operations.
type CircuitDeleteReq struct {
	Version int64 `json:"version"`
}

func (r *CircuitControlReq) validateBilling() error {
	if r.Fee != "" {
		if !circuitMoney.MatchString(r.Fee) {
			return inventory.InvalidErr
		}
		parts := strings.SplitN(r.Fee, ".", 2)
		whole := strings.TrimLeft(parts[0], "0")
		if whole == "" {
			whole = "0"
		}
		fraction := "00"
		if len(parts) == 2 {
			fraction = (parts[1] + "0")[:2]
		}
		r.Fee = whole + "." + fraction
	}
	for _, value := range []string{r.InstalledOn, r.ExpiresOn} {
		if value != "" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return inventory.InvalidErr
			}
		}
	}
	if r.InstalledOn != "" && r.ExpiresOn != "" && r.ExpiresOn < r.InstalledOn {
		return inventory.InvalidErr
	}
	return nil
}
