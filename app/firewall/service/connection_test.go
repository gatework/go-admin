package service

import (
	"context"
	"errors"
	"testing"

	delivery "go-admin/app/delivery/models"
	ds "go-admin/app/delivery/service"
	"go-admin/app/network/connection"
	network "go-admin/app/network/models"
)

func collectionCredentials(t *testing.T, s *Firewall) {
	t.Helper()
	t.Setenv("NC_DEVICE_USERNAME", "fixture-user")
	t.Setenv("NC_DEVICE_PASSWORD", "fixture-password")
	t.Setenv("NC_HOST_KEY_POLICY", "strict")
	t.Setenv("NC_KNOWN_HOSTS", "")
	if err := s.Orm.AutoMigrate(&delivery.Connection{}, &delivery.DeviceLease{}, &delivery.Audit{}); err != nil {
		t.Fatal(err)
	}
}

func TestPendingFirewallReadsDoNotEnableDelivery(t *testing.T) {
	s := testService(t)
	collectionCredentials(t, s)
	id := device(t, s, 7, "192.0.2.10")
	if err := s.Orm.Model(&network.Device{}).Where("id = ?", id).Update("status", "pending").Error; err != nil {
		t.Fatal(err)
	}
	c, err := s.Connection(context.Background(), id, all())
	if err != nil || !c.Ready || c.Vendor != "hillstone" || c.Profile != "hillstone" {
		t.Fatalf("read connection=%+v err=%v", c, err)
	}
	w := ds.Workflow{Service: s.Service, Config: ds.FromEnvironment()}
	write, err := w.Connection(context.Background(), id, all())
	if err != nil || write.Ready || write.Reason != "device_unavailable" {
		t.Fatalf("read admission changed delivery: %+v %v", write, err)
	}
	for _, status := range []string{"maintenance", "disabled", "retired"} {
		if err = s.Orm.Model(&network.Device{}).Where("id = ?", id).Update("status", status).Error; err != nil {
			t.Fatal(err)
		}
		c, err = s.Connection(context.Background(), id, all())
		if err != nil || c.Ready || c.Reason != "device_unavailable" {
			t.Fatalf("allowed %s: %+v %v", status, c, err)
		}
	}
}

func TestFirewallVendorResolutionRejectsContradictoryInventory(t *testing.T) {
	for _, row := range []struct{ vendor, os, model, want string }{
		{"palo alto networks", "PAN-OS", "PA-3220", "paloalto"}, {"Hillstone", "Hillstone", "SG-6000", "hillstone"},
		{"Juniper Networks", "Junos", "SRX-1500", "srx"}, {"Juniper", "ScreenOS", "SSG-140", "netscreen"},
		{"Cisco", "ASA", "ASA5525", "asa"}, {"Huawei", "VRP", "USG-6650", "huawei"}, {"Fortinet", "FortiOS", "FortiGate-100F", "fortinet"},
		{"Huawei", "PAN-OS", "USG-6650", ""}, {"Fortinet", "FortiManager", "FMG-100", ""}, {"Cisco", "FTD", "Firepower", ""},
	} {
		t.Run(row.vendor+row.os, func(t *testing.T) {
			d := network.Device{SourceFacts: network.SourceFacts{Vendor: row.vendor, OS: row.os, HardwareModel: row.model}, DeviceRole: "firewall"}
			if got := inferFirewallVendor(d); got != row.want {
				t.Fatalf("vendor=%q want=%q", got, row.want)
			}
		})
	}
}

func TestFirewallConnectionUsesSharedVersionedOptions(t *testing.T) {
	s := testService(t)
	collectionCredentials(t, s)
	id := device(t, s, 7, "192.0.2.11")
	c, err := s.SetConnection(context.Background(), id, connection.Input{Version: 0, Protocol: "ssh", Port: 2222}, all())
	if err != nil || c.Version != 1 || c.Port != 2222 {
		t.Fatal(c, err)
	}
	w := ds.Workflow{Service: s.Service, Config: ds.FromEnvironment()}
	shared, err := w.Connection(context.Background(), id, all())
	if err != nil || shared.Port != 2222 || shared.Version != 1 {
		t.Fatal(shared, err)
	}
	if _, err = s.SetConnection(context.Background(), id, connection.Input{Version: 0, Protocol: "ssh", Port: 22}, all()); !errors.Is(err, ErrConflict) {
		t.Fatal("stale connection accepted", err)
	}
	t.Setenv("NC_DEVICE_PASSWORD", "")
	c, err = s.Connection(context.Background(), id, all())
	if err != nil || c.Ready || c.Reason != "credentials_missing" {
		t.Fatal(c, err)
	}
}
