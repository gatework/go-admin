package service

import (
	"context"
	vxlan "github.com/gatework/go-vxlan"
	sdkconfig "github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	delivery "go-admin/app/delivery/models"
	"go-admin/app/fabric/models"
	"go-admin/app/fabric/service/dto"
	network "go-admin/app/network/models"
	"strings"
	"testing"
	"time"
)

func TestReviewMissingLeafMustAffectSummary(t *testing.T) {
	s := coreService(t)
	ctx := context.Background()
	p := operationsScope()
	for i, body := range []string{"hostname one\nvlan 10\n vn-segment 10010\ninterface nve1\n member vni 10010\n  ingress-replication protocol bgp\nevpn\n vni 10010 l2\n", "hostname two\n"} {
		d := coreDevice(t, s, []string{"SZX9-LEF-1", "SZX9-LEF-2"}[i], []string{"10.0.0.1", "10.0.0.2"}[i], 7)
		if _, _, err := s.VisibleMembers(ctx, 1, p); err != nil {
			t.Fatal(err)
		}
		var m models.Membership
		s.Orm.First(&m, "device_id = ?", d.Id)
		c := delivery.Connection{DeviceID: d.Id, Profile: "cisco_nxos", Protocol: "ssh", Port: 22, Version: 1}
		s.Orm.Create(&c)
		cfg := vxlan.ParseConfig(body, true)
		if !cfg.Complete() {
			t.Fatalf("fixture %+v", cfg)
		}
		for _, name := range vxlan.DimensionNames() {
			data := vxlan.Observation{Name: name, Status: "complete"}
			if name == "configuration" {
				data = cfg
			}
			if name == "nve_peers" {
				data.Records = []vxlan.Record{{"state": "up"}}
			}
			if name == "nve_vnis" {
				data.Records = []vxlan.Record{{"state": "up", "vni": 10010}}
			}
			row := models.Observation{FabricID: 1, DeviceID: d.Id, Dimension: name, Status: "complete", Data: data, ObservedAt: time.Now().UTC(), SourceHash: sourceHash(opsDevice(d), c), MembershipVersion: m.Version, RuleVersion: m.RuleVersion}
			if err := s.Orm.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	out, err := s.Detail(ctx, 1, p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("summary=%s findings=%+v segments=%+v", out.State, out.Findings, out.Segments)
	if len(out.Segments) != 1 || out.Segments[0].State != "different" {
		t.Fatal("fixture didn't establish missing leaf")
	}
	if out.State == "healthy" {
		t.Fatal("healthy summary despite missing service on a Leaf")
	}
}

func TestReviewEndpointConfigurationOnlyRefreshRetainsVRF(t *testing.T) {
	s := operationsFixture(t)
	ctx := context.Background()
	seedConfig(t, s, 1, gatewayConfig)
	var cfg models.Observation
	s.Orm.Where("device_id=1 AND dimension='configuration'").First(&cfg)
	ep := models.Observation{FabricID: 1, DeviceID: 1, Dimension: "l2route_mac_ips", Status: "complete", ObservedAt: time.Now().UTC(), SourceHash: cfg.SourceHash, MembershipVersion: 1, RuleVersion: 1, Data: vxlan.Observation{Name: "l2route_mac_ips", Status: "complete", Records: []vxlan.Record{{"topology_id": 8, "host_ip": "192.0.2.9", "mac": "aa:bb:cc:dd:ee:ff", "next_hops": []vxlan.Record{{"interface": "Eth1/1"}}}}}}
	s.Orm.Create(&ep)
	req := &dto.PageReq{Keyword: "192.0.2.9", VRF: "tenant"}
	req.PageIndex = 1
	req.PageSize = 10
	before, _, err := s.Endpoints(ctx, req, operationsScope())
	if err != nil || len(before) != 1 {
		t.Fatalf("before %v %v", before, err)
	}
	seedConfig(t, s, 1, gatewayConfig)
	after, _, err := s.Endpoints(ctx, req, operationsScope())
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 {
		t.Fatalf("configuration-only refresh lost endpoint from VRF query: before=%+v after=%+v", before, after)
	}
}

func TestReviewACLPreviewRejectsCrossVRF(t *testing.T) {
	s := operationsFixture(t)
	ctx := context.Background()
	acl := "ip access-list PROTECT\n 10 permit ip any any\n"
	source := strings.Replace(gatewayConfig, "  fabric forwarding mode anycast-gateway\n", "  fabric forwarding mode anycast-gateway\n  ip access-group PROTECT in\n", 1) + acl
	target := strings.ReplaceAll(gatewayConfig, "tenant", "other") + acl
	seedConfig(t, s, 1, source)
	seedConfig(t, s, 2, target)
	r := &dto.ChangeCreateReq{RequestKey: "review-cross-vrf-0001", SourceDeviceID: 1, TargetDeviceIDs: []int{2}, Kind: "svi_acl", VNI: 10008, Direction: "in"}
	change, err := s.PreviewChange(ctx, 1, r, 7, operationsScope())
	if err == nil {
		t.Fatalf("cross-VRF ACL accepted: %+v", change.Plan)
	}
}

func TestReviewAutomaticFabricExcludesAccessPoint(t *testing.T) {
	s := coreService(t)
	ctx := context.Background()
	d := coreDevice(t, s, "SZX9-AP1", "10.0.0.1", 7)
	if e := s.Orm.Model(&d).Update("device_role", "wireless_ap").Error; e != nil {
		t.Fatal(e)
	}
	rows, _, err := s.VisibleMembers(ctx, 1, operationsScope())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("regional AP joined Fabric automatically: %+v", rows[0].DeviceRole)
	}
}
func TestReviewBusyCollectionInvalidatesCurrentEvidence(t *testing.T) {
	s := operationsFixture(t)
	ctx := context.Background()
	seedConfig(t, s, 1, gatewayConfig)
	job, err := s.CreateCollection(ctx, 1, &dto.CollectionCreateReq{RequestKey: "review-busy-collect-0001", Mode: "configuration"}, 7, operationsScope())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Orm.Create(&delivery.DeviceLease{DeviceID: 1, Token: "other-write", ExpiresAt: time.Now().Add(time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	fake := &fakeFabricTransport{bodies: map[int]string{2: gatewayConfig, 3: gatewayConfig}}
	if err = NewWorker(s, fake).runCollection(ctx, job); err != nil {
		t.Fatal(err)
	}
	var loaded models.Collection
	s.Orm.First(&loaded, job.ID)
	rows, err := s.evidenceRows(ctx, 1, []int{1})
	if err != nil {
		t.Fatal(err)
	}
	e := DimensionEvidence(rows, "configuration", time.Now(), time.Hour)
	if e.State == "complete" {
		t.Fatalf("failed attempt %s/%s leaves current configuration complete", loaded.Targets[0].Status, loaded.Targets[0].ErrorCode)
	}
}
func TestReviewIPQueryCorrelatesKnownMACPort(t *testing.T) {
	s := operationsFixture(t)
	ctx := context.Background()
	seedConfig(t, s, 1, gatewayConfig)
	var cfg models.Observation
	s.Orm.Where("device_id=1 AND dimension='configuration'").First(&cfg)
	for _, name := range []string{"l2route_mac_ips", "mac_addresses"} {
		row := vxlan.Record{"topology_id": 8, "host_ip": "192.0.2.9", "mac": "aa:bb:cc:dd:ee:ff", "next_hops": []vxlan.Record{{"local": true}}}
		if name == "mac_addresses" {
			row = vxlan.Record{"vlan_id": 8, "mac": "aa:bb:cc:dd:ee:ff", "destination": "Eth1/10"}
		}
		ep := models.Observation{FabricID: 1, DeviceID: 1, Dimension: name, Status: "complete", ObservedAt: time.Now().UTC(), SourceHash: cfg.SourceHash, MembershipVersion: 1, RuleVersion: 1, Data: vxlan.Observation{Name: name, Status: "complete", Records: []vxlan.Record{row}}}
		s.Orm.Create(&ep)
	}
	req := &dto.PageReq{Keyword: "192.0.2.9", VRF: "tenant"}
	req.PageIndex = 1
	req.PageSize = 10
	rows, _, err := s.Endpoints(ctx, req, operationsScope())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Interface == "Eth1/10" {
			return
		}
	}
	t.Fatalf("IP lookup lost known local MAC port: %+v", rows)
}
func TestReviewSingleVPCLabelDoesNotProveDualHome(t *testing.T) {
	parsed, err := vxlan.ParseRuntime("l2route_mac_ips", "Topology Mac Address Host IP Prod Next-Hops\n8 aabb.ccdd.eeff 192.0.2.9 HMM vPC10\nTotal entries: 1\n", true)
	if err != nil || !parsed.Complete() {
		t.Fatalf("fixture %v %+v", err, parsed)
	}
	rows := endpointRows(models.Observation{Data: parsed}, "leaf", nil, "partial")
	if len(rows) != 1 {
		t.Fatal(rows)
	}
	if rows[0].Kind == "vpc_dual_homed" {
		t.Fatalf("single uncorrelated partial endpoint claimed dual home: %+v", rows[0])
	}
}

func TestReviewHistoricalChangeStillRequiresTargetScope(t *testing.T) {
	s := operationsFixture(t)
	ctx := context.Background()
	change := previewGateway(t, s)
	old := sdkconfig.ApplicationConfig.EnableDP
	sdkconfig.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { sdkconfig.ApplicationConfig.EnableDP = old })
	fid := 2
	s.Orm.Create(&models.Fabric{ID: 2, Code: "OTHER", Name: "Other", Version: 1, RulesVersion: 1})
	if e := s.Orm.Model(&models.Membership{}).Where("device_id=2").Updates(map[string]any{"fabric_id": fid, "mode": "manual"}).Error; e != nil {
		t.Fatal(e)
	}
	if e := s.Orm.Model(&network.Device{}).Where("id=2").Update("create_by", 8).Error; e != nil {
		t.Fatal(e)
	}
	p := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 7, RoleId: 1}
	got, err := s.GetChange(ctx, change.ID, p)
	if err == nil {
		t.Fatalf("historical hidden target exposed via full scope of current Fabric: source=%d targets=%+v", got.SourceDeviceID, got.Targets)
	}
}

func TestReviewACLValidationRejectsForgedCrossVRFPlan(t *testing.T) {
	s := operationsFixture(t)
	ctx := context.Background()
	acl := "ip access-list PROTECT\n 10 permit ip any any\n"
	source := strings.Replace(gatewayConfig, "  fabric forwarding mode anycast-gateway\n", "  fabric forwarding mode anycast-gateway\n  ip access-group PROTECT in\n", 1) + acl
	seedConfig(t, s, 1, source)
	seedConfig(t, s, 2, gatewayConfig+acl)
	r := &dto.ChangeCreateReq{RequestKey: "forged-cross-vrf-0001", SourceDeviceID: 1, TargetDeviceIDs: []int{2}, Kind: "svi_acl", VNI: 10008, Direction: "in"}
	c, err := s.PreviewChange(ctx, 1, r, 7, operationsScope())
	if err != nil {
		t.Fatal(err)
	}
	seedConfig(t, s, 2, strings.ReplaceAll(gatewayConfig, "tenant", "other")+acl)
	_, target, err := s.changeConfig(ctx, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	c.Targets[0].ConfigHash = opsHash(target)
	// Even internally consistent persisted hashes cannot authorize another tenant.
	if err := s.validateChange(ctx, c); err == nil {
		t.Fatal("forged plan accepted another VRF")
	}
}

func TestReviewRoleChangesReconcileAndManualUnknownRemainsAssigned(t *testing.T) {
	s := coreService(t)
	ctx := context.Background()
	d := coreDevice(t, s, "SZX9-LEF-1", "10.0.0.1", 7)
	if _, _, err := s.VisibleMembers(ctx, 1, operationsScope()); err != nil {
		t.Fatal(err)
	}
	s.Orm.Model(&d).Update("device_role", "access_switch")
	rows, _, err := s.VisibleMembers(ctx, 1, operationsScope())
	if err != nil || len(rows) != 0 {
		t.Fatalf("role drift retained automatic member: %v %+v", err, rows)
	}
	id := 1
	if _, err := s.UpdateMember(ctx, d.Id, &dto.MemberReq{Mode: "manual", FabricID: &id}, 7, operationsScope()); err != nil {
		t.Fatal(err)
	}
	s.Orm.Model(&d).Update("device_role", "unknown")
	rows, _, err = s.VisibleMembers(ctx, 1, operationsScope())
	if err != nil || len(rows) != 1 {
		t.Fatalf("manual unknown excluded: %v %+v", err, rows)
	}
}

func TestReviewHistoricalOperationListsAndCollectionRespectTargetScope(t *testing.T) {
	s := operationsFixture(t)
	ctx := context.Background()
	change := previewGateway(t, s)
	job, err := s.CreateCollection(ctx, 1, &dto.CollectionCreateReq{RequestKey: "historical-scope-collection", Mode: "configuration"}, 7, operationsScope())
	if err != nil {
		t.Fatal(err)
	}
	old := sdkconfig.ApplicationConfig.EnableDP
	sdkconfig.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { sdkconfig.ApplicationConfig.EnableDP = old })
	id := 2
	s.Orm.Create(&models.Fabric{ID: id, Code: "OTHER", Version: 1, RulesVersion: 1})
	s.Orm.Model(&models.Membership{}).Where("device_id=2").Updates(map[string]any{"fabric_id": id, "mode": "manual"})
	s.Orm.Model(&network.Device{}).Where("id=2").Update("create_by", 8)
	p := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 7, RoleId: 1}
	if _, err := s.GetCollection(ctx, job.ID, p); err == nil {
		t.Fatal("collection exposes hidden historical target")
	}
	if _, err := s.CreateCollection(ctx, 1, &dto.CollectionCreateReq{RequestKey: "historical-scope-collection", Mode: "configuration"}, 7, p); err == nil {
		t.Fatal("idempotent replay exposes hidden historical collection target")
	}
	req := &dto.PageReq{}
	req.PageIndex = 1
	req.PageSize = 20
	changes, count, err := s.Changes(ctx, 1, req, p)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || len(changes) != 0 {
		t.Fatalf("historical change %d exposed in list: %+v count=%d", change.ID, changes, count)
	}
	collections, count, err := s.Collections(ctx, 1, req, p)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || len(collections) != 0 {
		t.Fatalf("historical collection exposed in list: %+v count=%d", collections, count)
	}
}

func TestReviewVPCPartialProofCannotConfirmDualHome(t *testing.T) {
	rows := []dto.Endpoint{{FabricID: 1, DeviceID: 1, VRF: "red", VLAN: 10, VNI: 10010, MAC: "aa:bb:cc:dd:ee:ff", State: "current", Kind: "local_port", VPCID: "10", EvidenceID: 1}, {FabricID: 1, DeviceID: 2, VRF: "red", VLAN: 10, VNI: 10010, MAC: "aa:bb:cc:dd:ee:ff", State: "current", Kind: "local_port", VPCID: "10", EvidenceID: 2}}
	proof := map[int]dto.Node{1: {DeviceID: 1, IP: "10.0.0.1", VPCDomain: "1", VPCPeer: "10.0.0.2", State: "healthy"}, 2: {DeviceID: 2, IP: "10.0.0.2", VPCDomain: "1", VPCPeer: "10.0.0.1", State: "unknown"}}
	classifyEndpointCandidates(rows, proof)
	for _, r := range rows {
		if r.Kind == "vpc_dual_homed" {
			t.Fatal("partial peer evidence proved dual homing")
		}
	}
}
func TestReviewVPCConflictingLocalAttachmentsCannotConfirmDualHome(t *testing.T) {
	rows := []dto.Endpoint{{FabricID: 1, DeviceID: 1, VRF: "red", VLAN: 10, VNI: 10010, MAC: "aa:bb:cc:dd:ee:ff", State: "current", Kind: "local_port", VPCID: "10", EvidenceID: 1}, {FabricID: 1, DeviceID: 2, VRF: "red", VLAN: 10, VNI: 10010, MAC: "aa:bb:cc:dd:ee:ff", State: "current", Kind: "local_port", VPCID: "10", EvidenceID: 2}}
	conflict := rows[0]
	conflict.VPCID = "20"
	rows = append(rows, conflict)
	proof := map[int]dto.Node{1: {DeviceID: 1, IP: "10.0.0.1", VPCDomain: "1", VPCPeer: "10.0.0.2", State: "healthy"}, 2: {DeviceID: 2, IP: "10.0.0.2", VPCDomain: "1", VPCPeer: "10.0.0.1", State: "healthy"}}
	classifyEndpointCandidates(rows, proof)
	for _, r := range rows {
		if r.Kind == "vpc_dual_homed" {
			t.Fatal("conflicting attachment proved dual homing")
		}
	}
}

func TestReviewIPPortCorrelationRejectsIncompatibleEvidence(t *testing.T) {
	for _, scenario := range []string{"other_vrf", "other_vlan", "other_vni", "stale", "failed", "other_source", "other_membership", "remote"} {
		t.Run(scenario, func(t *testing.T) {
			s := operationsFixture(t)
			ctx := context.Background()
			seedConfig(t, s, 1, gatewayConfig)
			var cfg models.Observation
			if err := s.Orm.Where("device_id=1 AND dimension='configuration'").First(&cfg).Error; err != nil {
				t.Fatal(err)
			}
			now := time.Now().UTC()
			mac := vxlan.Record{"vlan_id": 8, "mac": "aa:bb:cc:dd:ee:ff", "destination": "Eth1/10"}
			port := models.Observation{FabricID: 1, DeviceID: 1, Dimension: "mac_addresses", Status: "complete", ObservedAt: now, SourceHash: cfg.SourceHash, MembershipVersion: 1, RuleVersion: 1}
			switch scenario {
			case "other_vrf":
				mac["vrf"] = "other"
			case "other_vlan":
				mac["vlan_id"] = 9
			case "other_vni":
				mac["vni"] = 20008
			case "stale":
				port.ObservedAt = now.Add(-time.Hour)
			case "failed":
				port.Status = "partial"
			case "other_source":
				port.SourceHash = "old-source"
			case "other_membership":
				port.MembershipVersion = 9
			case "remote":
				mac["destination"] = "nve1(192.0.2.2)"
			}
			port.Data = vxlan.Observation{Name: port.Dimension, Status: port.Status, Records: []vxlan.Record{mac}}
			ep := models.Observation{FabricID: 1, DeviceID: 1, Dimension: "l2route_mac_ips", Status: "complete", ObservedAt: now, SourceHash: cfg.SourceHash, MembershipVersion: 1, RuleVersion: 1, Data: vxlan.Observation{Name: "l2route_mac_ips", Status: "complete", Records: []vxlan.Record{{"topology_id": 8, "host_ip": "192.0.2.9", "mac": "aa:bb:cc:dd:ee:ff", "next_hops": []vxlan.Record{{"local": true}}}}}}
			if err := s.Orm.Create(&port).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.Orm.Create(&ep).Error; err != nil {
				t.Fatal(err)
			}
			req := &dto.PageReq{Keyword: "192.0.2.9", VRF: "tenant"}
			req.PageIndex = 1
			req.PageSize = 10
			rows, _, err := s.Endpoints(ctx, req, operationsScope())
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].Kind != "local_unknown_port" || rows[0].Interface != "" {
				t.Fatalf("incompatible evidence guessed port: %+v", rows)
			}
		})
	}
}

