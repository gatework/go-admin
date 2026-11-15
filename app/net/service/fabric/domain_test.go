package fabric

import (
	"testing"
	"time"

	vxlan "github.com/gatework/go-vxlan"
	models "github.com/gatework/motor/app/net/models/fabric"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
)

func TestRulesExposeConflictAndRespectManual(t *testing.T) {
	rules := []models.Rule{{FabricID: 1, Pattern: "SZX9", Enabled: true}, {FabricID: 2, Pattern: "LEF", Enabled: true}}
	m := ResolveMembership("SZX9-LEF-01", models.Membership{Mode: "auto"}, rules, 3)
	if m.Status != "conflict" || m.FabricID != nil {
		t.Fatalf("conflict became assigned: %+v", m)
	}
	id := 2
	m = ResolveMembership("SZX9-LEF-01", models.Membership{Mode: "manual", FabricID: &id}, rules, 3)
	if m.FabricID == nil || *m.FabricID != 2 || m.Status != "assigned" {
		t.Fatalf("manual overwritten: %+v", m)
	}
}
func TestLatestFailureRetainsLastCompleteAndCannotBeHealthy(t *testing.T) {
	now := time.Now().UTC()
	rows := []models.Observation{{ID: 1, Dimension: "nve_peers", Status: "complete", ObservedAt: now.Add(-time.Minute), Data: vxlan.Observation{Status: "complete"}}, {ID: 2, Dimension: "nve_peers", Status: "failed", ObservedAt: now, Data: vxlan.Observation{Status: "failed"}}}
	view := DimensionEvidence(rows, "nve_peers", now, 10*time.Minute)
	if view.Latest == nil || view.Latest.ID != 2 || view.LastComplete == nil || view.LastComplete.ID != 1 || view.State != "unknown" {
		t.Fatalf("lost partial history: %+v", view)
	}
}
func TestEndpointQueryNormalizesAndVRFIsolates(t *testing.T) {
	q, err := ParseEndpointQuery("AABB.CCDD.EEFF")
	if err != nil || q.Value != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("MAC: %+v %v", q, err)
	}
	q, err = ParseEndpointQuery("10.0.0.9/24")
	if err != nil || q.Value != "10.0.0.0/24" {
		t.Fatalf("CIDR: %+v %v", q, err)
	}
	if !q.Matches(dto.Endpoint{IP: "10.0.0.5", VRF: "red"}, "red") || q.Matches(dto.Endpoint{IP: "10.0.0.5", VRF: "blue"}, "red") {
		t.Fatal("VRF isolation failed")
	}
}
func TestVPCRequiresReciprocalPeerEvidence(t *testing.T) {
	rows := []dto.Node{{DeviceID: 1, IP: "10.0.0.1", VPCDomain: "1", VPCPeer: "10.0.0.2"}, {DeviceID: 2, IP: "10.0.0.2", VPCDomain: "1", VPCPeer: "10.0.0.9"}}
	if groups := VPCGroups(rows); len(groups) != 0 {
		t.Fatalf("domain alone formed pair: %+v", groups)
	}
	rows[1].VPCPeer = "10.0.0.1"
	if len(VPCGroups(rows)) != 1 {
		t.Fatal("reciprocal pair missing")
	}
}
func TestVPCNegativeKeepaliveIsFault(t *testing.T) {
	o, err := vxlan.ParseRuntime("vpc", "vPC domain id : 1\nPeer status : peer adjacency formed ok\nvPC keep-alive status : peer is not alive\nConfiguration consistency status : success\n", true)
	if err != nil {
		t.Fatal(err)
	}
	if bad, _ := healthRows("vpc", o.Records); !bad {
		t.Fatal("negative keepalive accepted as healthy")
	}
}
func TestConfiguredRoutingInterfaceIsMonitored(t *testing.T) {
	config := vxlan.ParseConfig("interface Ethernet1/1\n  no switchport\n  ip address 10.0.0.1/31\n  ip router ospf UNDERLAY area 0.0.0.0\n", true)
	if len(config.Records) != 1 {
		t.Fatal("parse config")
	}
	rows := fabricInterfaceRows(config.Records[0], []vxlan.Record{{"interface": "Eth1/1", "state": "down"}})
	if len(rows) != 1 {
		t.Fatalf("OSPF transport was excluded: %+v %+v", rows, config.Records[0]["interfaces"])
	}
}
func TestConfiguredVNIAndBGPPeerMissingFromCompleteRuntimeAreFaults(t *testing.T) {
	config := vxlan.Record{"nve": vxlan.Record{"members": vxlan.Record{"10010": vxlan.Record{}}}, "bgp": vxlan.Record{"neighbors": vxlan.Record{"192.0.2.9": vxlan.Record{"address_families": vxlan.Record{"l2vpn evpn": vxlan.Record{}}}}}}
	for _, dimension := range []string{"nve_vnis", "bgp_neighbors"} {
		if !missingExpectedRuntime(dimension, config, nil) {
			t.Fatalf("missing configured %s accepted as healthy", dimension)
		}
	}
}
func TestEndpointVPCCandidatesRequirePairAndMatchingService(t *testing.T) {
	rows := []dto.Endpoint{{FabricID: 1, DeviceID: 1, MAC: "aa:bb:cc:dd:ee:ff", VRF: "red", VNI: 10010, State: "current", Kind: "local_port", VPCID: "10", EvidenceID: 1}, {FabricID: 1, DeviceID: 2, MAC: "aa:bb:cc:dd:ee:ff", VRF: "red", VNI: 10010, State: "current", Kind: "local_port", VPCID: "10", EvidenceID: 2}}
	proof := map[int]dto.Node{1: {DeviceID: 1, IP: "10.0.0.1", State: "healthy", VPCDomain: "1", VPCPeer: "10.0.0.2"}, 2: {DeviceID: 2, IP: "10.0.0.2", State: "healthy", VPCDomain: "1", VPCPeer: "10.0.0.1"}}
	classifyEndpointCandidates(rows, proof)
	if rows[0].Kind != "vpc_dual_homed" || rows[1].Kind != "vpc_dual_homed" || rows[0].CandidateCount != 2 {
		t.Fatalf("confirmed pair lost: %+v", rows)
	}
	rows[0].Kind = "local_port"
	rows[1].Kind = "local_port"
	rows[1].VRF = "blue"
	classifyEndpointCandidates(rows, proof)
	if rows[0].Kind == "vpc_dual_homed" {
		t.Fatal("cross-VRF duplicate joined as vPC")
	}
}
func TestVPCMemberFaultAndWrongRuntimeDomainRemainFaults(t *testing.T) {
	rows := []vxlan.Record{{"domain_id": 2, "peer_status": "peer adjacency formed ok", "keepalive_status": "peer is alive", "consistency_status": "success", "members": []vxlan.Record{{"state": "down", "consistency": "success"}}}}
	if bad, _ := healthRows("vpc", rows); !bad {
		t.Fatal("down vPC member accepted")
	}
	if !missingExpectedRuntime("vpc", vxlan.Record{"vpc_domain": vxlan.Record{"id": 1}}, rows) {
		t.Fatal("different runtime domain accepted")
	}
}
