package service

import (
	"context"
	"github.com/gatework/go-net-connector/connector"
	"github.com/gatework/go-net-connector/vendors"
	wireless "github.com/gatework/go-wireless"
	"github.com/gatework/go-wireless/adapter"
	delivery "go-admin/app/delivery/models"
	ds "go-admin/app/delivery/service"
	network "go-admin/app/network/models"
	"go-admin/app/wireless/models"
	"os"
	"time"
)

type Collector interface {
	Collect(context.Context, network.Device, delivery.Connection, models.Collection) (wireless.Snapshot, bool, error)
}
type nativeCollector struct{}

func (nativeCollector) Collect(ctx context.Context, d network.Device, c delivery.Connection, j models.Collection) (snapshot wireless.Snapshot, closed bool, err error) {
	cfg, e := ds.FromEnvironment().ConnectionConfig(deviceSnapshot(d), c)
	if e != nil {
		return snapshot, true, e
	}
	// Large controllers return several MiB of verbose client records. Keep a
	// bounded read budget within the worker's overall 14-minute deadline.
	cfg.CommandTimeout = 3 * time.Minute
	cfg.MaxOutputBytes = 16 << 20
	cfg.MaxScriptOutputBytes = 16 << 20
	vendor, e := vendors.NewBuiltinRegistry().Lookup(c.Profile)
	if e != nil {
		return snapshot, true, e
	}
	device, e := connector.New(vendor.Profile(), cfg)
	if e != nil {
		return snapshot, true, e
	}
	var borrowed *adapter.Session
	defer func() {
		closeErr := device.Close()
		closed = closeErr == nil && (borrowed == nil || borrowed.CloseFailure() == nil)
		if closeErr != nil {
			err = wireless.NewFailure("session_close_failed", "close")
		}
	}()
	collector := wireless.NewCollector(wireless.CollectorOptions{APFactory: adapter.APFactory(time.Now)})
	snapshot, err = collector.Collect(ctx, wireless.CollectOptions{Target: wireless.Record{"controllerId": d.Id}, Platform: platform(c), Model: d.HardwareModel, ConnectorPlatform: c.Profile, Dimensions: j.Dimensions, APNames: j.APNames, APCredentials: func(context.Context) (wireless.APCredentials, error) {
		return wireless.APCredentials{Username: os.Getenv("WIRELESS_AP_USERNAME"), Password: os.Getenv("WIRELESS_AP_PASSWORD")}, nil
	}, Open: func(ctx context.Context, run func(wireless.Session) error) error {
		if e := device.Connect(ctx); e != nil {
			return e
		}
		borrowed, e = adapter.Borrow(device)
		if e != nil {
			return e
		}
		return run(borrowed)
	}})
	return
}
