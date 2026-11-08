package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"go-admin/app/delivery/models"
	"go-admin/app/delivery/service/dto"
	inventory "go-admin/app/network/models"
	ndto "go-admin/app/network/service/dto"
)

func automaticFixture(t *testing.T) *Workflow {
	t.Helper()
	s := fixture(t)
	if err := s.Orm.Where("device_id = ?", 1).Delete(&models.Connection{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Orm.Model(&inventory.Device{}).Where("id = ?", 1).Updates(map[string]any{"vendor": "H3C", "os": "Comware", "device_type": "switch", "device_role": "access_switch"}).Error; err != nil {
		t.Fatal(err)
	}
	return s
}

func jsonFields(t *testing.T, value any) map[string]any {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestAutomaticConnectionUsesInventoryWithoutWritingMetadata(t *testing.T) {
	s := automaticFixture(t)
	c, err := s.Connection(context.Background(), 1, scope())
	if err != nil || c.Profile != "h3c" || c.Protocol != "ssh" || c.Port != 22 || c.Version != 0 || !c.Ready || c.Reason != "" {
		t.Fatalf("effective connection: %+v %v", c, err)
	}
	fields := jsonFields(t, c)
	if fields["profileSource"] != "automatic" || fields["inferredProfile"] != "h3c" {
		t.Fatalf("inference metadata: %+v", fields)
	}
	var count int64
	if err = s.Orm.Model(&models.Connection{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("read materialized connection rows: %d %v", count, err)
	}
}

func TestConnectionInfersOnlySupportedUnambiguousProfiles(t *testing.T) {
	s := automaticFixture(t)
	cases := []struct{ name, vendor, kind, os, model, want string }{
		{"h3c switch", "New H3C Technologies", "switch", "Comware", "S6520", "h3c"},
		{"h3c controller type", "H3C", "wireless_controller", "Comware", "", "h3c_wireless"},
		{"h3c access point type", "H3C", "wireless_ap", "Comware", "", "h3c_wireless"},
		{"h3c controller model", "H3C", "", "Comware", "H3C WX3540H", "h3c_wireless"},
		{"h3c model without vendor", "", "switch", "Comware", "H3C S6520", "h3c"},
		{"cisco ios", "Cisco Systems", "router", "IOS", "", "cisco_ios"},
		{"cisco ios xe", "Cisco", "switch", "Cisco IOS-XE", "C9300", "cisco_ios"},
		{"cisco nexus os", "Cisco", "switch", "NX-OS", "", "cisco_nxos"},
		{"cisco nexus model", "Cisco", "switch", "", "N9K-C93180YC-FX", "cisco_nxos"},
		{"cisco catalyst model", "Cisco", "switch", "", "Catalyst 9300", "cisco_ios"},
		{"huawei", "Huawei Technologies", "switch", "VRP", "CE6865", "huawei"},
		{"hillstone", "Hillstone Networks", "firewall", "StoneOS", "", "hillstone"},
		{"radware", "Radware", "load_balancer", "Alteon OS", "Alteon 6420", "radware"},
		{"palo alto", "Palo Alto Networks", "firewall", "PAN-OS", "PA-5220", "palo_alto"},
		{"topsec", "Topsec", "firewall", "", "", "topsec"},
		{"fortinet fortios", "Fortinet", "firewall", "FortiOS", "FortiGate 200F", "fortinet"},
		{"juniper junos", "Juniper Networks", "router", "Junos", "MX204", "juniper"},
		{"centec", "Centec Networks", "switch", "", "", "centec"},
		{"unknown", "Unknown", "switch", "", "", ""},
		{"cisco ambiguous", "Cisco", "switch", "", "", ""},
		{"cisco asa", "Cisco", "firewall", "ASA", "ASA5525", ""},
		{"cisco ios xr", "Cisco", "router", "Cisco IOS XR", "ASR9000", ""},
		{"cisco iosxr", "Cisco", "router", "IOSXR", "", ""},
		{"cisco aci", "Cisco", "switch", "ACI", "N9K-C93180YC-FX", ""},
		{"conflicting cisco os and model", "Cisco", "switch", "IOS", "Nexus 9000", ""},
		{"conflicting vendor and os", "Huawei", "switch", "Junos", "", ""},
		{"h3c server", "H3C", "network_management_server", "Linux", "UniServer", ""},
		{"juniper screenos", "Juniper", "firewall", "ScreenOS", "NetScreen", ""},
		{"fortinet manager", "Fortinet", "network_management_server", "FortiManager", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := s.Orm.Model(&inventory.Device{}).Where("id = ?", 1).Updates(map[string]any{"vendor": tc.vendor, "device_type": tc.kind, "os": tc.os, "model": tc.model}).Error; err != nil {
				t.Fatal(err)
			}
			c, err := s.Connection(context.Background(), 1, scope())
			if err != nil || c.Profile != tc.want {
				t.Fatalf("profile = %q, want %q: %v", c.Profile, tc.want, err)
			}
			if tc.want == "" && (c.Ready || c.Reason != "profile_unknown" || jsonFields(t, c)["profileSource"] != "unknown") {
				t.Fatalf("unknown platform was not explicit: %+v", c)
			}
		})
	}
}

func TestManualProfileAndRestoreAutomaticRetainCAS(t *testing.T) {
	s := automaticFixture(t)
	ctx := context.Background()
	c, err := s.SetConnection(ctx, 1, dto.Connection{Version: 0, Profile: "h3c_wireless", Protocol: "ssh", Port: 2222}, 7, scope())
	if err != nil || c.Profile != "h3c_wireless" || c.Port != 2222 || c.Version != 1 || jsonFields(t, c)["profileSource"] != "manual" || jsonFields(t, c)["inferredProfile"] != "h3c" {
		t.Fatalf("override: %+v %v", c, err)
	}
	if _, err = s.SetConnection(ctx, 1, dto.Connection{Version: 0, Protocol: "ssh", Port: 22}, 7, scope()); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale restore: %v", err)
	}
	c, err = s.SetConnection(ctx, 1, dto.Connection{Version: 1, Protocol: "ssh", Port: 22}, 7, scope())
	if err != nil || c.Profile != "h3c" || c.Version != 2 || c.Port != 22 || jsonFields(t, c)["profileSource"] != "automatic" {
		t.Fatalf("restore: %+v %v", c, err)
	}
	var row models.Connection
	if err = s.Orm.First(&row, "device_id = ?", 1).Error; err != nil || row.Profile != "" || row.Version != 2 {
		t.Fatalf("automatic mode must stay empty and versioned: %+v %v", row, err)
	}
	if err = s.Orm.Model(&inventory.Device{}).Where("id = ?", 1).Update("device_type", "wireless_controller").Error; err != nil {
		t.Fatal(err)
	}
	c, err = s.Connection(ctx, 1, scope())
	if err != nil || c.Profile != "h3c_wireless" || c.Version != 2 {
		t.Fatalf("restored inference did not follow inventory: %+v %v", c, err)
	}
}

func TestAutomaticTargetsPersistGroupingAndHistoricalProfile(t *testing.T) {
	s := automaticFixture(t)
	ctx := context.Background()
	o, err := s.Create(ctx, request(), 7, "tester", scope())
	if err != nil {
		t.Fatal(err)
	}
	fields := jsonFields(t, o.Targets[0].Device)
	if fields["deviceType"] != "switch" || fields["deviceRole"] != "access_switch" || fields["profile"] != "h3c" || o.Targets[0].ConnectionVersion != 0 {
		t.Fatalf("group snapshot: %+v", o.Targets[0])
	}
	legacy := models.DeviceSnapshot{ID: 1, Name: "fixture", IP: "192.0.2.1", Vendor: "H3C"}
	if err = s.Orm.Model(&o.Targets[0]).Select("device").Updates(models.Target{Device: legacy}).Error; err != nil {
		t.Fatal(err)
	}
	if err = s.Orm.Model(&inventory.Device{}).Where("id = ?", 1).Updates(map[string]any{"vendor": "Cisco", "os": "NX-OS", "device_role": "leaf_switch"}).Error; err != nil {
		t.Fatal(err)
	}
	o, err = s.Get(ctx, o.ID, scope())
	fields = jsonFields(t, o.Targets[0].Device)
	if err != nil || fields["profile"] != "h3c" || fields["deviceRole"] != nil || fields["deviceType"] != nil {
		t.Fatalf("legacy grouping used live inventory instead of saved connection: %+v %v", fields, err)
	}
}

func TestNewSnapshotPreservesKnownEmptyRoleAndType(t *testing.T) {
	s := automaticFixture(t)
	if err := s.Orm.Model(&inventory.Device{}).Where("id = ?", 1).Updates(map[string]any{"device_role": "", "device_type": ""}).Error; err != nil {
		t.Fatal(err)
	}
	o, err := s.Create(context.Background(), request(), 7, "tester", scope())
	if err != nil {
		t.Fatal(err)
	}
	fields := jsonFields(t, o.Targets[0].Device)
	if fields["deviceRole"] != "" || fields["deviceType"] != "" {
		t.Fatalf("known unclassified values were lost: %+v", fields)
	}
}

func TestDeviceListUsesEffectiveConnections(t *testing.T) {
	s := automaticFixture(t)
	rows, count, err := s.Devices(context.Background(), &ndto.DevicePageReq{}, scope())
	if err != nil || count != 1 || len(rows) != 1 || rows[0].Connection.Profile != "h3c" || rows[0].Connection.Version != 0 {
		t.Fatalf("device defaults: %+v %d %v", rows, count, err)
	}
}

func TestSubmissionChecksSelectedGroupingBeforePersistingOrder(t *testing.T) {
	for _, tc := range []struct{ field, value, expected string }{
		{"device_type", "wireless_controller", `"expectedProfile":"h3c"`},
		{"device_type", "router", `"expectedDeviceType":"switch"`},
		{"device_role", "leaf_switch", `"expectedDeviceRole":"access_switch"`},
		{"device_role", "leaf_switch", `"expectedDeviceRole":""`},
	} {
		t.Run(tc.expected, func(t *testing.T) {
			s := automaticFixture(t)
			if err := s.Orm.Model(&inventory.Device{}).Where("id = ?", 1).Update(tc.field, tc.value).Error; err != nil {
				t.Fatal(err)
			}
			r := request()
			if err := json.Unmarshal([]byte(`[{"deviceId":1,"script":"system-view",`+tc.expected+`}]`), &r.Targets); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Create(context.Background(), r, 7, "tester", scope()); !errors.Is(err, ErrConflict) {
				t.Fatalf("stale selected grouping accepted: %v", err)
			}
			var count int64
			if err := s.Orm.Model(&models.Order{}).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("stale order persisted: %d %v", count, err)
			}
		})
	}
	t.Run("unchanged expectations", func(t *testing.T) {
		s := automaticFixture(t)
		r := request()
		if err := json.Unmarshal([]byte(`[{"deviceId":1,"script":"system-view","expectedProfile":"h3c","expectedDeviceRole":"access_switch","expectedDeviceType":"switch"}]`), &r.Targets); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Create(context.Background(), r, 7, "tester", scope()); err != nil {
			t.Fatalf("unchanged selected grouping rejected: %v", err)
		}
	})
}

func TestExecutionRechecksAutomaticProfileBeforeEveryWrite(t *testing.T) {
	for _, changeAt := range []string{"unchanged", "before_login", "after_login", "after_first_write"} {
		t.Run(changeAt, func(t *testing.T) {
			s := automaticFixture(t)
			ctx := context.Background()
			o := queued(t, s)
			change := func() error {
				return s.Orm.Model(&inventory.Device{}).Where("id = ?", 1).Update("device_type", "wireless_controller").Error
			}
			sends, opens := 0, 0
			f := &fakeSession{connect: func(context.Context) error {
				opens++
				if changeAt == "after_login" {
					return change()
				}
				return nil
			}, send: func(context.Context, string) (string, error) {
				sends++
				if changeAt == "after_first_write" {
					return "acknowledged", change()
				}
				return "acknowledged", nil
			}}
			w := NewWorker(s, func(models.Target) (Session, error) { return f, nil })
			target, err := w.Claim(ctx)
			if err != nil || target == nil {
				t.Fatalf("claim: %+v %v", target, err)
			}
			if changeAt == "before_login" {
				if err = change(); err != nil {
					t.Fatal(err)
				}
			}
			w.RunTarget(ctx, *target)
			out, err := s.Target(ctx, o.ID, target.ID, scope())
			wantStatus, wantSends := "failed", 0
			if changeAt == "unchanged" {
				wantStatus, wantSends = "succeeded", 2
			}
			if changeAt == "after_first_write" {
				wantStatus, wantSends = "uncertain", 1
			}
			if err != nil || out.Status != wantStatus || sends != wantSends || (changeAt == "before_login" && opens != 0) {
				t.Fatalf("profile recheck: %+v sends=%d opens=%d %v", out, sends, opens, err)
			}
		})
	}
}

func TestExecutionRejectsRoleChangeWithoutChangingDriver(t *testing.T) {
	s := automaticFixture(t)
	ctx := context.Background()
	o := queued(t, s)
	w := NewWorker(s, func(models.Target) (Session, error) {
		t.Error("role changed after authoring, transport must not be opened")
		return &fakeSession{}, nil
	})
	target, err := w.Claim(ctx)
	if err != nil || target == nil {
		t.Fatalf("claim: %+v %v", target, err)
	}
	if err = s.Orm.Model(&inventory.Device{}).Where("id = ?", 1).Update("device_role", "core_switch").Error; err != nil {
		t.Fatal(err)
	}
	w.RunTarget(ctx, *target)
	out, err := s.Target(ctx, o.ID, target.ID, scope())
	if err != nil || out.Status != "failed" || out.ErrorCode != "device_changed" || out.Attempted {
		t.Fatalf("role drift was executed: %+v %v", out, err)
	}
}

func failedAutomaticOrder(t *testing.T, s *Workflow) models.Order {
	t.Helper()
	ctx := context.Background()
	o := queued(t, s)
	w := NewWorker(s, func(models.Target) (Session, error) {
		return &fakeSession{connect: func(context.Context) error { return errors.New("offline login failure") }}, nil
	})
	target, err := w.Claim(ctx)
	if err != nil || target == nil {
		t.Fatalf("claim: %+v %v", target, err)
	}
	w.RunTarget(ctx, *target)
	o, err = s.Get(ctx, o.ID, scope())
	if err != nil || !o.Retryable || o.Targets[0].Attempted {
		t.Fatalf("failed-before-write fixture: %+v %v", o, err)
	}
	return o
}

func TestRetryPreservesOriginalIdentityAndGrouping(t *testing.T) {
	for _, tc := range []struct{ field, value string }{
		{"device_type", "wireless_controller"}, {"device_role", "core_switch"}, {"ip", "192.0.2.2"}, {"hostname", "replacement"}, {"vendor", "Huawei"}, {"model", "replacement"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			s := automaticFixture(t)
			o := failedAutomaticOrder(t, s)
			if err := s.Orm.Model(&inventory.Device{}).Where("id = ?", 1).Update(tc.field, tc.value).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := s.Retry(context.Background(), o.ID, dto.Action{Version: o.Version, RequestKey: "retry-selected-identity"}, 7, "tester", scope()); !errors.Is(err, ErrConflict) {
				t.Fatalf("retry blessed changed %s: %v", tc.field, err)
			}
		})
	}
}

