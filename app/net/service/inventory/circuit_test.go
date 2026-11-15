package inventory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/inventory"
	"github.com/gatework/motor/app/net/service/inventory/provider"
	"github.com/gatework/motor/common/errkind"
	"github.com/gatework/motor/internal/testconfig"
)

type circuitSource struct {
	configured bool
	err        error
	hosts      map[string]provider.ZabbixHost
	items      map[string]provider.ZabbixItem
	history    map[string][]provider.ZabbixSample
	trends     map[string][]provider.ZabbixTrend
	partial    bool
}

func (s *circuitSource) Configured() bool { return s.configured }
func (s *circuitSource) Hosts(context.Context, string) (provider.ZabbixHostList, error) {
	return provider.ZabbixHostList{}, s.err
}
func (s *circuitSource) Items(context.Context, string, string) (provider.ZabbixItemList, error) {
	return provider.ZabbixItemList{}, s.err
}
func (s *circuitSource) Host(_ context.Context, id string) (provider.ZabbixHost, error) {
	if s.err != nil {
		return provider.ZabbixHost{}, s.err
	}
	h, ok := s.hosts[id]
	if !ok {
		return h, inventory.CircuitBindingErr
	}
	return h, nil
}
func (s *circuitSource) Item(_ context.Context, id string) (provider.ZabbixItem, error) {
	if s.err != nil {
		return provider.ZabbixItem{}, s.err
	}
	i, ok := s.items[id]
	if !ok {
		return i, inventory.CircuitBindingErr
	}
	return i, nil
}
func (s *circuitSource) History(_ context.Context, id, stringType string, from, till int64) ([]provider.ZabbixSample, bool, error) {
	return s.history[id], s.partial, s.err
}
func (s *circuitSource) Trends(_ context.Context, id string, from, till int64) ([]provider.ZabbixTrend, bool, error) {
	return s.trends[id], s.partial, s.err
}

func newCircuitService(t *testing.T) *Circuit {
	t.Helper()
	d := testService(t)
	if err := d.DB.AutoMigrate(&inventory.Circuit{}); err != nil {
		t.Fatal(err)
	}
	return &Circuit{DB: d.DB, Source: &circuitSource{}}
}
func circuitDraft(no string) *dto.CircuitControlReq {
	return &dto.CircuitControlReq{CircuitNo: no, Name: "Branch", Provider: "Carrier"}
}
func knownCircuitSource() *circuitSource {
	return &circuitSource{configured: true, hosts: map[string]provider.ZabbixHost{"1": {ID: "1", Name: "Router", Host: "router"}}, items: map[string]provider.ZabbixItem{
		"10": {ID: "10", HostID: "1", Name: "Receive", Units: "bps", ValueType: "0", Status: "0"},
		"11": {ID: "11", HostID: "1", Name: "Transmit", Units: "Bps", ValueType: "3", Status: "0"},
		"20": {ID: "20", HostID: "2", Name: "Other host", Units: "bps", ValueType: "3", Status: "0"},
	}, history: map[string][]provider.ZabbixSample{}, trends: map[string][]provider.ZabbixTrend{}}
}

