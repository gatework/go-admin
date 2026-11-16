package fabric

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	vxlan "github.com/gatework/go-vxlan"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/delivery"
	models "github.com/gatework/motor/app/net/models/fabric"
	network "github.com/gatework/motor/app/net/models/inventory"
	ds "github.com/gatework/motor/app/net/service/delivery"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
)

const gatewayConfig = "hostname test\nvrf context tenant\n  vni 50000\nvlan 8\n  vn-segment 10008\ninterface Vlan8\n  vrf member tenant\n  ip address 192.0.2.254/24\n  fabric forwarding mode anycast-gateway\n"
const missingGatewayConfig = "hostname test\nvrf context tenant\n  vni 50000\nvlan 8\n  vn-segment 10008\ninterface Vlan8\n  vrf member tenant\n  fabric forwarding mode anycast-gateway\n"

func operationsFixture(t *testing.T) *Fabric {
	t.Helper()
	t.Setenv("NETWORK_DELIVERY_ENABLED", "true")
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	tables := append(ds.Tables(), &models.Fabric{}, &models.RuleSet{}, &models.Membership{}, &models.Observation{}, &models.Collection{}, &models.Change{}, &network.Device{})
	if e = db.AutoMigrate(tables...); e != nil {
		t.Fatal(e)
	}
	for _, q := range []string{"CREATE TABLE sys_user(user_id INTEGER PRIMARY KEY,role_id INTEGER,dept_id INTEGER,status TEXT,deleted_at INTEGER DEFAULT 0)", "CREATE TABLE sys_role(role_id INTEGER PRIMARY KEY,data_scope TEXT,status TEXT,role_key TEXT,deleted_at INTEGER DEFAULT 0)", "INSERT INTO sys_user(user_id,role_id,dept_id,status) VALUES(7,1,1,'2')", "INSERT INTO sys_role(role_id,data_scope,status,role_key) VALUES(1,'1','2','admin')"} {
		if e := db.Exec(q).Error; e != nil {
			t.Fatal(e)
		}
	}
	f := models.Fabric{ID: 1, Code: "SZX9", Version: 1, RulesVersion: 1, RuntimeMinutes: 5, ConfigMinutes: 30}
	if e := db.Create(&f).Error; e != nil {
		t.Fatal(e)
	}
	rules := models.RuleSet{ID: 1, Version: 1, Rules: []models.Rule{{FabricID: 1, Pattern: "SZX9", Enabled: true}}}
	if e := db.Create(&rules).Error; e != nil {
		t.Fatal(e)
	}
	for i := 1; i <= 3; i++ {
		d := network.Device{IP: []string{"", "192.0.2.1", "192.0.2.2", "192.0.2.3"}[i], Name: "SZX9-LEF-" + string(rune('0'+i)), SourceTags: []string{}, Status: "active", DeviceRole: "leaf_switch", Tags: []string{},
			ID:       i,
			CreateBy: 7}
		if e := db.Create(&d).Error; e != nil {
			t.Fatal(e)
		}
		c := delivery.Connection{DeviceID: i, Profile: "cisco_nxos", Protocol: "ssh", Port: 22, Version: 1}
		if e := db.Create(&c).Error; e != nil {
			t.Fatal(e)
		}
		id := 1
		m := models.Membership{DeviceID: i, FabricID: &id, Mode: "auto", Status: "assigned", Version: 1, RuleVersion: 1, NameSnapshot: d.Name}
		if e := db.Create(&m).Error; e != nil {
			t.Fatal(e)
		}
	}
	return &Fabric{DB: db}
}
func operationsScope() *actions.DataPermission {
	return &actions.DataPermission{DataScope: actions.DataScopeAll, UserID: 7, RoleID: 1}
}

type fakeFabricTransport struct {
	bodies        map[int]string
	writes        []int
	failWrite     bool
	opens         []int
	after         string
	commandBodies map[string]string
}

func (f *fakeFabricTransport) Open(ctx context.Context, d delivery.DeviceSnapshot, c delivery.Connection, fn func(Session) error) error {
	f.opens = append(f.opens, d.ID)
	return fn(&fakeFabricSession{f: f, id: d.ID})
}

type fakeFabricSession struct {
	f  *fakeFabricTransport
	id int
}

