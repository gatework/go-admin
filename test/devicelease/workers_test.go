//go:build integration

package devicelease_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	native "github.com/gatework/firemon/device"
	"github.com/gatework/go-net-connector/connector"
	wl "github.com/gatework/go-wireless"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	base "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"github.com/google/uuid"
	dm "go-admin/app/delivery/models"
	ds "go-admin/app/delivery/service"
	dd "go-admin/app/delivery/service/dto"
	fm "go-admin/app/fabric/models"
	fs "go-admin/app/fabric/service"
	fd "go-admin/app/fabric/service/dto"
	fwmodel "go-admin/app/firewall/models"
	fw "go-admin/app/firewall/service"
	nm "go-admin/app/network/models"
	wm "go-admin/app/wireless/models"
	ws "go-admin/app/wireless/service"
	wd "go-admin/app/wireless/service/dto"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMain(m *testing.M) {
	if os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN") == "" {
		fmt.Fprintln(os.Stderr, "devicelease integration requires GO_ADMIN_TEST_POSTGRES_DSN")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func database(t *testing.T) (*gorm.DB, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN")
	open := func(dsn string) *gorm.DB {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal("open PostgreSQL:", err)
		}
		p, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		p.SetMaxOpenConns(4)
		return db
	}
	admin := open(dsn)
	schema := "lease_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	// Each pool connection gets search_path at startup, not via one pooled SET.
	a, b := open(dsn+" search_path="+schema), open(dsn+" search_path="+schema)
	t.Cleanup(func() {
		for _, db := range []*gorm.DB{a, b} {
			p, _ := db.DB()
			p.Close()
		}
		if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Error(err)
		}
		p, _ := admin.DB()
		p.Close()
	})
	tables := append(ds.Tables(), ws.Tables()...)
	tables = append(tables, &nm.Device{}, &nm.DeviceGroup{}, &fm.Fabric{}, &fm.RuleSet{}, &fm.Membership{}, &fm.Observation{}, &fm.Collection{}, &fm.Change{})
	if err := a.AutoMigrate(tables...); err != nil {
		t.Fatal(err)
	}
	if err := fwmodel.Migrate(a); err != nil {
		t.Fatal(err)
	}
	return a, b
}

type barrier struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int32
	once    sync.Once
	unblock sync.Once
	closed  bool
}

func newBarrier(t *testing.T, closed bool) *barrier {
	b := &barrier{entered: make(chan struct{}), release: make(chan struct{}), closed: closed}
	t.Cleanup(b.finish)
	return b
}
func (b *barrier) finish() { b.unblock.Do(func() { close(b.release) }) }
func (b *barrier) wait(ctx context.Context) {
	b.calls.Add(1)
	b.once.Do(func() { close(b.entered) })
	select {
	case <-ctx.Done():
	case <-b.release:
	}
}
func entered(t *testing.T, b *barrier) {
	t.Helper()
	select {
	case <-b.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("worker never entered fake transport")
	}
}
func done(t *testing.T, ch <-chan error) {
	t.Helper()
	select {
	case err := <-ch:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not finish")
	}
}

type session struct{ b *barrier }

func (s session) Connect(ctx context.Context) error {
	s.b.wait(ctx)
	return errors.New("synthetic login failure")
}
func (s session) Execute(context.Context, connector.Script, ds.ExecutionHooks) (string, error) {
	return "", errors.New("unexpected execution")
}
func (s session) Close() error {
	if !s.b.closed {
		return errors.New("synthetic close failure")
	}
	return nil
}

type collector struct{ b *barrier }

func (c collector) Collect(ctx context.Context, _ nm.Device, _ dm.Connection, _ wm.Collection) (wl.Snapshot, bool, error) {
	c.b.wait(ctx)
	return wl.Snapshot{}, c.b.closed, errors.New("synthetic collection failure")
}

type transport struct{ b *barrier }

func (t transport) Open(ctx context.Context, _ dm.DeviceSnapshot, _ dm.Connection, _ func(fs.FabricSession) error) error {
	t.b.wait(ctx)
	return receipt{t.b.closed}
}

type receipt struct{ closed bool }

func (receipt) Error() string           { return "synthetic collection failure" }
func (r receipt) TransportClosed() bool { return r.closed }

type reader struct{ closed bool }

func (reader) Read(context.Context, native.View) (native.Capture, error) {
	return native.Capture{}, errors.New("connection test must not read")
}
func (r reader) Close() error {
	if !r.closed {
		return errors.New("synthetic close failure")
	}
	return nil
}

var permission = &actions.DataPermission{DataScope: actions.DataScopeAll, UserId: 7, RoleId: 1}

