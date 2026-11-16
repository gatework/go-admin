package deviceaccess

import (
	"os"
	"strings"
	"time"

	"github.com/gatework/go-net-connector/connector"
	"github.com/gatework/go-net-connector/vendors"
	"github.com/gatework/motor/app/net/models/connection"
	"github.com/gatework/motor/common/errors"
	"github.com/gatework/motor/common/secret"
)

// MaxOutputBytes bounds what one session may read back from a device.
const MaxOutputBytes = 4 << 20

// CredentialsErr is a profile without usable credentials, or a host-key
// policy the connector does not know.
//
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var CredentialsErr = errors.New(errors.KindInvalid, "device_access.credentials_missing", "") //revive:disable-line:error-naming

// Credentials sign in to a device. The passwords are secret.Secret, so a
// Config logged or printed by mistake shows "[redacted]".
type Credentials struct {
	Username       string
	Password       secret.Secret
	EnablePassword secret.Secret
}

// Config is the server-owned half of device access.
type Config struct {
	// Shared credentials apply to every profile without its own.
	Shared Credentials
	// Profiles override Shared per connector profile (NC_<PROFILE>_*).
	Profiles map[string]Credentials
	// HostKeyPolicy is "strict" (the default) or "accept_new"; KnownHosts is
	// the known_hosts file the policy checks against.
	HostKeyPolicy, KnownHosts string
}

// FromEnvironment loads the credentials and host-key settings.
func FromEnvironment() Config {
	c := Config{
		Shared: Credentials{
			Username:       os.Getenv("NC_DEVICE_USERNAME"),
			Password:       secret.Secret(os.Getenv("NC_DEVICE_PASSWORD")),
			EnablePassword: secret.Secret(os.Getenv("NC_DEVICE_ENABLE_PASSWORD")),
		},
		Profiles:      profileCredentials(),
		HostKeyPolicy: os.Getenv("NC_HOST_KEY_POLICY"),
		KnownHosts:    os.Getenv("NC_KNOWN_HOSTS"),
	}
	if c.HostKeyPolicy == "" {
		c.HostKeyPolicy = "strict"
	}
	return c
}

// Configured reports whether a session with profile could sign in: it has a
// username and password, and the host-key policy is one the connector knows.
func (c Config) Configured(profile string) bool {
	creds := c.credentials(profile)
	return creds.Username != "" && creds.Password.IsSet() && (c.HostKeyPolicy == "" || c.HostKeyPolicy == "strict" || c.HostKeyPolicy == "accept_new")
}

// ConnectionConfig is the connector configuration for a session to host with
// the device's stored settings. Secrets are revealed only into the returned
// value, which is never persisted.
func (c Config) ConnectionConfig(host string, s connection.Settings) (connector.ConnectionConfig, error) {
	if !c.Configured(s.Profile) {
		return connector.ConnectionConfig{}, CredentialsErr
	}
	creds := c.credentials(s.Profile)
	return connector.ConnectionConfig{
		Host: host, Protocol: connector.Protocol(s.Protocol), Port: s.Port,
		Username: creds.Username, Password: creds.Password.Reveal(), EnablePassword: creds.EnablePassword.Reveal(),
		HostKeyPolicy: connector.HostKeyPolicy(c.HostKeyPolicy), KnownHosts: c.KnownHosts,
		LoginTimeout: 30 * time.Second, CommandTimeout: 60 * time.Second, WriteTimeout: 10 * time.Second,
		MaxOutputBytes: MaxOutputBytes, MaxScriptOutputBytes: MaxOutputBytes,
	}, nil
}

// credentials are the profile's own, field by field, falling back to the
// shared ones.
func (c Config) credentials(profile string) Credentials {
	own, ok := c.Profiles[strings.ToLower(profile)]
	if !ok {
		return c.Shared
	}
	merged := c.Shared
	if own.Username != "" {
		merged.Username = own.Username
	}
	if own.Password.IsSet() {
		merged.Password = own.Password
	}
	if own.EnablePassword.IsSet() {
		merged.EnablePassword = own.EnablePassword
	}
	return merged
}

// profileCredentials reads the per-profile overrides once, for every profile
// the connector knows.
func profileCredentials() map[string]Credentials {
	profiles := map[string]Credentials{}
	for _, profile := range vendors.NewBuiltinRegistry().Names() {
		prefix := "NC_" + strings.ToUpper(profile) + "_"
		own := Credentials{
			Username:       os.Getenv(prefix + "USERNAME"),
			Password:       secret.Secret(os.Getenv(prefix + "PASSWORD")),
			EnablePassword: secret.Secret(os.Getenv(prefix + "ENABLE_PASSWORD")),
		}
		if own != (Credentials{}) {
			profiles[strings.ToLower(profile)] = own
		}
	}
	return profiles
}
