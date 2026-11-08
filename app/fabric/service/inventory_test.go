package service

import (
	"context"
	"testing"
	"time"

	vxlan "github.com/gatework/go-vxlan"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	common "github.com/go-admin-team/go-admin-core/v2/sdk/contract/models"
	base "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	delivery "go-admin/app/delivery/models"
	"go-admin/app/fabric/models"
	"go-admin/app/fabric/service/dto"
	network "go-admin/app/network/models"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func coreService(t *testing.T) *Fabric {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	if err := db.AutoMigrate(&network.Device{}, &models.Fabric{}, &models.RuleSet{}, &models.Membership{}, &models.Observation{}, &models.Assessment{}, &delivery.Connection{}); err != nil {
		t.Fatal(err)
	}
	f := models.Fabric{ID: 1, Code: "SZX9", Name: "Test", Version: 1, RulesVersion: 1, RuntimeMinutes: 5, ConfigMinutes: 30}
	if err := db.Create(&f).Error; err != nil {
		t.Fatal(err)
	}
	rules := models.RuleSet{ID: 1, Version: 1, Rules: []models.Rule{{FabricID: 1, Pattern: "SZX9", Enabled: true}}}
	if err := db.Create(&rules).Error; err != nil {
		t.Fatal(err)
	}
	return &Fabric{Service: base.Service{Orm: db}}
}
func coreDevice(t *testing.T, s *Fabric, name, ip string, owner int) network.Device {
	t.Helper()
	d := network.Device{SourceFacts: network.SourceFacts{IP: ip, Name: name, Vendor: "Cisco", OS: "NX-OS", SourceTags: []string{}}, Version: 1, DeviceRole: "leaf_switch", Status: "active", Tags: []string{}, ControlBy: common.ControlBy{CreateBy: owner}}
	if err := s.Orm.Create(&d).Error; err != nil {
		t.Fatal(err)
	}
	return d
}
func TestPartialInventoryScopeCannotClaimFabricHealthOrMutateSettings(t *testing.T) {
	s := coreService(t)
	coreDevice(t, s, "SZX9-LEF-1", "10.0.0.1", 1)
	coreDevice(t, s, "SZX9-LEF-2", "10.0.0.2", 2)
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	p := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 1}
	d, err := s.Detail(context.Background(), 1, p)
	if err != nil {
		t.Fatal(err)
	}
	if d.CompleteScope || len(d.Nodes) != 1 || d.State == "healthy" {
		t.Fatalf("partial falsely complete: %+v", d)
	}
	_, err = s.Settings(context.Background(), 1, &dto.SettingsReq{Version: 1, RuntimeMinutes: 5, ConfigMinutes: 30}, p)
	if err != ErrScope {
		t.Fatalf("partial mutated settings: %v", err)
	}
}
func TestRenamingReevaluatesAutomaticMembershipWithoutChangingManual(t *testing.T) {
	s := coreService(t)
	ctx := context.Background()
	d := coreDevice(t, s, "SZX9-LEF-1", "10.0.0.1", 1)
	p := &actions.DataPermission{DataScope: actions.DataScopeAll}
	rows, _, err := s.VisibleMembers(ctx, 1, p)
	if err != nil || len(rows) != 1 {
		t.Fatalf("initial: %v", err)
	}
	if err := s.Orm.Model(&d).Update("name", "OTHER-LEF-1").Error; err != nil {
		t.Fatal(err)
	}
	rows, _, err = s.VisibleMembers(ctx, 1, p)
	if err != nil || len(rows) != 0 {
		t.Fatalf("renamed stayed assigned: %v %v", rows, err)
	}
	id := 1
	if _, err = s.UpdateMember(ctx, d.Id, &dto.MemberReq{Mode: "manual", FabricID: &id}, 1, p); err != nil {
		t.Fatal(err)
	}
	rows, _, err = s.VisibleMembers(ctx, 1, p)
	if err != nil || len(rows) != 1 {
		t.Fatalf("manual lost: %v", err)
	}
}
func TestMatrixIncludesMissingLeafAndExplicitUnboundACL(t *testing.T) {
	config := vxlan.Record{"vlan_vnis": vxlan.Record{"10": 10010}, "interfaces": vxlan.Record{"Vlan10": vxlan.Record{"vrf": "red", "acl_bindings": vxlan.Record{}}}, "nve": vxlan.Record{"members": vxlan.Record{"10010": vxlan.Record{}}}, "evpn": vxlan.Record{"10010": vxlan.Record{"type": "l2"}}}
	rows := segmentMatrix([]dto.Node{{DeviceID: 1, IP: "10.0.0.1", Role: "leaf_switch"}, {DeviceID: 2, IP: "10.0.0.2", Role: "leaf_switch"}, {DeviceID: 3, IP: "10.0.0.3", Role: "spine_switch"}}, vxlan.Record{"10.0.0.1": config, "10.0.0.2": vxlan.Record{}})
	if len(rows) != 1 || len(rows[0].Cells) != 2 || rows[0].Cells[1].State != "missing" || rows[0].Cells[0].ACLIn != "unbound" {
		t.Fatalf("bad matrix: %+v", rows)
	}
}
func TestUnusedInterfaceDoesNotProduceFabricFault(t *testing.T) {
	config := vxlan.Record{"interfaces": vxlan.Record{"Ethernet1/1": vxlan.Record{"ospf": vxlan.Record{"process": "1"}}, "Ethernet1/48": vxlan.Record{}}}
	rows := []vxlan.Record{{"interface": "Eth1/48", "state": "notconnect"}, {"interface": "Eth1/1", "state": "up"}}
	filtered := fabricInterfaceRows(config, rows)
	if bad, _ := healthRows("interfaces", filtered); bad {
		t.Fatal("unused interface marked fault")
	}
}
func TestOutOfOrderObservationAndAgeUseAcquisitionTime(t *testing.T) {
	now := time.Now().UTC()
	rows := []models.Observation{{ID: 5, Dimension: "configuration", Status: "complete", ObservedAt: now.Add(-time.Hour)}, {ID: 1, Dimension: "configuration", Status: "partial", ObservedAt: now.Add(-time.Minute)}}
	e := DimensionEvidence(rows, "configuration", now, 10*time.Minute)
	if e.Latest.ID != 1 || e.LastComplete.ID != 5 || e.State != "unknown" {
		t.Fatalf("arrival order overrode evidence time: %+v", e)
	}
}
func TestIdentityChangeInvalidatesDimensionEvidence(t *testing.T) {
	s := coreService(t)
	ctx := context.Background()
	d := coreDevice(t, s, "SZX9-LEF-1", "10.0.0.1", 1)
	p := &actions.DataPermission{DataScope: actions.DataScopeAll}
	if _, _, err := s.VisibleMembers(ctx, 1, p); err != nil {
		t.Fatal(err)
	}
	var m models.Membership
	s.Orm.First(&m, "device_id = ?", d.Id)
	conn := delivery.Connection{DeviceID: d.Id, Profile: "cisco_nxos", Protocol: "ssh", Port: 22, Version: 1}
	s.Orm.Create(&conn)
	o := models.Observation{FabricID: 1, DeviceID: d.Id, Dimension: "configuration", Status: "complete", ObservedAt: time.Now().UTC(), MembershipVersion: m.Version, RuleVersion: m.RuleVersion, SourceHash: sourceHash(opsDevice(d), conn), Data: vxlan.Observation{Status: "complete", Records: []vxlan.Record{{"review_status": "complete"}}}}
	s.Orm.Create(&o)
	before, err := s.Detail(ctx, 1, p)
	if err != nil {
		t.Fatal(err)
	}
	if before.Nodes[0].Dimensions["configuration"].State != "complete" {
		t.Fatal("initial evidence unexpectedly rejected")
	}
	s.Orm.Model(&d).Update("ip", "10.0.0.99")
	after, err := s.Detail(ctx, 1, p)
	if err != nil {
		t.Fatal(err)
	}
	if after.Nodes[0].Dimensions["configuration"].State != "unknown" {
		t.Fatal("identity drift reused old complete evidence")
	}
	if before.EvidenceKey == after.EvidenceKey {
		t.Fatal("identity drift did not invalidate assessment key")
	}
}
func TestFailedConfigurationRetainsObservedMatrixAsUnknown(t *testing.T) {
	s := coreService(t)
	ctx := context.Background()
	d := coreDevice(t, s, "SZX9-LEF-1", "10.0.0.1", 1)
	p := &actions.DataPermission{DataScope: actions.DataScopeAll}
	s.VisibleMembers(ctx, 1, p)
	var m models.Membership
	s.Orm.First(&m, "device_id = ?", d.Id)
	c := delivery.Connection{DeviceID: d.Id, Profile: "cisco_nxos", Version: 1}
	s.Orm.Create(&c)
	parsed := vxlan.ParseConfig("hostname LEAF\nvlan 10\n  vn-segment 10010\n", true)
	if parsed.Status != "complete" {
		t.Fatalf("fixture: %s", parsed)
	}
	base := models.Observation{FabricID: 1, DeviceID: d.Id, Dimension: "configuration", Status: "complete", ObservedAt: time.Now().Add(-time.Minute), MembershipVersion: m.Version, RuleVersion: m.RuleVersion, SourceHash: sourceHash(opsDevice(d), c), Data: parsed}
	s.Orm.Create(&base)
	s.Orm.Create(&models.Observation{FabricID: 1, DeviceID: d.Id, Dimension: "configuration", Status: "failed", ObservedAt: time.Now(), MembershipVersion: m.Version, RuleVersion: m.RuleVersion, SourceHash: base.SourceHash, Data: vxlan.Observation{Status: "failed"}})
	out, err := s.Detail(ctx, 1, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Segments) != 1 || len(out.Segments[0].Cells) != 1 || out.Segments[0].Cells[0].State != "unknown" {
		t.Fatalf("failure cleared historical matrix: %+v", out.Segments)
	}
}
func TestParsedMacRemoteVTEPDestinationIsNotLocalPort(t *testing.T) {
	parsed, err := vxlan.ParseRuntime("mac_addresses", "VLAN MAC Address Type age Secure NTFY Ports\n* 110 0011.2233.4455 dynamic 0 F F nve1(192.0.2.2)\n", true)
	if err != nil || parsed.Status != "complete" {
		t.Fatalf("fixture: %s %v", parsed, err)
	}
	rows := endpointRows(models.Observation{Data: parsed}, "leaf", nil, "current")
	if len(rows) != 1 || rows[0].Kind != "remote_vtep" || rows[0].RemoteVTEP != "192.0.2.2" || rows[0].Interface != "" {
		t.Fatalf("remote VTEP classified local: %+v", rows)
	}
}
func TestEvidenceQueryBoundsHistoryToLatestAndLastComplete(t *testing.T) {
	s := coreService(t)
	now := time.Now().UTC()
	for i := 0; i < 40; i++ {
		status := "partial"
		if i < 20 {
			status = "complete"
		}
		o := models.Observation{FabricID: 1, DeviceID: 3, Dimension: "nve_peers", Status: status, ObservedAt: now.Add(time.Duration(i) * time.Minute)}
		if err := s.Orm.Create(&o).Error; err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.evidenceRows(context.Background(), 1, []int{3})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != 40 || rows[1].ID != 20 {
		t.Fatalf("unbounded or wrong evidence pointers: %+v", rows)
	}
}
