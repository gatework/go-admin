package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	vxlan "github.com/gatework/go-vxlan"
	"go-admin/app/fabric/service/dto"
)

func TestAssessmentRepairsVPCUsesFinalMemberHealth(t *testing.T) {
	for _, state := range []string{"fault", "unknown"} {
		t.Run(state, func(t *testing.T) {
			nodes := []dto.Node{{DeviceID: 1, IP: "a", VPCDomain: "1", VPCPeer: "b", State: "healthy"}, {DeviceID: 2, IP: "b", VPCDomain: "1", VPCPeer: "a", State: "healthy"}}
			detail := dto.Detail{Nodes: nodes, VPCGroups: VPCGroups(nodes), IssueGroups: []dto.IssueGroup{{Devices: []dto.IssueDevice{{DeviceID: 2, Status: map[string]string{"fault": "affected", "unknown": "unknown"}[state]}}}}}
			detail.State = "healthy"
			reconcileIssueHealth(&detail)
			if detail.VPCGroups[0].State != state {
				t.Fatalf("pair=%s member=%s", detail.VPCGroups[0].State, detail.Nodes[1].State)
			}
		})
	}
}
func TestAssessmentRepairsDisabledNVEWithEnabledSVI(t *testing.T) {
	cfg := vxlan.Record{"nve": vxlan.Record{"interface": "nve1", "shutdown": true}, "interfaces": vxlan.Record{"Vlan8": vxlan.Record{"vrf": "tenant"}}}
	rows := fabricInterfaceRows(cfg, []vxlan.Record{{"interface": "nve1", "state": "down"}, {"interface": "Vlan8", "state": "up"}, {"interface": "nve99", "state": "down"}})
	if len(rows) != 1 || text(rows[0]["interface"]) != "Vlan8" {
		t.Fatalf("disabled/unconfigured NVE included: %+v", rows)
	}
}
func TestAssessmentRepairsSameVNIContextsAndPayload(t *testing.T) {
	nodes := []dto.Node{issueNode(t, 1, issueCompleteConfig), issueNode(t, 2, strings.ReplaceAll(issueCompleteConfig, "tenant", "blue"))}
	configs := vxlan.Record{}
	for _, n := range nodes {
		configs[n.IP] = currentIssueConfig(n)
	}
	segments := segmentMatrix(nodes, configs)
	if len(segments) != 2 {
		t.Fatalf("collapsed contexts: %+v", segments)
	}
	for _, segment := range segments {
		if segment.VRF == "" {
			t.Fatal("lost VRF")
		}
		for _, cell := range segment.Cells {
			if cell.Configuration["config"] != nil {
				t.Fatal("full device configuration repeated in service cell")
			}
			if cell.State == "missing" {
				t.Fatal("same VNI in another VRF mislabeled missing")
			}
		}
	}
	detail := dto.Detail{Nodes: nodes, Segments: segments}
	detail.CompleteScope = true
	attachRemediations(&detail)
	for _, f := range detail.Findings {
		if f.Remediation != nil && len(f.Remediation.TargetDeviceIDs) > 0 {
			t.Fatalf("cross-context repair hint: %+v", f)
		}
	}
}
func TestAssessmentRepairsAmbiguousFindingDoesNotAttachToTenant(t *testing.T) {
	detail := dto.Detail{Nodes: []dto.Node{issueNode(t, 1, issueCompleteConfig), issueNode(t, 2, strings.ReplaceAll(issueCompleteConfig, "tenant", "blue"))}, Findings: []dto.Finding{{VNI: 10008, Code: "fabric_gateway", Severity: "fault"}}}
	groups := buildIssueGroups(detail)
	standalone := false
	for _, g := range groups {
		if len(g.FindingIndices) == 0 {
			continue
		}
		if g.Check == "segment_configuration" {
			t.Fatalf("ambiguous finding attached to %s", g.VRF)
		}
		standalone = true
	}
	if !standalone {
		t.Fatal("unresolved finding disappeared")
	}
}
func BenchmarkAssessmentMatrixPayload(b *testing.B) {
	for _, count := range []int{10, 100, 500} {
		b.Run(fmt.Sprintf("vnis_%d", count), func(b *testing.B) {
			nodes := []dto.Node{}
			configs := vxlan.Record{}
			for device := 1; device <= 4; device++ {
				interfaces, mappings, members := vxlan.Record{}, vxlan.Record{}, vxlan.Record{}
				for i := 1; i <= count; i++ {
					interfaces[fmt.Sprintf("Vlan%d", i)] = vxlan.Record{"vrf": "tenant", "addresses": []string{fmt.Sprintf("10.%d.%d.254/24", i/256, i%256)}}
					mappings[fmt.Sprint(i)] = 10000 + i
					members[fmt.Sprint(10000+i)] = vxlan.Record{"ingress_replication": "bgp"}
				}
				ip := fmt.Sprint(device)
				configs[ip] = vxlan.Record{"interfaces": interfaces, "vlan_vnis": mappings, "nve": vxlan.Record{"members": members}}
				nodes = append(nodes, dto.Node{DeviceID: device, IP: ip, Role: "leaf_switch", ConfigState: "current"})
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				payload, err := json.Marshal(segmentMatrix(nodes, configs))
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(len(payload)), "serialized_bytes/op")
			}
		})
	}
}