func (s *fakeFabricSession) Read(ctx context.Context, command string) (vxlan.Capture, error) {
	if strings.HasPrefix(command, "terminal ") {
		return vxlan.Capture{Complete: true}, nil
	}
	if command == "show running-config" {
		body := s.f.bodies[s.id]
		if s.f.after != "" {
			if slices.Contains(s.f.writes, s.id) {
				body = s.f.after
			}
		}
		return vxlan.Capture{Body: body, Complete: true}, nil
	}
	if body, ok := s.f.commandBodies[command]; ok {
		return vxlan.Capture{Body: body, Complete: true}, nil
	}
	return vxlan.Capture{Body: "unsupported fixture runtime", Complete: true}, nil
}
func (s *fakeFabricSession) Write(ctx context.Context, command string) error {
	s.f.writes = append(s.f.writes, s.id)
	if s.f.failWrite {
		return errors.New("socket lost after send")
	}
	return nil
}
func seedConfig(t *testing.T, s *Fabric, id int, body string) {
	t.Helper()
	var d network.Device
	var c delivery.Connection
	s.DB.First(&d, id)
	s.DB.First(&c, "device_id = ?", id)
	o := vxlan.ParseConfig(body, true)
	if o.Status != "complete" {
		t.Fatalf("fixture incomplete: %+v", o.Issues)
	}
	now := time.Now().UTC()
	o.CompletedAt = &now
	row := models.Observation{FabricID: 1, DeviceID: id, Dimension: "configuration", Status: o.Status, Data: o, ObservedAt: now, Version: 1, ConfigHash: hash(o.Records), SourceHash: sourceHash(deviceSnapshot(d), c), MembershipVersion: 1, RuleVersion: 1}
	if e := s.DB.Create(&row).Error; e != nil {
		t.Fatal(e)
	}
}
func TestCollectionRoundtripPreservesPrivateTransportAndIdempotency(t *testing.T) {
	s := operationsFixture(t)
	ctx := context.Background()
	r := &dto.CollectionCreateReq{RequestKey: "collect-test-0001", Mode: "configuration"}
	a, e := s.CreateCollection(ctx, 1, r, 7, operationsScope())
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.CreateCollection(ctx, 1, r, 7, operationsScope())
	if e != nil || a.ID != b.ID {
		t.Fatalf("idempotency %v", e)
	}
	var loaded models.Collection
	if e = s.DB.First(&loaded, a.ID).Error; e != nil {
		t.Fatal(e)
	}
	if loaded.Targets[0].Device.ID != 1 || loaded.Targets[0].Connection.Version != 1 || loaded.Targets[0].SourceHash == "" {
		t.Fatalf("private provenance lost: %+v", loaded.Targets[0])
	}
	encoded, _ := json.Marshal(loaded)
	if strings.Contains(string(encoded), "192.0.2.1") || strings.Contains(string(encoded), "SourceHash") {
		t.Fatal("private provenance leaked")
	}
	fake := &fakeFabricTransport{bodies: map[int]string{1: gatewayConfig, 2: gatewayConfig, 3: gatewayConfig}}
	w := NewWorker(s, fake)
	if e = w.runCollection(ctx, loaded); e != nil {
		t.Fatal(e)
	}
	if e = s.DB.First(&loaded, a.ID).Error; e != nil {
		t.Fatal(e)
	}
	if loaded.Status != "complete" || len(fake.opens) != 3 {
		t.Fatalf("collection %s opens %v targets %+v", loaded.Status, fake.opens, loaded.Targets)
	}
	var count int64
	s.DB.Model(&models.Observation{}).Count(&count)
	if count != 6 {
		t.Fatalf("observation count %d", count)
	}
}
func TestCollectionCannotRaceDeliveryDeviceLease(t *testing.T) {
	s := operationsFixture(t)
	ctx := context.Background()
	job, e := s.CreateCollection(ctx, 1, &dto.CollectionCreateReq{RequestKey: "lease-collect-0001", Mode: "configuration"}, 7, operationsScope())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.DB.Create(&delivery.DeviceLease{DeviceID: 1, HolderID: 22, Token: "delivery", ExpiresAt: time.Now().Add(time.Minute)}).Error; e != nil {
		t.Fatal(e)
	}
	fake := &fakeFabricTransport{bodies: map[int]string{2: gatewayConfig, 3: gatewayConfig}}
	if e = NewWorker(s, fake).runCollection(ctx, job); e != nil {
		t.Fatal(e)
	}
	for _, id := range fake.opens {
		if id == 1 {
			t.Fatal("opened leased device")
		}
	}
	var lease delivery.DeviceLease
	s.DB.First(&lease, 1)
	if lease.Token != "delivery" {
		t.Fatal("stole lease")
	}
}
func previewGateway(t *testing.T, s *Fabric) models.Change {
	t.Helper()
	seedConfig(t, s, 1, gatewayConfig)
	seedConfig(t, s, 2, missingGatewayConfig)
	seedConfig(t, s, 3, missingGatewayConfig)
	c, e := s.PreviewChange(context.Background(), 1, &dto.ChangeCreateReq{RequestKey: "gateway-change-0001", SourceDeviceID: 1, TargetDeviceIDs: []int{2, 3}, Kind: "svi_gateway_add", VNI: 10008}, 7, operationsScope())
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func TestPreviewImmutableProvenanceRoundtripAndDeliveryBypassBlocked(t *testing.T) {
	s := operationsFixture(t)
	c := previewGateway(t, s)
	var loaded models.Change
	if e := s.DB.First(&loaded, c.ID).Error; e != nil {
		t.Fatal(e)
	}
	if loaded.Targets[0].ConfigHash == "" || loaded.Targets[0].SourceHash == "" || loaded.Targets[0].Device.ID != 2 {
		t.Fatal("lost private change provenance")
	}
	s.DB.Model(&delivery.Order{}).Where("id = ?", c.DeliveryOrderID).Update("status", "scheduled")
	workflow := ds.Workflow{Service: s.Service, Config: ds.Config{Enabled: true}}
	if _, e := workflow.Action(context.Background(), c.DeliveryOrderID, 1, "execute", 7, operationsScope()); e == nil {
		t.Fatal("generic execute bypass")
	}
	var order delivery.Order
	s.DB.First(&order, c.DeliveryOrderID)
	if order.Kind != "fabric" || order.Status != "scheduled" {
		t.Fatal("bad order")
	}
}
func TestUnknownWriteOutcomeStopsFollowingTargetsAndNeverReplays(t *testing.T) {
	s := operationsFixture(t)
	c := previewGateway(t, s)
	c, e := s.ExecuteChange(context.Background(), c.ID, &dto.VersionReq{Version: c.Version}, 7, operationsScope())
	if e != nil {
		t.Fatal(e)
	}
	fake := &fakeFabricTransport{bodies: map[int]string{1: gatewayConfig, 2: missingGatewayConfig, 3: missingGatewayConfig}, failWrite: true}
	w := NewWorker(s, fake)
	if e = w.runChange(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	var loaded models.Change
	s.DB.First(&loaded, c.ID)
	if loaded.Status != "uncertain" || loaded.Targets[0].Status != "uncertain" || loaded.Targets[1].Status != "cancelled" || len(fake.writes) != 1 {
		t.Fatalf("unknown outcome %+v writes %v", loaded, fake.writes)
	}
	var latest models.Observation
	s.DB.Where("device_id=2 AND dimension = ?", "configuration").Order("observed_at DESC,id DESC").First(&latest)
	if latest.Status != "failed" {
		t.Fatal("uncertain write left complete config current")
	}
	if len(loaded.Targets[0].Receipts) != 1 || loaded.Targets[0].Receipts[0].Status != "attempting" {
		t.Fatal("write intent not durable")
	}
	if e = w.Tick(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(fake.writes) != 1 {
		t.Fatal("replayed uncertain write")
	}
	var lease delivery.DeviceLease
	s.DB.First(&lease, 2)
	if !lease.Quarantined {
		t.Fatal("unknown transport not quarantined")
	}
}
func TestDriftAndAuthorizationPreventWrites(t *testing.T) {
	for _, kind := range []string{"drift", "grouping", "authorization"} {
		t.Run(kind, func(t *testing.T) {
			s := operationsFixture(t)
			c := previewGateway(t, s)
			c, e := s.ExecuteChange(context.Background(), c.ID, &dto.VersionReq{Version: c.Version}, 7, operationsScope())
			if e != nil {
				t.Fatal(e)
			}
			fake := &fakeFabricTransport{bodies: map[int]string{1: gatewayConfig, 2: missingGatewayConfig, 3: missingGatewayConfig}}
			switch kind {
			case "drift":
				fake.bodies[2] = gatewayConfig
			case "grouping":
				s.DB.Model(&models.Fabric{}).Where("id = 1").Update("rules_version", 2)
			case "authorization":
				s.DB.Exec("UPDATE sys_user SET status='1' WHERE user_id=7")
			}
			if e = NewWorker(s, fake).runChange(context.Background(), c); e != nil {
				t.Fatal(e)
			}
			if len(fake.writes) > 0 {
				t.Fatal("wrote after drift")
			}
			var loaded models.Change
			s.DB.First(&loaded, c.ID)
			if loaded.Status != "failed" {
				t.Fatalf("unexpected %s", loaded.Status)
			}
		})
	}
}
func TestRecoverInterruptedChangeDoesNotRequeueAndKeepsLease(t *testing.T) {
	s := operationsFixture(t)
	c := previewGateway(t, s)
	c.Targets[0].Status = "running"
	c.Targets[0].Attempted = true
	past := time.Now().UTC().Add(-time.Minute)
	if e := s.DB.Model(&models.Change{}).Where("id = ?", c.ID).Updates(map[string]any{"status": "running", "targets": jsonText(c.Targets), "claim_until": past}).Error; e != nil {
		t.Fatal(e)
	}
	s.DB.Create(&delivery.DeviceLease{DeviceID: 2, HolderID: c.Targets[0].DeliveryTargetID, Token: "dead", ExpiresAt: past})
	fake := &fakeFabricTransport{}
	w := NewWorker(s, fake)
	if e := w.Recover(context.Background()); e != nil {
		t.Fatal(e)
	}
	var loaded models.Change
	s.DB.First(&loaded, c.ID)
	if loaded.Status != "uncertain" || loaded.Targets[1].Status != "cancelled" {
		t.Fatalf("recovery %+v", loaded)
	}
	var lease delivery.DeviceLease
	s.DB.First(&lease, 2)
	if !lease.Quarantined {
		t.Fatal("expired lease not quarantined")
	}
	if len(fake.opens) > 0 {
		t.Fatal("recovery performed transport")
	}
}

type failingFabricTransport struct{}

func (failingFabricTransport) Open(context.Context, delivery.DeviceSnapshot, delivery.Connection, func(Session) error) error {
	return errors.New("transport unavailable")
}
func TestNewConnectionFailureSupersedesHealthyLatestWithoutDeletingHistory(t *testing.T) {
	s := operationsFixture(t)
	seedConfig(t, s, 1, gatewayConfig)
	job, e := s.CreateCollection(context.Background(), 1, &dto.CollectionCreateReq{RequestKey: "failed-collect-0001", Mode: "configuration"}, 7, operationsScope())
	if e != nil {
		t.Fatal(e)
	}
	if e = NewWorker(s, failingFabricTransport{}).runCollection(context.Background(), job); e != nil {
		t.Fatal(e)
	}
	var rows []models.Observation
	s.DB.Where("device_id = 1 AND dimension = ?", "configuration").Order("observed_at DESC,id DESC").Find(&rows)
	if len(rows) != 2 || rows[0].Status != "failed" || rows[1].Status != "complete" {
		t.Fatalf("failed attempt hid latest or cleared history %+v", rows)
	}
}
func TestMixedSupportedCollectionKeepsEligibleTargets(t *testing.T) {
	s := operationsFixture(t)
	s.DB.Model(&delivery.Connection{}).Where("device_id = 2").Update("profile", "h3c")
	s.DB.Where("device_id = 3").Delete(&delivery.Connection{})
	job, e := s.CreateCollection(context.Background(), 1, &dto.CollectionCreateReq{RequestKey: "mixed-collect-0001", Mode: "configuration"}, 7, operationsScope())
	if e != nil {
		t.Fatal(e)
	}
	fake := &fakeFabricTransport{bodies: map[int]string{1: gatewayConfig}}
	if e = NewWorker(s, fake).runCollection(context.Background(), job); e != nil {
		t.Fatal(e)
	}
	var loaded models.Collection
	s.DB.First(&loaded, job.ID)
	if len(fake.opens) != 1 || loaded.Targets[0].Status != "complete" || loaded.Targets[1].Status != "unsupported" || loaded.Targets[2].Status != "failed" || loaded.Targets[2].ErrorCode != "profile_unknown" {
		t.Fatalf("mixed collection lost targets: %+v", loaded.Targets)
	}
}
func TestSchedulerDisabledDefaultAndReconcilesRename(t *testing.T) {
	s := operationsFixture(t)
	w := NewWorker(s, &fakeFabricTransport{})
	now := time.Now().UTC()
	if e := w.schedule(context.Background(), now); e != nil {
		t.Fatal(e)
	}
	var count int64
	s.DB.Model(&models.Collection{}).Count(&count)
	if count != 0 {
		t.Fatal("scheduled disabled collection")
	}
	s.DB.Model(&models.Fabric{}).Where("id = 1").Update("collection_enabled", true)
	s.DB.Model(&network.Device{}).Where("id = 3").Update("name", "OTHER-LEF-3")
	if e := w.schedule(context.Background(), now); e != nil {
		t.Fatal(e)
	}
	var jobs []models.Collection
	s.DB.Order("id").Find(&jobs)
	if len(jobs) != 2 {
		t.Fatalf("expected runtime/config jobs %d", len(jobs))
	}
	for _, job := range jobs {
		if len(job.Targets) != 2 {
			t.Fatalf("scheduled stale membership %+v", job)
		}
	}
	if e := w.schedule(context.Background(), now.Add(time.Minute)); e != nil {
		t.Fatal(e)
	}
	s.DB.Model(&models.Collection{}).Count(&count)
	if count != 2 {
		t.Fatal("duplicate scheduled jobs")
	}
}
func TestPostRuntimeFaultAndUnknownAreDifferentFromHealthy(t *testing.T) {
	config := vxlan.Record{"bgp": vxlan.Record{"asn": 65000, "neighbors": vxlan.Record{"192.0.2.2": vxlan.Record{"address_families": vxlan.Record{"l2vpn evpn": vxlan.Record{}}}}}}
	snapshot := vxlan.Snapshot{Dimensions: map[string]vxlan.Observation{"bgp_neighbors": {Name: "bgp_neighbors", Status: "complete", Records: []vxlan.Record{{"state": "idle"}}}, "interfaces": {Status: "complete"}, "port_channels": {Status: "complete"}}}
	if postRuntimeState(snapshot, config) != "fault" {
		t.Fatal("runtime protocol fault called healthy")
	}
	snapshot.Dimensions["bgp_neighbors"] = vxlan.Observation{Status: "failed"}
	if postRuntimeState(snapshot, config) != "unknown" {
		t.Fatal("missing runtime called healthy")
	}
	snapshot.Dimensions["bgp_neighbors"] = vxlan.Observation{Status: "complete", Records: []vxlan.Record{{"state": "established", "peer_ip": "192.0.2.2", "vrf": "default"}}}
	if postRuntimeState(snapshot, config) != "healthy" {
		t.Fatal("healthy runtime not accepted")
	}
}
func TestPublicReadbackDoesNotLeakConfigurationButStorageRetainsIt(t *testing.T) {
	s := operationsFixture(t)
	c := previewGateway(t, s)
	secret := "private-secret-example"
	snapshot := vxlan.Snapshot{Dimensions: map[string]vxlan.Observation{"configuration": {Name: "configuration", Status: "complete", Records: []vxlan.Record{{"secret": secret}}, Evidence: []vxlan.Record{{"sha256": "abc", "command": "show running-config", "raw": secret}}}}}
	c.Targets[0].PreEvidence = snapshot
	if e := s.DB.Model(&models.Change{}).Where("id = ?", c.ID).Update("targets", jsonText(c.Targets)).Error; e != nil {
		t.Fatal(e)
	}
	var loaded models.Change
	if e := s.DB.First(&loaded, c.ID).Error; e != nil {
		t.Fatal(e)
	}
	if loaded.Targets[0].PreEvidence.Dimensions["configuration"].Records[0]["secret"] != secret {
		t.Fatal("private readback lost")
	}
	raw, _ := json.Marshal(loaded)
	if strings.Contains(string(raw), secret) || !strings.Contains(string(raw), "preEvidence") || !strings.Contains(string(raw), "sha256") {
		t.Fatalf("unsafe or absent public evidence %s", raw)
	}
}

func TestSuccessfulChangeReadsBackAndContinuesSerially(t *testing.T) {
	s := operationsFixture(t)
	c := previewGateway(t, s)
	c, e := s.ExecuteChange(context.Background(), c.ID, &dto.VersionReq{Version: c.Version}, 7, operationsScope())
	if e != nil {
		t.Fatal(e)
	}
	fake := &fakeFabricTransport{bodies: map[int]string{1: gatewayConfig, 2: missingGatewayConfig, 3: missingGatewayConfig}, after: gatewayConfig, commandBodies: map[string]string{"show port-channel summary": "", "show interface brief": "Interface Status Description\nVlan8 up business\n"}}
	if e = NewWorker(s, fake).runChange(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	var loaded models.Change
	s.DB.First(&loaded, c.ID)
	if loaded.Status != "succeeded" {
		t.Fatalf("unexpected status %s targets %+v", loaded.Status, loaded.Targets)
	}
	if len(fake.writes) != 8 {
		t.Fatalf("writes %v", fake.writes)
	}
	for i, id := range fake.writes {
		expected := 2
		if i >= 4 {
			expected = 3
		}
		if id != expected {
			t.Fatal("targets interleaved")
		}
	}
	for _, target := range loaded.Targets {
		if !target.ConfigurationVerified || target.RuntimeState != "healthy" || target.CompletedSteps != 4 || len(target.PostEvidence.Dimensions) == 0 {
			t.Fatalf("unverified target %+v", target)
		}
	}
}
func TestRuntimeFaultAfterSatisfiedStopsFollowingTarget(t *testing.T) {
	s := operationsFixture(t)
	c := previewGateway(t, s)
	c, e := s.ExecuteChange(context.Background(), c.ID, &dto.VersionReq{Version: c.Version}, 7, operationsScope())
	if e != nil {
		t.Fatal(e)
	}
	fake := &fakeFabricTransport{bodies: map[int]string{1: gatewayConfig, 2: missingGatewayConfig, 3: missingGatewayConfig}, after: gatewayConfig, commandBodies: map[string]string{"show port-channel summary": "", "show interface brief": "Interface Status Description\nVlan8 down business\n"}}
	if e = NewWorker(s, fake).runChange(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	var loaded models.Change
	s.DB.First(&loaded, c.ID)
	if loaded.Status != "failed" || loaded.Targets[0].RuntimeState != "fault" || !loaded.Targets[0].ConfigurationVerified || loaded.Targets[1].Status != "cancelled" || len(fake.writes) != 4 {
		t.Fatalf("runtime gate failed status %s targets %+v writes %v", loaded.Status, loaded.Targets, fake.writes)
	}
}
func TestRecheckPermissionBeforeEachWrite(t *testing.T) {
	s := operationsFixture(t)
	c := previewGateway(t, s)
	c, e := s.ExecuteChange(context.Background(), c.ID, &dto.VersionReq{Version: c.Version}, 7, operationsScope())
	if e != nil {
		t.Fatal(e)
	}
	fake := &revokingTransport{base: &fakeFabricTransport{bodies: map[int]string{1: gatewayConfig, 2: missingGatewayConfig, 3: missingGatewayConfig}}, s: s}
	if e = NewWorker(s, fake).runChange(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	if len(fake.base.writes) != 1 {
		t.Fatalf("continued after revocation %v", fake.base.writes)
	}
	var loaded models.Change
	s.DB.First(&loaded, c.ID)
	if loaded.Status != "uncertain" {
		t.Fatalf("expected uncertain %s", loaded.Status)
	}
}

type revokingTransport struct {
	base *fakeFabricTransport
	s    *Fabric
}

func (f *revokingTransport) Open(ctx context.Context, d delivery.DeviceSnapshot, c delivery.Connection, fn func(Session) error) error {
	return f.base.Open(ctx, d, c, func(s Session) error { return fn(&revokingSession{Session: s, owner: f}) })
}

type revokingSession struct {
	Session
	owner *revokingTransport
}

func (s *revokingSession) Write(ctx context.Context, command string) error {
	if e := s.Session.Write(ctx, command); e != nil {
		return e
	}
	return s.owner.s.DB.Exec("UPDATE sys_user SET status='1' WHERE user_id=7").Error
}

func TestReassignedDeviceLeaseStopsNextWrite(t *testing.T) {
	s := operationsFixture(t)
	c := previewGateway(t, s)
	c, e := s.ExecuteChange(context.Background(), c.ID, &dto.VersionReq{Version: c.Version}, 7, operationsScope())
	if e != nil {
		t.Fatal(e)
	}
	fake := &leaseRevokingTransport{base: &fakeFabricTransport{bodies: map[int]string{1: gatewayConfig, 2: missingGatewayConfig, 3: missingGatewayConfig}}, s: s}
	if e = NewWorker(s, fake).runChange(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	if len(fake.base.writes) != 1 {
		t.Fatalf("continued after lease reassignment %v", fake.base.writes)
	}
	var lease delivery.DeviceLease
	s.DB.First(&lease, 2)
	if lease.Token != "replacement" {
		t.Fatal("deleted replacement worker's lease")
	}
}

type leaseRevokingTransport struct {
	base *fakeFabricTransport
	s    *Fabric
}

func (f *leaseRevokingTransport) Open(ctx context.Context, d delivery.DeviceSnapshot, c delivery.Connection, fn func(Session) error) error {
	return f.base.Open(ctx, d, c, func(s Session) error { return fn(&leaseRevokingSession{Session: s, owner: f, id: d.ID}) })
}

type leaseRevokingSession struct {
	Session
	owner *leaseRevokingTransport
	id    int
}

func (s *leaseRevokingSession) Write(ctx context.Context, command string) error {
	if e := s.Session.Write(ctx, command); e != nil {
		return e
	}
	return s.owner.s.DB.Model(&delivery.DeviceLease{}).Where("device_id = ?", s.id).Update("token", "replacement").Error
}

func TestCancelPreviewPersistsAllTargetOutcomes(t *testing.T) {
	s := operationsFixture(t)
	c := previewGateway(t, s)
	cancelled, e := s.CancelChange(context.Background(), c.ID, &dto.VersionReq{Version: c.Version}, 7, operationsScope())
	if e != nil {
		t.Fatal(e)
	}
	if cancelled.Status != "cancelled" {
		t.Fatal("cancel not persisted")
	}
	for _, target := range cancelled.Targets {
		if target.Status != "cancelled" {
			t.Fatal("target not cancelled")
		}
	}
	if _, e = s.ExecuteChange(context.Background(), c.ID, &dto.VersionReq{Version: cancelled.Version}, 7, operationsScope()); e == nil {
		t.Fatal("cancelled plan executable")
	}
}
func TestControlledACLRequiresExistingMatchingPolicyAndCannotCreateACL(t *testing.T) {
	entries := []vxlan.Record{{"sequence": 10, "action": "permit", "expression": "ip any any"}}
	source := vxlan.Record{"vlan_vnis": vxlan.Record{"8": 10008}, "interfaces": vxlan.Record{"Vlan8": vxlan.Record{"acl_bindings": vxlan.Record{"in": "BUSINESS"}}}, "acls": vxlan.Record{"BUSINESS": entries}, "address_groups": vxlan.Record{}}
	plan, e := controlledPlan(source, &dto.ChangeCreateReq{Kind: "svi_acl", VNI: 10008, Direction: "in"})
	if e != nil {
		t.Fatal(e)
	}
	if plan.Change()["create_acl"] != false {
		t.Fatal("ACL creation allowed")
	}
	target := vxlan.Record{"vlan_vnis": vxlan.Record{"8": 10008}, "interfaces": vxlan.Record{"Vlan8": vxlan.Record{"acl_bindings": vxlan.Record{"in": nil}}}, "acls": vxlan.Record{"BUSINESS": entries}, "address_groups": vxlan.Record{}}
	if !plan.Eligible(target) {
		t.Fatal("existing matching ACL not eligible")
	}
	for _, command := range plan.Commands() {
		if strings.Contains(command, "ip access-list") || strings.Contains(command, "no ") {
			t.Fatal("ACL create/delete in plan")
		}
	}
	delete(target["acls"].(map[string]any), "BUSINESS")
	if plan.Eligible(target) {
		t.Fatal("target missing ACL accepted")
	}
	if _, e := controlledPlan(source, &dto.ChangeCreateReq{Kind: "arbitrary_script", VNI: 10008}); e == nil {
		t.Fatal("arbitrary kind accepted")
	}
}
