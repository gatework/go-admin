package service

import (
	"context"
	"errors"
	"net/netip"
	"os/exec"
	"regexp"
	"strings"

	adapter "github.com/gatework/firemon/adapter/netconnector"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	delivery "go-admin/app/delivery/models"
	ds "go-admin/app/delivery/service"
	"go-admin/app/firewall/models"
	"go-admin/app/network/connection"
	inventory "go-admin/app/network/models"
	ns "go-admin/app/network/service"
	ndto "go-admin/app/network/service/dto"
)

type ConnectionStatus struct {
	connection.Settings
	Vendor                string `json:"vendor"`
	CredentialsConfigured bool   `json:"credentialsConfigured"`
	ContextRequired       bool   `json:"contextRequired"`
}

type FirewallDevice struct {
	inventory.Device
	Connection ConnectionStatus  `json:"connection"`
	Instances  []models.Instance `json:"instances"`
}

var unsupportedFirewall = regexp.MustCompile(`(?i)\b(?:fortimanager|fortianalyzer|panorama|firepower|ftd|ios|comware|linux|windows|nexus)\b`)

func platformWord(value string) string {
	return strings.NewReplacer(" ", "", "-", "", "_", "").Replace(strings.ToLower(value))
}
func inferFirewallVendor(d inventory.Device) string {
	if unsupportedFirewall.MatchString(d.OS + " " + d.HardwareModel) {
		return ""
	}
	v, os, model := platformWord(d.Vendor), platformWord(d.OS), platformWord(d.HardwareModel)
	brand := ""
	switch {
	case strings.Contains(v, "paloalto"):
		brand = "paloalto"
	case strings.Contains(v, "hillstone"):
		brand = "hillstone"
	case strings.Contains(v, "huawei"):
		brand = "huawei"
	case strings.Contains(v, "fortinet"):
		brand = "fortinet"
	case strings.Contains(v, "netscreen"):
		brand = "netscreen"
	case strings.Contains(v, "juniper"):
		brand = "juniper"
	case strings.Contains(v, "cisco"):
		brand = "cisco"
	}
	hint := ""
	switch {
	case strings.Contains(os, "panos"):
		hint = "paloalto"
	case strings.Contains(os, "screenos"):
		hint = "netscreen"
	case strings.Contains(os, "stoneos") || os == "hillstone":
		hint = "hillstone"
	case strings.Contains(os, "fortios"):
		hint = "fortinet"
	case strings.Contains(os, "junos"):
		hint = "srx"
	case strings.Contains(os, "asa"):
		hint = "asa"
	case strings.Contains(os, "vrp") || strings.Contains(os, "huawei"):
		hint = "huawei"
	}
	if hint == "" {
		switch {
		case strings.Contains(model, "asa"):
			hint = "asa"
		case strings.Contains(model, "srx"):
			hint = "srx"
		case strings.Contains(model, "ssg") || strings.Contains(model, "netscreen"):
			hint = "netscreen"
		case strings.Contains(model, "fortigate"):
			hint = "fortinet"
		case strings.Contains(model, "usg"):
			hint = "huawei"
		default:
			if brand != "juniper" && brand != "cisco" {
				hint = brand
			}
		}
	}
	if hint == "" {
		return ""
	}
	if brand != "" && brand != hint && !(brand == "juniper" && (hint == "srx" || hint == "netscreen")) && !(brand == "cisco" && hint == "asa") {
		return ""
	}
	return hint
}

func firewallInventory(d inventory.Device) bool {
	return d.DeviceType == "firewall" || d.DeviceRole == "firewall"
}

