package service

import (
	"context"
	"errors"
	"github.com/gatework/go-net-connector/vendors"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	network "go-admin/app/network/service"
	ndto "go-admin/app/network/service/dto"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalid    = errors.New("delivery.invalid_input")
	ErrConflict   = errors.New("delivery.version_conflict")
	ErrNotFound   = errors.New("delivery.not_found")
	ErrDisabled   = errors.New("delivery.disabled")
	ErrConnection = errors.New("delivery.connection_not_ready")
	ErrCancelled  = errors.New("delivery.cancelled")
	ErrIdentity   = errors.New("delivery.device_changed")
)

const MaxTargets = 100
const MaxScriptBytes = 64 << 10
const MaxTotalBytes = 4 << 20
const MaxOutputBytes = 4 << 20

type Config struct {
	Enabled                                                       bool
	Username, Password, EnablePassword, HostKeyPolicy, KnownHosts string
	Grace                                                         time.Duration
	Concurrency                                                   int
}

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
func (c Config) configured(profile string) bool {
	u, p, _ := c.credentials(profile)
	return u != "" && p != "" && (c.HostKeyPolicy == "" || c.HostKeyPolicy == "strict" || c.HostKeyPolicy == "accept_new")
}

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

type InventoryGroup struct {
	DeviceType string `json:"deviceType"`
	DeviceRole string `json:"deviceRole"`
	Vendor     string `json:"vendor"`
	Model      string `json:"model"`
	Count      int64  `json:"count"`
}

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