func TestReviewSameIPDifferentMACPositionsRemainAmbiguous(t *testing.T) {
	rows := []dto.Endpoint{{FabricID: 1, DeviceID: 1, VRF: "red", VLAN: 10, VNI: 10010, IP: "192.0.2.9", MAC: "aa:bb:cc:dd:ee:01", Interface: "Eth1/1", State: "current", Kind: "local_port"}, {FabricID: 1, DeviceID: 1, VRF: "red", VLAN: 10, VNI: 10010, IP: "192.0.2.9", MAC: "aa:bb:cc:dd:ee:02", Interface: "Eth1/2", State: "current", Kind: "local_port"}}
	classifyEndpointCandidates(rows, nil)
	if rows[0].CandidateCount != 2 || rows[1].CandidateCount != 2 {
		t.Fatalf("same IP candidate ambiguity hidden: %+v", rows)
	}
}

func TestReviewEndpointDualHomeRequiresCurrentHealthyRuntimePair(t *testing.T) {
	for _, status := range []string{"complete", "partial", "failed"} {
		t.Run(status, func(t *testing.T) {
			s := operationsFixture(t)
			ctx := context.Background()
			for _, device := range []int{1, 2} {
				ip, peer := "192.0.2.1", "192.0.2.2"
				if device == 2 {
					ip, peer = peer, ip
				}
				seedConfig(t, s, device, gatewayConfig+"vpc domain 1\n peer-keepalive destination "+peer+" source "+ip+" vrf management\ninterface port-channel10\n vpc 10\n")
				var cfg models.Observation
				s.Orm.Where("device_id=? AND dimension='configuration'", device).First(&cfg)
				parsed, err := vxlan.ParseRuntime("vpc", "vPC domain id : 1\nPeer status : peer adjacency formed ok\nvPC keep-alive status : peer is alive\nConfiguration consistency status : success\n", true)
				if err != nil || !parsed.Complete() {
					t.Fatalf("fixture %v %+v", err, parsed)
				}
				runtimeStatus := "complete"
				if device == 2 {
					runtimeStatus = status
				}
				parsed.Status = runtimeStatus
				runtime := models.Observation{FabricID: 1, DeviceID: device, Dimension: "vpc", Status: runtimeStatus, ObservedAt: time.Now().UTC(), SourceHash: cfg.SourceHash, MembershipVersion: 1, RuleVersion: 1, Data: parsed}
				if err := s.Orm.Create(&runtime).Error; err != nil {
					t.Fatal(err)
				}
				ep := models.Observation{FabricID: 1, DeviceID: device, Dimension: "l2route_mac_ips", Status: "complete", ObservedAt: time.Now().UTC(), SourceHash: cfg.SourceHash, MembershipVersion: 1, RuleVersion: 1, Data: vxlan.Observation{Name: "l2route_mac_ips", Status: "complete", Records: []vxlan.Record{{"topology_id": 8, "host_ip": "192.0.2.9", "mac": "aa:bb:cc:dd:ee:ff", "next_hops": []vxlan.Record{{"interface": "Po10"}}}}}}
				if err := s.Orm.Create(&ep).Error; err != nil {
					t.Fatal(err)
				}
			}
			req := &dto.PageReq{Keyword: "192.0.2.9", VRF: "tenant"}
			req.PageIndex = 1
			req.PageSize = 10
			rows, _, err := s.Endpoints(ctx, req, operationsScope())
			if err != nil || len(rows) != 2 {
				t.Fatalf("rows %+v %v", rows, err)
			}
			for _, row := range rows {
				if (row.Kind == "vpc_dual_homed") != (status == "complete") {
					t.Fatalf("runtime %s wrong classification: %+v", status, row)
				}
			}
		})
	}
}