func TestAssessmentRepairsScopedFindingMatchesEveryBusinessDimension(t *testing.T) {
	f := dto.Finding{Scope: "business", VRF: "tenant", VNI: 10008, Check: "acl_binding", Direction: "in", Code: "fabric_acl_binding"}
	matching := issueScope{vrf: "tenant", vni: 10008, check: "acl_binding", direction: "in"}
	if !findingMatchesIssue(f, matching) {
		t.Fatal("exact business scope did not match")
	}
	for name, scope := range map[string]issueScope{
		"vrf":       {vrf: "blue", vni: 10008, check: "acl_binding", direction: "in"},
		"vni":       {vrf: "tenant", vni: 20009, check: "acl_binding", direction: "in"},
		"check":     {vrf: "tenant", vni: 10008, check: "acl_policy", direction: "in"},
		"direction": {vrf: "tenant", vni: 10008, check: "acl_binding", direction: "out"},
	} {
		if findingMatchesIssue(f, scope) {
			t.Fatalf("finding crossed %s", name)
		}
	}
	f.Scope = "fabric"
	if findingMatchesIssue(f, matching) {
		t.Fatal("fabric-wide finding attached to business")
	}
}
func TestAssessmentRepairsCellProjectsOnlyBusinessFields(t *testing.T) {
	n := issueNode(t, 1, issueCompleteConfig)
	cfg := currentIssueConfig(n)
	segments := segmentMatrix([]dto.Node{n}, vxlan.Record{n.IP: cfg})
	encoded, err := json.Marshal(segments)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"config":`) || strings.Contains(string(encoded), `"config_digest":`) {
		t.Fatal("full device config serialized in cell")
	}
	for _, key := range []string{"vlan", "svi", "nve", "evpn", "missing"} {
		if _, ok := segments[0].Cells[0].Configuration[key]; !ok {
			t.Fatalf("missing projected field %s", key)
		}
	}
	if currentIssueConfig(n)["config_digest"] == nil {
		t.Fatal("node evidence lost original configuration")
	}
}

func TestAssessmentRepairsKnownFindingSupportsOnlyItsVRF(t *testing.T) {
	acl := "ip access-list PROTECT\n 10 permit ip any any\n"
	bound := strings.Replace(issueCompleteConfig, "  fabric forwarding mode anycast-gateway\n", "  fabric forwarding mode anycast-gateway\n  ip access-group PROTECT in\n", 1) + acl
	detail := dto.Detail{Nodes: []dto.Node{issueNode(t, 1, bound), issueNode(t, 2, issueCompleteConfig+acl), issueNode(t, 3, strings.ReplaceAll(bound, "tenant", "blue")), issueNode(t, 4, strings.ReplaceAll(issueCompleteConfig+acl, "tenant", "blue")), issueNode(t, 5, "")}, Findings: []dto.Finding{{Scope: "business", VRF: "tenant", VNI: 10008, Check: "acl_binding", Direction: "in", DeviceID: 2, Code: "fabric_acl_binding", Severity: "fault"}}}
	groups := buildIssueGroups(detail)
	tenant := issueBy(t, groups, "acl_binding", "tenant", 10008, "in")
	blue := issueBy(t, groups, "acl_binding", "blue", 10008, "in")
	if len(tenant.FindingIndices) != 1 || len(blue.FindingIndices) != 0 {
		t.Fatalf("cross-VRF evidence: tenant=%+v blue=%+v", tenant.FindingIndices, blue.FindingIndices)
	}
	for _, d := range tenant.Devices {
		if d.DeviceID == 1 && d.Status != "healthy" {
			t.Fatalf("positive source changed: %+v", d)
		}
		if d.DeviceID == 5 && d.Status != "unknown" {
			t.Fatalf("unknown source changed: %+v", d)
		}
		if d.DeviceID == 2 && len(d.FindingIndices) != 1 {
			t.Fatalf("known target lost evidence: %+v", d)
		}
		if d.DeviceID != 2 && len(d.FindingIndices) != 0 {
			t.Fatalf("supporting evidence crossed device: %+v", d)
		}
	}
}

func TestAssessmentRepairsEvidenceKeepsExplicitBusinessAndFabricScopes(t *testing.T) {
	detail := dto.Detail{Nodes: []dto.Node{issueNode(t, 1, issueCompleteConfig), issueNode(t, 2, strings.ReplaceAll(issueCompleteConfig, "tenant", "blue"))}, Findings: []dto.Finding{
		{Scope: "business", VRF: "tenant", Check: "acl_binding", Direction: "in", DeviceID: 1, VNI: 10008, Code: "acl_in_unbound", Severity: "unknown"},
		{Scope: "business", VRF: "blue", Check: "acl_binding", Direction: "in", DeviceID: 2, VNI: 10008, Code: "acl_in_unbound", Severity: "unknown"},
		{VNI: 10008, Code: "fabric_gateway", Severity: "fault"},
	}}
	groups := buildIssueGroups(detail)
	for vrf, index := range map[string]int{"tenant": 0, "blue": 1} {
		group := issueBy(t, groups, "acl_binding", vrf, 10008, "in")
		if len(group.FindingIndices) != 1 || group.FindingIndices[0] != index {
			t.Fatalf("mixed business evidence: %+v", group)
		}
	}
	group := issueBy(t, groups, "fabric_gateway", "", 10008, "")
	encoded, err := json.Marshal(group)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]any
	if err := json.Unmarshal(encoded, &shape); err != nil {
		t.Fatal(err)
	}
	if shape["scope"] != "fabric" {
		t.Fatalf("ambiguous scope presented as default VRF: %s", encoded)
	}
}