func effectiveFirewallConnection(d inventory.Device, stored connection.Settings) ConnectionStatus {
	c := ConnectionStatus{Settings: stored, Vendor: inferFirewallVendor(d)}
	c.Ready = false
	c.Reason = ""
	c.ProfileSource = "automatic"
	if stored.Profile != "" {
		c.ProfileSource = "manual"
	}
	if profile, err := adapter.Profile(c.Vendor); err == nil {
		c.InferredProfile = profile.Name
	}
	if c.Profile == "" {
		c.Profile = c.InferredProfile
	}
	c.ContextRequired = c.Vendor == "fortinet"
	switch {
	case !firewallInventory(d) || (d.Status != "pending" && d.Status != "active"):
		c.Reason = "device_unavailable"
	case c.InferredProfile == "":
		c.Reason = "profile_unknown"
	case c.Profile != c.InferredProfile:
		c.Reason = "profile_mismatch"
	case c.Protocol != "ssh" && c.Protocol != "telnet" || c.Port < 1 || c.Port > 65535:
		c.Reason = "connection_invalid"
	default:
		if _, err := netip.ParseAddr(d.IP); err != nil {
			c.Reason = "address_invalid"
			break
		}
		_, err := ds.FromEnvironment().ConnectionConfig(delivery.DeviceSnapshot{ID: d.Id, IP: d.IP}, c.Settings)
		c.CredentialsConfigured = err == nil
		if err != nil {
			c.Reason = "credentials_missing"
			break
		}
		if _, err = exec.LookPath(c.Protocol); err != nil {
			c.Reason = "transport_unavailable"
			break
		}
		c.Ready = true
	}
	return c
}

func (s *Firewall) connectionFor(ctx context.Context, d inventory.Device) (ConnectionStatus, error) {
	stored, err := connection.Load(ctx, s.Orm, d.Id)
	if err != nil {
		return ConnectionStatus{}, err
	}
	return effectiveFirewallConnection(d, stored), nil
}
func (s *Firewall) Connection(ctx context.Context, id int, p *actions.DataPermission) (ConnectionStatus, error) {
	var d inventory.Device
	if id <= 0 {
		return ConnectionStatus{}, ErrInvalid
	}
	if err := deviceScope(s.db(ctx), p).First(&d, id).Error; err != nil {
		return ConnectionStatus{}, safeError(err)
	}
	if !firewallInventory(d) {
		return ConnectionStatus{}, ErrNotFound
	}
	return s.connectionFor(ctx, d)
}
func (s *Firewall) SetConnection(ctx context.Context, id int, in connection.Input, p *actions.DataPermission) (ConnectionStatus, error) {
	current, err := s.Connection(ctx, id, p)
	if err != nil {
		return current, err
	}
	// Manual options cannot replace authoritative vendor evidence with another
	// platform. Blank retains automatic resolution when inventory is corrected.
	if in.Profile != "" && (current.InferredProfile == "" || in.Profile != current.InferredProfile) {
		return current, ErrInvalid
	}
	err = connection.Save(ctx, s.Orm, id, in, s.Actor, p)
	switch {
	case errors.Is(err, connection.ErrInvalid):
		return current, ErrInvalid
	case errors.Is(err, connection.ErrConflict):
		return current, ErrConflict
	case errors.Is(err, connection.ErrNotFound):
		return current, ErrNotFound
	case err != nil:
		return current, err
	}
	return s.Connection(ctx, id, p)
}

func (s *Firewall) Devices(ctx context.Context, in *ndto.DevicePageReq, p *actions.DataPermission) ([]FirewallDevice, int64, error) {
	in.FirewallOnly = true
	service := ns.Device{Service: s.Service}
	devices, count, err := service.Page(ctx, in, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]FirewallDevice, 0, len(devices))
	ids := make([]int, 0, len(devices))
	for _, d := range devices {
		ids = append(ids, d.Id)
	}
	stored := map[int]connection.Settings{}
	instances := map[int][]models.Instance{}
	if len(ids) > 0 {
		var settings []connection.Settings
		if err = s.db(ctx).Where("device_id IN ?", ids).Find(&settings).Error; err != nil {
			return nil, 0, err
		}
		for _, c := range settings {
			stored[c.DeviceID] = c
		}
		var rows []models.Instance
		if err = s.db(ctx).Where("device_id IN ?", ids).Order("id").Find(&rows).Error; err != nil {
			return nil, 0, err
		}
		for _, i := range rows {
			instances[i.DeviceID] = append(instances[i.DeviceID], i)
		}
	}
	for _, d := range devices {
		c, ok := stored[d.Id]
		if !ok {
			c = connection.Settings{DeviceID: d.Id, Protocol: "ssh", Port: 22}
		}
		list := instances[d.Id]
		if list == nil {
			list = []models.Instance{}
		}
		out = append(out, FirewallDevice{Device: d, Connection: effectiveFirewallConnection(d, c), Instances: list})
	}
	return out, count, nil
}
