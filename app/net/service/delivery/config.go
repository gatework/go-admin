package delivery

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gatework/go-net-connector/vendors"
	"github.com/gatework/motor-core/sdk/contract/actions"
	ndto "github.com/gatework/motor/app/net/service/dto/inventory"
	network "github.com/gatework/motor/app/net/service/inventory"
)

//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var (
	// InvalidErr identifies invalid failures.
	InvalidErr = errors.New("delivery.invalid_input")
	// ConflictErr identifies conflict failures.
	ConflictErr = errors.New("delivery.version_conflict")
	// NotFoundErr identifies not found failures.
	NotFoundErr = errors.New("delivery.not_found")
	// DisabledErr identifies disabled failures.
	DisabledErr = errors.New("delivery.disabled")
	// ConnectionErr identifies connection failures.
	ConnectionErr = errors.New("delivery.connection_not_ready")
	// CancelledErr identifies cancelled failures.
	CancelledErr = errors.New("delivery.cancelled")
	// IdentityErr identifies identity failures.
	IdentityErr = errors.New("delivery.device_changed")
)

//revive:enable:error-naming

// MaxTargets bounds the permitted targets.
const MaxTargets = 100

// MaxScriptBytes bounds the permitted script bytes.
const MaxScriptBytes = 64 << 10

// MaxTotalBytes bounds the permitted total bytes.
const MaxTotalBytes = 4 << 20

// MaxOutputBytes bounds the permitted output bytes.
const MaxOutputBytes = 4 << 20

// Config holds server-owned credentials and limits for device operations.
type Config struct {
	Enabled                                                       bool
	Username, Password, EnablePassword, HostKeyPolicy, KnownHosts string
	Grace                                                         time.Duration
	Concurrency                                                   int
}

// FromEnvironment loads the server-owned device transport configuration.
func FromEnvironment() Config {
	c := Config{Enabled: os.Getenv("NETWORK_DELIVERY_ENABLED") == "true", Username: os.Getenv("NC_DEVICE_USERNAME"), Password: os.Getenv("NC_DEVICE_PASSWORD"), EnablePassword: os.Getenv("NC_DEVICE_ENABLE_PASSWORD"), HostKeyPolicy: os.Getenv("NC_HOST_KEY_POLICY"), KnownHosts: os.Getenv("NC_KNOWN_HOSTS"), Grace: 600 * time.Second, Concurrency: 5}
	if c.HostKeyPolicy == "" {
		c.HostKeyPolicy = "strict"
	}
	if n, e := strconv.Atoi(os.Getenv("NETWORK_DELIVERY_CONCURRENCY")); e == nil && n >= 1 && n <= 20 {
		c.Concurrency = n
	}
	return c
}

// Options provides the options operation within the caller's authorized delivery scope.
func (c Config) Options() Options {
	o := Options{Enabled: c.Enabled, Concurrency: c.Concurrency, GraceSeconds: int(c.Grace.Seconds()), MaxTargets: MaxTargets, MaxScriptBytes: MaxScriptBytes, Profiles: vendors.NewBuiltinRegistry().Names(), InventoryGroups: []InventoryGroup{}}
	for _, p := range o.Profiles {
		if c.configured(p) {
			o.Configured = true
		}
	}
	if !o.Enabled {
		o.Reason = "delivery_disabled"
	} else if !o.Configured {
		o.Reason = "credentials_missing"
	}
	return o
}
func (c Config) credentials(profile string) (string, string, string) {
	prefix := "NC_" + strings.ToUpper(profile) + "_"
	u, p, e := c.Username, c.Password, c.EnablePassword
	if v, ok := os.LookupEnv(prefix + "USERNAME"); ok {
		u = v
	}
	if v, ok := os.LookupEnv(prefix + "PASSWORD"); ok {
		p = v
	}
	if v, ok := os.LookupEnv(prefix + "ENABLE_PASSWORD"); ok {
		e = v
	}
	return u, p, e
}

// Options describes options for delivery operations.
type Options struct {
	Enabled         bool                 `json:"enabled"`
	Configured      bool                 `json:"configured"`
	Reason          string               `json:"reason"`
	Concurrency     int                  `json:"concurrency"`
	GraceSeconds    int                  `json:"graceSeconds"`
	MaxTargets      int                  `json:"maxTargets"`
	MaxScriptBytes  int                  `json:"maxScriptBytes"`
	Profiles        []string             `json:"profiles"`
	InventoryGroups []InventoryGroup     `json:"inventoryGroups"`
	DeviceGroups    ndto.DeviceGroupTree `json:"deviceGroups"`
}

// InventoryGroup describes inventory group for delivery operations.
type InventoryGroup struct {
	DeviceType string `json:"deviceType"`
	DeviceRole string `json:"deviceRole"`
	Vendor     string `json:"vendor"`
	Model      string `json:"model"`
	Count      int64  `json:"count"`
}

// Options provides the options operation within the caller's authorized delivery scope.
func (s *Workflow) Options(ctx context.Context, p *actions.DataPermission) (Options, error) {
	o := s.Config.Options()
	err := s.inventory(ctx, p).Select("COALESCE(device_type, '') AS device_type, COALESCE(device_role, '') AS device_role, COALESCE(vendor, '') AS vendor, COALESCE(model, '') AS model, COUNT(*) AS count").Group("COALESCE(device_type, ''), COALESCE(device_role, ''), COALESCE(vendor, ''), COALESCE(model, '')").Order("device_type, device_role, vendor, model").Scan(&o.InventoryGroups).Error
	if err != nil {
		return o, err
	}
	groups := network.DeviceGroup{Service: s.Service}
	o.DeviceGroups, err = groups.Tree(ctx, p)
	return o, err
}

func (c Config) configured(profile string) bool {
	u, p, _ := c.credentials(profile)
	return u != "" && p != "" && (c.HostKeyPolicy == "" || c.HostKeyPolicy == "strict" || c.HostKeyPolicy == "accept_new")
}