func TestCircuitCRUDCASLifecycleAndNumberNeverReused(t *testing.T) {
	s := newCircuitService(t)
	ctx := context.Background()
	row, err := s.Create(ctx, circuitDraft(" c-01 "), 7, fullScope())
	if err != nil || row.Version != 1 || row.CircuitNo != "C-01" {
		t.Fatalf("create: %+v %v", row, err)
	}
	draft := dto.CircuitControlReq(row.CircuitFields)
	draft.Name = "Edited"
	updated, err := s.Update(ctx, row.ID, &draft, 8, fullScope())
	if err != nil || updated.Version != 2 || updated.CreateBy != 7 || updated.UpdateBy != 8 {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if _, err = s.Update(ctx, row.ID, &draft, 9, fullScope()); !errors.Is(err, inventory.ConflictErr) {
		t.Fatalf("stale update: %v", err)
	}
	if err = s.Delete(ctx, row.ID, 1, 9, fullScope()); !errors.Is(err, inventory.ConflictErr) {
		t.Fatalf("stale delete: %v", err)
	}
	if err = s.Delete(ctx, row.ID, 2, 9, fullScope()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create(ctx, circuitDraft("C-01"), 7, fullScope()); !errors.Is(err, inventory.CircuitDuplicateErr) {
		t.Fatalf("reused deleted number: %v", err)
	}
	draft = *circuitDraft("C-02")
	draft.Status = "terminated"
	row, err = s.Create(ctx, &draft, 7, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	draft.Version = row.Version
	draft.Status = "active"
	if _, err = s.Update(ctx, row.ID, &draft, 7, fullScope()); !errors.Is(err, inventory.CircuitLifecycleErr) {
		t.Fatalf("terminated restored: %v", err)
	}
	if err = s.Delete(ctx, row.ID, row.Version, 7, fullScope()); !errors.Is(err, inventory.CircuitLifecycleErr) {
		t.Fatalf("terminated deleted: %v", err)
	}
}

func TestCircuitDuplicateValidationPreservesVersionForCorrectedRetry(t *testing.T) {
	s := newCircuitService(t)
	ctx := context.Background()
	reserved, err := s.Create(ctx, circuitDraft("RESERVED"), 7, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	editable, err := s.Create(ctx, circuitDraft("EDITABLE"), 7, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	for _, deleted := range []bool{false, true} {
		if deleted {
			if err := s.Delete(ctx, reserved.ID, reserved.Version, 7, fullScope()); err != nil {
				t.Fatal(err)
			}
		}
		_, err := s.Create(ctx, circuitDraft(" reserved "), 7, fullScope())
		if kind, _ := errkind.Of(err); !errors.Is(err, inventory.CircuitDuplicateErr) || kind != errkind.Invalid {
			t.Errorf("create duplicate deleted=%v: %v", deleted, err)
		}
		draft := draftFromCircuit(editable)
		draft.CircuitNo, draft.Name = "RESERVED", "must not persist"
		_, err = s.Update(ctx, editable.ID, draft, 8, fullScope())
		if kind, _ := errkind.Of(err); !errors.Is(err, inventory.CircuitDuplicateErr) || kind != errkind.Invalid {
			t.Errorf("update duplicate deleted=%v: %v", deleted, err)
		}
		current, err := s.Get(ctx, editable.ID, fullScope())
		if err != nil || current.Version != 1 || current.CircuitNo != "EDITABLE" || current.Name != "Branch" || current.UpdateBy != 7 {
			t.Fatalf("failed duplicate changed stored record: %+v %v", current, err)
		}
	}
	draft := draftFromCircuit(editable)
	draft.CircuitNo = "CORRECTED"
	updated, err := s.Update(ctx, editable.ID, draft, 8, fullScope())
	if err != nil || updated.Version != 2 || updated.CircuitNo != "CORRECTED" {
		t.Fatalf("corrected retry failed: %+v %v", updated, err)
	}
	_, err = s.Update(ctx, editable.ID, draft, 8, fullScope())
	if kind, _ := errkind.Of(err); !errors.Is(err, inventory.ConflictErr) || kind != errkind.Conflict {
		t.Fatalf("stale CAS no longer uses 409: %v", err)
	}
}

func TestCircuitScopesOwnershipOptionsExportAndNilFailClosed(t *testing.T) {

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	s := newCircuitService(t)
	ctx := context.Background()
	self := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 7}
	visible, err := s.Create(ctx, circuitDraft("C-1"), 7, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	draft := circuitDraft("C-2")
	draft.Provider = "Hidden carrier"
	hidden, err := s.Create(ctx, draft, 8, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	rows, count, err := s.Page(ctx, &dto.CircuitPageReq{}, self)
	if err != nil || count != 1 || len(rows) != 1 || rows[0].ID != visible.ID {
		t.Fatalf("scope: %+v %d %v", rows, count, err)
	}
	if _, err = s.Get(ctx, hidden.ID, self); !errors.Is(err, inventory.CircuitNotFoundErr) {
		t.Fatalf("hidden read: %v", err)
	}
	draft = draftFromCircuit(hidden)
	draft.Name = "forbidden"
	if _, err = s.Update(ctx, hidden.ID, draft, 7, self); !errors.Is(err, inventory.CircuitNotFoundErr) {
		t.Fatalf("hidden update: %v", err)
	}
	if err = s.Delete(ctx, hidden.ID, hidden.Version, 7, self); !errors.Is(err, inventory.CircuitNotFoundErr) {
		t.Fatalf("hidden delete: %v", err)
	}
	options, err := s.Options(ctx, self)
	if err != nil || len(options.Providers) != 1 || options.Providers[0] != "Carrier" {
		t.Fatalf("options: %+v %v", options, err)
	}
	data, err := s.Export(ctx, &dto.CircuitPageReq{}, self)
	if err != nil || strings.Contains(string(data), "Hidden carrier") || !strings.HasPrefix(string(data), "\ufeff") {
		t.Fatalf("export scope: %s %v", data, err)
	}
	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = false
	})
	_, count, err = s.Page(ctx, &dto.CircuitPageReq{}, nil)
	if err != nil || count != 0 {
		t.Fatalf("nil permission opened reads: %d %v", count, err)
	}
	if _, err = s.Create(ctx, circuitDraft("C-3"), 7, nil); !errors.Is(err, inventory.CircuitScopeErr) {
		t.Fatalf("nil permission created: %v", err)
	}
	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})
	device := inventory.Device{SourceFacts: fact("192.0.2.1", "hidden"), Tags: []string{}}
	device.SetCreateBy(8)
	if err = s.DB.Create(&device).Error; err != nil {
		t.Fatal(err)
	}
	draft = circuitDraft("C-3")
	draft.AEndpoint.DeviceID = &device.ID
	if _, err = s.Create(ctx, draft, 7, self); !errors.Is(err, inventory.CircuitDeviceErr) {
		t.Fatalf("hidden device linked: %v", err)
	}
}

func draftFromCircuit(c inventory.Circuit) *dto.CircuitControlReq {
	r := dto.CircuitControlReq(c.CircuitFields)
	return &r
}

func TestCircuitChangedBindingValidatesAtomicallyAndUnchangedSurvivesOutage(t *testing.T) {
	s := newCircuitService(t)
	source := knownCircuitSource()
	s.Source = source
	ctx := context.Background()
	draft := circuitDraft("C-1")
	draft.AEndpoint.Monitoring = inventory.CircuitBinding{HostID: "1", InItemID: "10", HostName: "forged", InItemName: "forged"}
	row, err := s.Create(ctx, draft, 7, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	if row.AEndpoint.Monitoring.HostName != "Router" || row.AEndpoint.Monitoring.InItemName != "Receive" {
		t.Fatalf("forged snapshot persisted: %+v", row)
	}
	draft = draftFromCircuit(row)
	draft.Name = "must not persist"
	draft.AEndpoint.Monitoring.OutItemID = "20"
	if _, err = s.Update(ctx, row.ID, draft, 7, fullScope()); !errors.Is(err, inventory.CircuitBindingErr) {
		t.Fatalf("cross-host item accepted: %v", err)
	}
	unchanged, err := s.Get(ctx, row.ID, fullScope())
	if err != nil || unchanged.Name != "Branch" || unchanged.Version != 1 {
		t.Fatalf("partial write: %+v %v", unchanged, err)
	}
	for _, units := range []string{"", "bytes", "%"} {
		source.items["11"] = provider.ZabbixItem{ID: "11", HostID: "1", Name: "bad", Units: units, ValueType: "3", Status: "0"}
		draft.AEndpoint.Monitoring.OutItemID = "11"
		if _, err = s.Update(ctx, row.ID, draft, 7, fullScope()); !errors.Is(err, inventory.CircuitBindingErr) {
			t.Fatalf("bad units %q: %v", units, err)
		}
	}
	source.err = inventory.ZabbixUpstreamErr
	draft = draftFromCircuit(row)
	draft.Name = "Edited offline"
	draft.AEndpoint.Monitoring.HostName = "forged again"
	row, err = s.Update(ctx, row.ID, draft, 7, fullScope())
	if err != nil || row.Name != "Edited offline" || row.AEndpoint.Monitoring.HostName != "Router" {
		t.Fatalf("unchanged binding failed: %+v %v", row, err)
	}
	draft = draftFromCircuit(row)
	draft.AEndpoint.Monitoring = inventory.CircuitBinding{}
	row, err = s.Update(ctx, row.ID, draft, 7, fullScope())
	if err != nil || row.AEndpoint.Monitoring.HostID != "" {
		t.Fatalf("clear binding failed: %+v %v", row, err)
	}
}

func TestCircuitExportEscapesFormulaAndRejectsMoreThan10000(t *testing.T) {
	s := newCircuitService(t)
	ctx := context.Background()
	draft := circuitDraft("C-1")
	draft.Name = "=SUM(1,2)"
	if _, err := s.Create(ctx, draft, 7, fullScope()); err != nil {
		t.Fatal(err)
	}
	data, err := s.Export(ctx, &dto.CircuitPageReq{}, fullScope())
	if err != nil || !strings.Contains(string(data), "'=SUM") {
		t.Fatalf("unsafe export: %s %v", data, err)
	}
	if err = s.DB.Exec(`WITH RECURSIVE nums(n) AS (SELECT 2 UNION ALL SELECT n+1 FROM nums WHERE n <= 10000) INSERT INTO sys_network_circuit(circuit_no,name,provider,category,purpose,status,payer,currency,version,deleted_at) SELECT 'C-'||n,'n','p','other','other','planned','us','CNY',1,0 FROM nums`).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = s.Export(ctx, &dto.CircuitPageReq{}, fullScope()); !errors.Is(err, inventory.CircuitExportLimitErr) {
		t.Fatalf("oversize export silently truncated: %v", err)
	}
}