func TestRetryRejectsUnknownLegacyGroupingButRecoversExistingRequest(t *testing.T) {
	s := automaticFixture(t)
	ctx := context.Background()
	o := failedAutomaticOrder(t, s)
	action := dto.Action{Version: o.Version, RequestKey: "retry-idempotency-identity"}
	retried, err := s.Retry(ctx, o.ID, action, 7, "tester", scope())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Orm.Model(&inventory.Device{}).Where("id = ?", 1).Update("device_role", "core_switch").Error; err != nil {
		t.Fatal(err)
	}
	again, err := s.Retry(ctx, o.ID, action, 7, "tester", scope())
	if err != nil || again.ID != retried.ID {
		t.Fatalf("lost-response recovery blocked by later inventory change: %+v %v", again, err)
	}
	legacy := models.DeviceSnapshot{ID: 1, Name: "fixture", IP: "192.0.2.1", Vendor: "H3C"}
	if err = s.Orm.Model(&o.Targets[0]).Select("device").Updates(models.Target{Device: legacy}).Error; err != nil {
		t.Fatal(err)
	}
	action.RequestKey = "retry-legacy-no-grouping"
	if _, err = s.Retry(ctx, o.ID, action, 7, "tester", scope()); !errors.Is(err, ErrConflict) {
		t.Fatalf("unknown legacy grouping silently accepted: %v", err)
	}
}
