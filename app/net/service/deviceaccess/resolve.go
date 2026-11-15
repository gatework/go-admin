package deviceaccess

import (
	"context"
	"os/exec"

	"gorm.io/gorm"

	"github.com/gatework/motor/app/net/models/connection"
	"github.com/gatework/motor/app/net/models/inventory"
)

// Resolve loads the device's stored connection settings and resolves them
// with Effective. It performs no I/O against the device and never creates a
// settings row.
func (c Config) Resolve(ctx context.Context, db *gorm.DB, d inventory.Device) (connection.Settings, error) {
	stored, err := connection.Load(ctx, db, d.ID)
	if err != nil {
		return stored, err
	}
	return c.Effective(d, stored), nil
}

// Effective fills in what stored settings leave open: the inferred profile
// (used when none was chosen), where the profile came from, and whether a
// session could be opened now, with the reason when not.
func (c Config) Effective(d inventory.Device, s connection.Settings) connection.Settings {
	s.InferredProfile = InferProfile(d)
	s.ProfileSource = "manual"
	s.Ready, s.Reason = false, ""
	if s.Profile == "" {
		s.Profile, s.ProfileSource = s.InferredProfile, "automatic"
	}
	if _, err := ConnectorProfile(s.Profile); err != nil {
		s.ProfileSource, s.Reason = "unknown", "profile_unknown"
		return s
	}
	s.Ready = c.Configured(s.Profile)
	if !s.Ready {
		s.Reason = "credentials_missing"
	}
	if s.Ready {
		if _, err := exec.LookPath(s.Protocol); err != nil {
			s.Ready = false
			s.Reason = "transport_unavailable"
		}
	}
	if d.Status != "active" {
		s.Ready, s.Reason = false, "device_unavailable"
	}
	return s
}
