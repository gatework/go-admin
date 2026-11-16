package delivery

import (
	"context"
	"os"
	"strconv"
	"time"

	"github.com/gatework/go-net-connector/vendors"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	ndto "github.com/gatework/motor/app/net/service/dto/inventory"
	network "github.com/gatework/motor/app/net/service/inventory"
	errs "github.com/gatework/motor/common/errors"
)

//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var (
	// InvalidErr is a request the service refuses as malformed.
	InvalidErr = errs.New(errs.KindInvalid, "delivery.invalid_input", "")
	// ConflictErr is a write against a version that has since changed.
	ConflictErr = errs.New(errs.KindConflict, "delivery.version_conflict", "")
	// NotFoundErr is an order or target that does not exist or is out of scope.
	NotFoundErr = errs.New(errs.KindNotFound, "delivery.not_found", "")
	// DisabledErr is any delivery request while NETWORK_DELIVERY_ENABLED is off.
	DisabledErr = errs.New(errs.KindUnavailable, "delivery.disabled", "")
	// ConnectionErr is a device whose connection profile cannot be used yet:
	// no profile, an unknown platform, or credentials that are not configured.
	ConnectionErr = errs.New(errs.KindInvalid, "delivery.connection_not_ready", "")
	// CancelledErr is a target whose order was cancelled or whose lease was lost
	// while it ran.
	CancelledErr = errs.New(errs.KindConflict, "delivery.cancelled", "")
	// IdentityErr is a device whose identity changed between planning and
	// execution, so the planned script may no longer apply to it.
	IdentityErr = errs.New(errs.KindConflict, "delivery.device_changed", "")
)

//revive:enable:error-naming

// MaxTargets bounds the permitted targets.
const MaxTargets = 100

// MaxScriptBytes bounds the permitted script bytes.
const MaxScriptBytes = 64 << 10

// MaxTotalBytes bounds the permitted total bytes.
const MaxTotalBytes = 4 << 20

// MaxOutputBytes bounds what one target may read back from its device.
const MaxOutputBytes = deviceaccess.MaxOutputBytes

// Config holds delivery's switches and the device access it runs with.
type Config struct {
	Enabled     bool
	Access      deviceaccess.Config
	Grace       time.Duration
	Concurrency int
}

// FromEnvironment loads delivery's configuration.
func FromEnvironment() Config {
	c := Config{
		Enabled:     os.Getenv("NETWORK_DELIVERY_ENABLED") == "true",
		Access:      deviceaccess.FromEnvironment(),
		Grace:       600 * time.Second,
		Concurrency: 5,
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
		if c.Access.Configured(p) {
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