func fixture(t *testing.T, kind string, a, b *gorm.DB, gate *barrier) (*ds.Worker, func(context.Context) error) {
	t.Helper()
	t.Setenv("NC_DEVICE_USERNAME", "fixture")
	t.Setenv("NC_DEVICE_PASSWORD", "fixture")
	t.Setenv("NETWORK_DELIVERY_ENABLED", "true")
	profile, vendor, osName, typ, role := "h3c", "H3C", "Comware", "wireless_controller", ""
	if kind == "fabric" {
		profile, vendor, osName, typ, role = "cisco_nxos", "Cisco", "NX-OS", "switch", "leaf_switch"
	}
	if kind == "firewall" {
		profile, vendor, osName, typ, role = "hillstone", "Hillstone", "StoneOS", "firewall", "firewall"
	}
	d := nm.Device{SourceFacts: nm.SourceFacts{IP: "192.0.2.1", Name: "SZX9-LEF-1", Vendor: vendor, OS: osName, SourceTags: []string{}}, DeviceType: typ, DeviceRole: role, Status: "active", Version: 1, Tags: []string{}}
	d.CreateBy = 7
	if err := a.Create(&d).Error; err != nil {
		t.Fatal(err)
	}
	if err := a.Create(&dm.Connection{DeviceID: d.Id, Profile: profile, Protocol: "ssh", Port: 22, Version: 1}).Error; err != nil {
		t.Fatal(err)
	}
	flow := &ds.Workflow{Service: base.Service{Orm: a}, Config: ds.Config{Enabled: true, Username: "fixture", Password: "fixture", Grace: time.Minute, Concurrency: 2}}
	order, err := flow.Create(context.Background(), dd.Create{RequestKey: "lease-delivery-request", Title: "lease test", Targets: []dd.InputTarget{{DeviceID: d.Id, Script: "show version"}}}, 7, "fixture", permission)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = flow.Action(context.Background(), order.ID, order.Version, "execute", 7, permission); err != nil {
		t.Fatal(err)
	}
	worker := ds.NewWorker(flow, func(dm.Target) (ds.Session, error) { return session{gate}, nil })
	switch kind {
	case "wireless":
		s := &ws.Wireless{Service: base.Service{Orm: b}}
		if _, err := s.CreateCollection(context.Background(), d.Id, wd.CollectionReq{RequestKey: "lease-wireless-request"}, 7, permission); err != nil {
			t.Fatal(err)
		}
		return worker, ws.NewWorker(s, collector{gate}).Tick
	case "fabric":
		for _, row := range []any{&fm.Fabric{ID: 1, Code: "SZX9", Version: 1, RulesVersion: 1, RuntimeMinutes: 5, ConfigMinutes: 30}, &fm.RuleSet{ID: 1, Version: 1, Rules: []fm.Rule{{FabricID: 1, Pattern: "SZX9", Enabled: true}}}} {
			if err := b.Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		s := &fs.Fabric{Service: base.Service{Orm: b}}
		if _, err := s.CreateCollection(context.Background(), 1, &fd.CollectionCreateReq{RequestKey: "lease-fabric-request", Mode: "all"}, 7, permission); err != nil {
			t.Fatal(err)
		}
		return worker, fs.NewWorker(s, transport{gate}).Tick
	default:
		if err := b.Create(&fwmodel.Domain{ID: 1, Code: "default", Name: "fixture", Version: 1}).Error; err != nil {
			t.Fatal(err)
		}
		s := &fw.Firewall{Service: base.Service{Orm: b}, Actor: 7, Root: t.TempDir()}
		instance, err := s.CreateInstance(context.Background(), fw.InstanceInput{DeviceID: d.Id, DomainID: 1, Vendor: "hillstone", Mode: "routed"}, permission)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.EnqueueCollection(context.Background(), fw.CollectionInput{RequestKey: "lease-firewall-request", Kind: "connection_test", InstanceIDs: []int{instance.ID}}, permission); err != nil {
			t.Fatal(err)
		}
		return worker, fw.NewCollectionWorker(s, func(ctx context.Context, _ fw.CollectionSource) (native.Reader, error) {
			gate.wait(ctx)
			return reader{gate.closed}, nil
		}).Tick
	}
}

func TestWorkersSharePhysicalOwnership(t *testing.T) {
	for _, kind := range []string{"wireless", "fabric", "firewall"} {
		for _, deliveryFirst := range []bool{false, true} {
			for _, closed := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/delivery_first=%t/closed=%t", kind, deliveryFirst, closed), func(t *testing.T) {
					a, b := database(t)
					gate := newBarrier(t, closed)
					delivery, peer := fixture(t, kind, a, b, gate)
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					finished := make(chan error, 1)
					ended := make(chan struct{})
					if deliveryFirst {
						target, err := delivery.Claim(ctx)
						if err != nil || target == nil {
							t.Fatalf("delivery claim: %v", err)
						}
						go func() { defer close(ended); delivery.RunTarget(ctx, *target); finished <- nil }()
					} else {
						go func() { defer close(ended); finished <- peer(ctx) }()
					}
					t.Cleanup(func() {
						gate.finish()
						select {
						case <-ended:
						case <-time.After(10 * time.Second):
						}
					})
					entered(t, gate)
					var owner dm.DeviceLease
					if err := a.First(&owner, "device_id > 0").Error; err != nil {
						t.Fatal(err)
					}
					if deliveryFirst {
						if err := peer(ctx); err != nil {
							t.Fatal(err)
						}
					} else {
						target, err := delivery.Claim(ctx)
						if err != nil || target != nil {
							t.Fatalf("competing delivery acquired: %v %v", target, err)
						}
					}
					if gate.calls.Load() != 1 {
						t.Fatal("second worker opened transport")
					}
					var after dm.DeviceLease
					if err := b.First(&after, owner.DeviceID).Error; err != nil || after.Token != owner.Token {
						t.Fatal("competing worker changed owner", err)
					}
					gate.finish()
					done(t, finished)
					var count int64
					if err := a.Model(&dm.DeviceLease{}).Where("device_id = ?", owner.DeviceID).Count(&count).Error; err != nil {
						t.Fatal(err)
					}
					if closed && count != 0 {
						t.Fatal("confirmed close retained lease")
					}
					if !closed {
						if err := a.First(&after, owner.DeviceID).Error; err != nil || !after.Quarantined {
							t.Fatal("unknown close was not quarantined", err)
						}
						if target, err := delivery.Claim(ctx); err != nil || target != nil {
							t.Fatal("quarantined device reacquired", err)
						}
					}
				})
			}
		}
	}
}
