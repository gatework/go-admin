package delivery

import (
	models "github.com/gatework/motor/app/net/models/delivery"
	"github.com/gatework/motor/app/net/models/inventory"
)

func deviceSnapshot(d inventory.Device, profile string) models.DeviceSnapshot {
	return models.DeviceSnapshot{ID: d.ID, Name: d.Name, Hostname: d.Hostname, IP: d.IP, Vendor: d.Vendor, Model: d.HardwareModel, DeviceType: &d.DeviceType, DeviceRole: &d.DeviceRole, Profile: profile}
}

// Old snapshots omit classification; new snapshots preserve even empty values.
// A mutable display name is not a transport identity.
func matchesSnapshot(current, saved models.DeviceSnapshot) bool {
	if current.ID != saved.ID || current.IP != saved.IP || current.Hostname != saved.Hostname || current.Vendor != saved.Vendor || current.Model != saved.Model {
		return false
	}
	if saved.Profile != "" && current.Profile != saved.Profile {
		return false
	}
	if saved.DeviceRole != nil && (current.DeviceRole == nil || *current.DeviceRole != *saved.DeviceRole) {
		return false
	}
	return saved.DeviceType == nil || current.DeviceType != nil && *current.DeviceType == *saved.DeviceType
}
