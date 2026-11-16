package deviceaccess

import (
	"net/netip"
	"os/exec"

	"github.com/gatework/motor/app/net/models/connection"
	"github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/common/errors"
)

// Use is what a session will do on the device, which decides the inventory
// statuses it may be opened for.
type Use uint8

const (
	// Read sessions only collect (show commands). A device Netdisco has just
	// discovered is still "pending" review, and reading it is how an operator
	// learns what it is, so pending devices may be read.
	Read Use = iota + 1
	// Write sessions change configuration, and only on a device an operator
	// has accepted: "active".
	Write
)

// UnavailableErr is a device whose inventory status does not allow the use.
//
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var UnavailableErr = errors.New(errors.KindInvalid, "device_access.device_unavailable", "") //revive:disable-line:error-naming

// Allows reports whether a device in inventory status may be opened for u.
// Maintenance, disabled and retired devices are opened for nothing.
func (u Use) Allows(status string) bool {
	switch u {
	case Read:
		return status == "pending" || status == "active"
	case Write:
		return status == "active"
	default:
		return false
	}
}

// Check reports why a session for use cannot be opened to d now with the
// settings s, or "" when it can. The reasons, in the order checked, are
// device_unavailable, connection_invalid, address_invalid,
// credentials_missing and transport_unavailable. It does not judge the
// profile itself: Effective does for connector profiles, and firewall for its
// own vendors.
func (c Config) Check(d inventory.Device, s connection.Settings, use Use) string {
	switch {
	case !use.Allows(d.Status):
		return "device_unavailable"
	case s.Protocol != "ssh" && s.Protocol != "telnet" || s.Port < 1 || s.Port > 65535:
		return "connection_invalid"
	}
	if _, err := netip.ParseAddr(d.IP); err != nil {
		return "address_invalid"
	}
	if !c.Configured(s.Profile) {
		return "credentials_missing"
	}
	if _, err := exec.LookPath(s.Protocol); err != nil {
		return "transport_unavailable"
	}
	return ""
}
