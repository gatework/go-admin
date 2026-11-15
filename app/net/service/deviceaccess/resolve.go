package deviceaccess

import (
	"context"

	"gorm.io/gorm"

	"github.com/gatework/motor/app/net/models/connection"
	"github.com/gatework/motor/app/net/models/inventory"
)

// Resolve loads the device's stored connection settings and resolves them
// with Effective for use. It performs no I/O against the device and never
// creates a settings row.
func (c Config) Resolve(ctx context.Context, db *gorm.DB, d inventory.Device, use Use) (connection.Settings, error) {
	stored, err := connection.Load(ctx, db, d.ID)
	if err != nil {
		return stored, err
	}
	return c.Effective(d, stored, use), nil
}

// Effective fills in what stored settings leave open: the inferred profile
// (used when none was chosen), where the profile came from, and whether a
// session for use could be opened now, with the reason when not. A device the
// use does not allow is device_unavailable before anything else; then an
// unregistered profile is profile_unknown; then Check decides.
func (c Config) Effective(d inventory.Device, s connection.Settings, use Use) connection.Settings {
	s.InferredProfile = InferProfile(d)
	s.ProfileSource = "manual"
	if s.Profile == "" {
		s.Profile, s.ProfileSource = s.InferredProfile, "automatic"
	}
	_, err := ConnectorProfile(s.Profile)
	if err != nil {
		s.ProfileSource = "unknown"
	}
	switch {
	case !use.Allows(d.Status):
		s.Reason = "device_unavailable"
	case err != nil:
		s.Reason = "profile_unknown"
	default:
		s.Reason = c.Check(d, s, use)
	}
	s.Ready = s.Reason == ""
	return s
}
