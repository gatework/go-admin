package service

import (
	"context"
	"errors"
	"testing"

	delivery "go-admin/app/delivery/models"
	network "go-admin/app/network/models"
)

func TestFabricEffectiveConnectionSupportsAutomaticNXOSAndRejectsChangedDriver(t *testing.T) {
	s := operationsFixture(t)
	if err := s.Orm.Where("device_id = ?", 1).Delete(&delivery.Connection{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Orm.Model(&network.Device{}).Where("id = ?", 1).Updates(map[string]any{"vendor": "Cisco", "os": "NX-OS"}).Error; err != nil {
		t.Fatal(err)
	}
	var d network.Device
	if err := s.Orm.First(&d, 1).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	targets, err := s.opsTargets(ctx, []network.Device{d})
	if err != nil || len(targets) != 1 || targets[0].Status != "pending" || targets[0].Connection.Profile != "cisco_nxos" || targets[0].Connection.Version != 0 || targets[0].Connection.Port != 22 {
		t.Fatalf("automatic fabric target: %+v %v", targets, err)
	}
	w := NewWorker(s, &fakeFabricTransport{})
	target := targets[0]
	if err = w.checkSource(ctx, target.Device, target.Connection, target.SourceHash); err != nil {
		t.Fatalf("unchanged default rejected: %v", err)
	}
	if err = s.Orm.Model(&d).Update("os", "IOS-XE").Error; err != nil {
		t.Fatal(err)
	}
	if err = w.checkSource(ctx, target.Device, target.Connection, target.SourceHash); !errors.Is(err, errOpsConflict) {
		t.Fatalf("changed inferred driver accepted: %v", err)
	}
}
