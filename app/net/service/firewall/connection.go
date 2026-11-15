package firewall

import (
	"context"
	"errors"
	"regexp"
	"strings"

	adapter "github.com/gatework/firemon/adapter/netconnector"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/connection"
	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	ndto "github.com/gatework/motor/app/net/service/dto/inventory"
	ns "github.com/gatework/motor/app/net/service/inventory"
)

// ConnectionStatus carries connection status state for this module.
type ConnectionStatus struct {
	connection.Settings
	Vendor                string `json:"vendor"`
	CredentialsConfigured bool   `json:"credentialsConfigured"`
	ContextRequired       bool   `json:"contextRequired"`
}

// Device carries firewall device state for this module.
type Device struct {
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
	brand := firewallBrand(v)
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
	access := deviceaccess.FromEnvironment()
	c := ConnectionStatus{Settings: stored, Vendor: inferFirewallVendor(d)}
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
	c.CredentialsConfigured = access.Configured(c.Profile)
	// Collection only reads, so a device still pending review may be collected.
	switch {
	case !firewallInventory(d) || !deviceaccess.Read.Allows(d.Status):
		c.Reason = "device_unavailable"
	case c.InferredProfile == "":
		c.Reason = "profile_unknown"
	case c.Profile != c.InferredProfile:
		c.Reason = "profile_mismatch"
	default:
		c.Reason = access.Check(d, c.Settings, deviceaccess.Read)
	}
	c.Ready = c.Reason == ""
	return c
}

// Connection provides connection for this module.
func (s *Firewall) Connection(ctx context.Context, id int, p *actions.DataPermission) (ConnectionStatus, error) {
	var d inventory.Device
	if id <= 0 {
		return ConnectionStatus{}, InvalidErr
	}
	if err := deviceScope(s.db(ctx), p).First(&d, id).Error; err != nil {
		return ConnectionStatus{}, safeError(err)
	}
	if !firewallInventory(d) {
		return ConnectionStatus{}, NotFoundErr
	}
	return s.connectionFor(ctx, d)
}

// SetConnection provides set connection for this module.
func (s *Firewall) SetConnection(ctx context.Context, id int, in connection.Input, p *actions.DataPermission) (ConnectionStatus, error) {
	current, err := s.Connection(ctx, id, p)
	if err != nil {
		return current, err
	}
	// Manual options cannot replace authoritative vendor evidence with another
	// platform. Blank retains automatic resolution when inventory is corrected.
	if in.Profile != "" && (current.InferredProfile == "" || in.Profile != current.InferredProfile) {
		return current, InvalidErr
	}
	err = connection.Save(ctx, s.DB, id, in, s.Actor, p)
	switch {
	case errors.Is(err, connection.InvalidErr):
		return current, InvalidErr
	case errors.Is(err, connection.ConflictErr):
		return current, ConflictErr
	case errors.Is(err, connection.NotFoundErr):
		return current, NotFoundErr
	case err != nil:
		return current, err
	}
	return s.Connection(ctx, id, p)
}

// Devices provides devices for this module.
func (s *Firewall) Devices(ctx context.Context, in *ndto.DevicePageReq, p *actions.DataPermission) ([]Device, int64, error) {
	in.FirewallOnly = true
	service := ns.Device{Service: s.Service}
	devices, count, err := service.Page(ctx, in, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Device, 0, len(devices))
	ids := make([]int, 0, len(devices))
	for _, d := range devices {
		ids = append(ids, d.ID)
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
		c, ok := stored[d.ID]
		if !ok {
			c = connection.Settings{DeviceID: d.ID, Protocol: "ssh", Port: 22}
		}
		list := instances[d.ID]
		if list == nil {
			list = []models.Instance{}
		}
		out = append(out, Device{Device: d, Connection: effectiveFirewallConnection(d, c), Instances: list})
	}
	return out, count, nil
}

func (s *Firewall) connectionFor(ctx context.Context, d inventory.Device) (ConnectionStatus, error) {
	stored, err := connection.Load(ctx, s.DB, d.ID)
	if err != nil {
		return ConnectionStatus{}, err
	}
	return effectiveFirewallConnection(d, stored), nil
}

func firewallBrand(v string) string {
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
	return brand
}
