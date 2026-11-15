package fabric

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	vxlan "github.com/gatework/go-vxlan"
	models "github.com/gatework/motor/app/net/models/fabric"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
)

const issueCompleteConfig = gatewayConfig + "interface nve1\n member vni 10008\n  ingress-replication protocol bgp\n"

func issueNode(t *testing.T, id int, body string) dto.Node {
	t.Helper()
	n := dto.Node{DeviceID: id, IP: string(rune('a' + id)), Role: "leaf_switch", Supported: true, ConfigState: "current", Dimensions: map[string]dto.Evidence{}}
	if body != "" {
		parsed := vxlan.ParseConfig(body, true)
		if !parsed.Complete() {
			t.Fatalf("fixture: %+v issues=%+v body=%s", parsed, parsed.Issues, body)
		}
		o := &models.Observation{ID: id, Dimension: "configuration", Status: "complete", Data: parsed, ObservedAt: time.Now().UTC()}
		n.Dimensions["configuration"] = dto.Evidence{State: "complete", Latest: o, LastComplete: o}
	}
	return n
}
func issueBy(t *testing.T, groups []dto.IssueGroup, check, vrf string, vni int, direction string) dto.IssueGroup {
	t.Helper()
	for _, g := range groups {
		if g.Check == check && g.VRF == vrf && g.VNI == vni && g.Direction == direction {
			return g
		}
	}
	t.Fatalf("missing group %s/%s/%d/%s: %+v", check, vrf, vni, direction, groups)
	return dto.IssueGroup{}
}
func issueStatuses(g dto.IssueGroup) map[int]string {
	out := map[int]string{}
	for _, d := range g.Devices {
		out[d.DeviceID] = d.Status
	}
	return out
}
func TestIssuesPresenceDistinguishesPositiveUnknownAndExcludedRole(t *testing.T) {
	nodes := []dto.Node{issueNode(t, 1, issueCompleteConfig), issueNode(t, 2, "hostname absent\n"), issueNode(t, 3, ""), issueNode(t, 4, issueCompleteConfig)}
	nodes[3].Role = "spine_switch"
	detail := dto.Detail{Nodes: nodes,
		CompleteScope: false}
	groups := buildIssueGroups(detail)
	g := issueBy(t, groups, "segment_presence", "tenant", 10008, "")
	want := map[int]string{1: "healthy", 2: "affected", 3: "unknown", 4: "not_applicable"}
	if !reflect.DeepEqual(issueStatuses(g), want) || g.Counts.Affected != 1 || g.Counts.Healthy != 1 || g.Counts.Unknown != 1 || g.Counts.NotApplicable != 1 || g.CompleteScope {
		t.Fatalf("outcomes %+v", g)
	}
	for _, d := range g.Devices {
		if d.DeviceID == 1 && (len(d.EvidenceIDs) != 1 || d.ObservedAt == nil) {
			t.Fatal("missing positive evidence", d)
		}
	}
}
func TestIssuesConfigurationVariantsNeverChooseMajority(t *testing.T) {
	a := issueCompleteConfig
	b := strings.Replace(a, "192.0.2.254/24", "192.0.2.253/24", 1)
	detail := dto.Detail{Nodes: []dto.Node{issueNode(t, 1, a), issueNode(t, 2, a), issueNode(t, 3, b)},
		CompleteScope: true}
	g := issueBy(t, buildIssueGroups(detail), "segment_configuration", "tenant", 10008, "")
	if g.Counts.Affected != 3 || g.Counts.Healthy != 0 {
		t.Fatalf("majority guessed as baseline: %+v", g)
	}
}
func TestIssuesBusinessAndACLDirectionRemainIsolatedAndStable(t *testing.T) {
	acl := "ip access-list PROTECT\n 10 permit ip any any\n"
	a := strings.Replace(issueCompleteConfig, "  fabric forwarding mode anycast-gateway\n", "  fabric forwarding mode anycast-gateway\n  ip access-group PROTECT in\n", 1) + acl
	b := issueCompleteConfig + acl
	other := strings.NewReplacer("tenant", "blue", "10008", "20009", "Vlan8", "Vlan9", "vlan 8", "vlan 9", "50000", "50001", "192.0.2.", "192.0.3.").Replace(issueCompleteConfig)
	detail := dto.Detail{Nodes: []dto.Node{issueNode(t, 1, strings.Replace(a, "  ingress-replication protocol bgp\n", "  ingress-replication protocol bgp\n member vni 20009\n  ingress-replication protocol bgp\n", 1)+strings.Replace(strings.Replace(other, "hostname test\n", "", 1), "interface nve1\n member vni 20009\n  ingress-replication protocol bgp\n", "", 1)), issueNode(t, 2, b)},
		CompleteScope: true}
	groups := buildIssueGroups(detail)
	in := issueBy(t, groups, "acl_binding", "tenant", 10008, "in")
	out := issueBy(t, groups, "acl_binding", "tenant", 10008, "out")
	blue := issueBy(t, groups, "segment_presence", "blue", 20009, "")
	if in.Counts.Affected != 1 || in.Counts.Healthy != 1 || out.Counts.Unknown != 2 || len(out.Remediations) != 0 || in.Key == out.Key || in.Key == blue.Key {
		t.Fatalf("business isolation: %+v %+v %+v", in, out, blue)
	}
	detail.Nodes[0], detail.Nodes[1] = detail.Nodes[1], detail.Nodes[0]
	if issueBy(t, buildIssueGroups(detail), "acl_binding", "tenant", 10008, "in").Key != in.Key {
		t.Fatal("unstable key")
	}
}
func TestIssuesStaleEvidenceCannotPassRuntimeAndUnconfiguredRequiresCurrentConfig(t *testing.T) {
	a := issueNode(t, 1, "hostname test\n")
	b := issueNode(t, 2, "")
	c := issueNode(t, 3, "hostname test\nrouter ospf 1\n")
	c.Dimensions["ospf_neighbors"] = dto.Evidence{State: "unknown", Stale: true, Latest: &models.Observation{ID: 30, Status: "complete", Data: vxlan.Observation{Records: []vxlan.Record{{"state": "FULL/DR"}}}}}
	d := dto.Detail{Nodes: []dto.Node{a, b, c}}
	g := issueBy(t, buildIssueGroups(d), "runtime_ospf_neighbors", "", 0, "")
	if !reflect.DeepEqual(issueStatuses(g), map[int]string{1: "not_applicable", 2: "unknown", 3: "unknown"}) || g.Devices[2].Reason != "evidence_stale" {
		t.Fatalf("unproven health: %+v", g)
	}
}
func TestIssuesRuntimeReusesExpectedMembershipAndPositiveHealth(t *testing.T) {
	cfg := gatewayConfig + "interface nve1\n member vni 10008\n  ingress-replication protocol bgp\n"
	a, b, c := issueNode(t, 1, cfg), issueNode(t, 2, cfg), issueNode(t, 3, cfg)
	for _, n := range []*dto.Node{&a, &b, &c} {
		n.Dimensions["nve_vnis"] = dto.Evidence{State: "complete", Latest: &models.Observation{ID: 10 + n.DeviceID, Status: "complete", Data: vxlan.Observation{Records: []vxlan.Record{{"state": "up", "vni": 10008}}}}}
	}
	b.Dimensions["nve_vnis"].Latest.Data.Records = nil
	c.Dimensions["nve_vnis"].Latest.Data.Records[0]["state"] = "down"
	g := issueBy(t, buildIssueGroups(dto.Detail{Nodes: []dto.Node{a, b, c}}), "runtime_nve_vnis", "", 0, "")
	if !reflect.DeepEqual(issueStatuses(g), map[int]string{1: "healthy", 2: "affected", 3: "affected"}) {
		t.Fatalf("runtime assessment %+v", g)
	}
}
func TestIssuesRemediationOnlyRetainsKnownAffectedTargetsAndDeduplicatesFindings(t *testing.T) {
	d := dto.Detail{Nodes: []dto.Node{issueNode(t, 1, issueCompleteConfig), issueNode(t, 2, "hostname absent\n"), issueNode(t, 3, "")},
		CompleteScope: true}
	repair := dto.Remediation{Kind: "vlan_vni_add", VNI: 10008, TargetDeviceIDs: []int{1, 2, 2, 3, 999}}
	d.Findings = []dto.Finding{{VNI: 10008, Code: "service_matrix_different", Severity: "fault"}, {VNI: 10008, Code: "repair_vlan_vni_add_", Severity: "warning", Remediation: &repair}}
	g := issueBy(t, buildIssueGroups(d), "segment_presence", "tenant", 10008, "")
	if len(g.Devices) != 3 || len(g.Remediations) != 1 || !reflect.DeepEqual(g.Remediations[0].TargetDeviceIDs, []int{2}) || len(g.FindingIndices) != 1 {
		t.Fatalf("unsafe remediation/counts %+v", g)
	}
	d.CompleteScope = false
	if len(issueBy(t, buildIssueGroups(d), "segment_presence", "tenant", 10008, "").Remediations) != 0 {
		t.Fatal("partial remediation")
	}
}
func TestDetailPersistsIssueContractAndAssessmentVersion(t *testing.T) {
	s := operationsFixture(t)
	if err := s.DB.AutoMigrate(&models.Assessment{}); err != nil {
		t.Fatal(err)
	}
	seedConfig(t, s, 1, gatewayConfig)
	out, err := s.Detail(context.Background(), 1, operationsScope())
	if err != nil {
		t.Fatal(err)
	}
	if out.IssueCount != len(out.IssueGroups) || out.IssueCount == 0 {
		t.Fatalf("missing issue count %+v", out.Summary)
	}
	var a models.Assessment
	if err = s.DB.First(&a, out.AssessmentID).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(a.Result), `"issueGroups"`) {
		t.Fatal("issue output not frozen")
	}
}

func TestIssuesSameVNIInDifferentVRFsCannotOfferCrossTenantRepair(t *testing.T) {
	red := issueNode(t, 1, issueCompleteConfig)
	blue := issueNode(t, 2, strings.ReplaceAll(issueCompleteConfig, "tenant", "blue"))
	d := dto.Detail{Nodes: []dto.Node{red, blue},
		CompleteScope: true}
	r := dto.Remediation{Kind: "vlan_vni_add", VNI: 10008, TargetDeviceIDs: []int{1, 2}}
	d.Findings = []dto.Finding{{VNI: 10008, Code: "repair_vlan_vni_add_", Remediation: &r}}
	groups := buildIssueGroups(d)
	a := issueBy(t, groups, "segment_presence", "tenant", 10008, "")
	b := issueBy(t, groups, "segment_presence", "blue", 10008, "")
	if a.Key == b.Key || a.Devices[1].Reason != "service_context_conflict" || b.Devices[0].Reason != "service_context_conflict" || len(a.Remediations)+len(b.Remediations) != 0 {
		t.Fatalf("cross VRF conflated: %+v %+v", a, b)
	}
	for _, g := range groups {
		if g.Check == "segment_configuration" && g.Counts.Healthy > 0 {
			t.Fatalf("cross VRF config deemed consistent: %+v", g)
		}
	}
}
func TestIssuesIncompleteConfigCannotProveHealthyAndUnknownLatestOverridesHistory(t *testing.T) {
	a := issueNode(t, 1, issueCompleteConfig)
	b := issueNode(t, 2, issueCompleteConfig)
	evidence := b.Dimensions["configuration"]
	evidence.State = "unknown"
	evidence.Stale = true
	b.Dimensions["configuration"] = evidence
	g := issueBy(t, buildIssueGroups(dto.Detail{Nodes: []dto.Node{a, b}}), "segment_configuration", "tenant", 10008, "")
	if g.Counts.Healthy != 0 || g.Counts.Unknown != 2 {
		t.Fatalf("historical evidence passed: %+v", g)
	}
}
func TestIssuesFallbackNeverTreatsAbsentFindingAsPassingOrLeaksHiddenIDs(t *testing.T) {
	d := dto.Detail{Nodes: []dto.Node{issueNode(t, 1, issueCompleteConfig), issueNode(t, 2, issueCompleteConfig)}, Findings: []dto.Finding{{Code: "unmodeled", DeviceID: 1, Severity: "fault"}, {Code: "unmodeled", DeviceID: 1, Severity: "fault"}, {Code: "other", DeviceID: 999, Severity: "fault"}}}
	g := issueBy(t, buildIssueGroups(d), "unmodeled", "", 0, "")
	if g.Counts.Affected != 1 || g.Counts.Unknown != 1 || len(g.Devices) != 2 || len(g.FindingIndices) != 2 || len(g.Devices[0].FindingIndices) != 2 {
		t.Fatalf("fallback leaked/guessed: %+v", g)
	}
}

func TestIssuesGatewayHintKeepsOnlyAffectedBusinessTargets(t *testing.T) {
	a := issueNode(t, 1, issueCompleteConfig)
	b := issueNode(t, 2, strings.Replace(issueCompleteConfig, "  ip address 192.0.2.254/24\n", "", 1))
	c := issueNode(t, 3, strings.ReplaceAll(issueCompleteConfig, "tenant", "other"))
	r := dto.Remediation{Kind: "svi_gateway_add", VNI: 10008, TargetDeviceIDs: []int{1, 2, 3}}
	d := dto.Detail{Nodes: []dto.Node{a, b, c}, Findings: []dto.Finding{{Scope: "business", VRF: "tenant", Check: "segment_configuration", VNI: 10008, Code: "repair_svi_gateway_add_", Remediation: &r}},
		CompleteScope: true}
	g := issueBy(t, buildIssueGroups(d), "segment_configuration", "tenant", 10008, "")
	if len(g.Remediations) != 1 || !reflect.DeepEqual(g.Remediations[0].TargetDeviceIDs, []int{2}) {
		t.Fatalf("gateway target leaked %+v", g)
	}
}
func TestIssuesValuesRemainReadableAndBounded(t *testing.T) {
	a := issueNode(t, 1, issueCompleteConfig)
	b := issueNode(t, 2, strings.Replace(issueCompleteConfig, "192.0.2.254", "192.0.2.253", 1))
	g := issueBy(t, buildIssueGroups(dto.Detail{Nodes: []dto.Node{a, b}}), "segment_configuration", "tenant", 10008, "")
	for _, d := range g.Devices {
		if len(d.Value) > 160 || strings.HasPrefix(d.Value, "{") || !strings.Contains(d.Value, "192.0.2.") {
			t.Fatalf("not a readable table value: %s", d.Value)
		}
	}
}

func TestDetailNodeHealthIncludesBusinessIssueOutcomes(t *testing.T) {
	s := operationsFixture(t)
	if err := s.DB.AutoMigrate(&models.Assessment{}); err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 3; id++ {
		cfg := issueCompleteConfig + " member vni 50000 associate-vrf\n"
		if id == 3 {
			cfg = strings.Replace(cfg, "192.0.2.254", "192.0.2.253", 1)
		}
		seedConfig(t, s, id, cfg)
		var config models.Observation
		if err := s.DB.Where("device_id=? AND dimension='configuration'", id).First(&config).Error; err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"nve_peers", "nve_vnis", "interfaces", "port_channels"} {
			row := config
			row.ID = 0
			row.Dimension = name
			row.Data = vxlan.Observation{Name: name, Status: "complete", Records: []vxlan.Record{}}
			if name == "nve_peers" {
				row.Data.Records = []vxlan.Record{{"state": "up"}}
			}
			if name == "nve_vnis" {
				row.Data.Records = []vxlan.Record{{"state": "up", "vni": 10008}, {"state": "up", "vni": 50000}}
			}
			if err := s.DB.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	out, err := s.Detail(context.Background(), 1, operationsScope())
	if err != nil {
		t.Fatal(err)
	}
	g := issueBy(t, out.IssueGroups, "segment_configuration", "tenant", 10008, "")
	if g.Counts.Affected != 3 {
		t.Fatalf("fixture didn't prove business variant: %+v", g)
	}
	for _, n := range out.Nodes {
		if n.State != "fault" {
			t.Fatalf("business-affected node state remains %s", n.State)
		}
	}
	if out.State != "fault" {
		t.Fatalf("business fault summary %s", out.State)
	}
}

func TestIssuesBorderConsistencyFindingDoesNotBecomeLeafOutcome(t *testing.T) {
	leaf := issueNode(t, 1, issueCompleteConfig)
	border := issueNode(t, 2, issueCompleteConfig)
	border.Role = "border_switch"
	d := dto.Detail{Nodes: []dto.Node{leaf, border}, Findings: []dto.Finding{{Code: "fabric_gateway", VNI: 10008, Role: "borders", Severity: "fault", Evidence: []vxlan.Record{{"host": border.IP, "value": []string{"192.0.2.254/24"}}}}}}
	g := issueBy(t, buildIssueGroups(d), "fabric_gateway", "", 10008, "")
	if g.Devices[0].Status != "not_applicable" || g.Devices[1].Status != "affected" {
		t.Fatalf("border finding applied to Leaf: %+v", g)
	}
}

func TestIssuesACLPolicyCannotProveConsistencyWithUnresolvedPeer(t *testing.T) {
	bound := strings.Replace(issueCompleteConfig, "  fabric forwarding mode anycast-gateway\n", "  fabric forwarding mode anycast-gateway\n  ip access-group PROTECT in\n", 1)
	a := issueNode(t, 1, bound+"ip access-list PROTECT\n 10 permit ip any any\n")
	b := issueNode(t, 2, bound)
	g := issueBy(t, buildIssueGroups(dto.Detail{Nodes: []dto.Node{a, b}}), "acl_policy", "tenant", 10008, "in")
	if g.Devices[0].Status != "unknown" || g.Devices[0].Reason != "comparison_incomplete" || g.Devices[1].Status != "affected" || g.Devices[1].Reason != "acl_definition_missing" {
		t.Fatalf("unresolved policy peer proved consistency: %+v", g)
	}
}

func TestIssuesACLHintsKeepEachVRFAndDirectionTargets(t *testing.T) {
	acl := "ip access-list PROTECT\n 10 permit ip any any\n"
	bound := strings.Replace(issueCompleteConfig, "  fabric forwarding mode anycast-gateway\n", "  fabric forwarding mode anycast-gateway\n  ip access-group PROTECT in\n", 1)
	nodes := []dto.Node{issueNode(t, 1, bound+acl), issueNode(t, 2, issueCompleteConfig+acl), issueNode(t, 3, strings.ReplaceAll(bound+acl, "tenant", "blue")), issueNode(t, 4, strings.ReplaceAll(issueCompleteConfig+acl, "tenant", "blue"))}
	r := dto.Remediation{Kind: "svi_acl", VNI: 10008, Direction: "in", TargetDeviceIDs: []int{2, 4}}
	d := dto.Detail{Nodes: nodes, Findings: []dto.Finding{
		{Scope: "business", VRF: "tenant", Check: "acl_binding", Direction: "in", VNI: 10008, Code: "repair_svi_acl_in", Remediation: &r},
		{Scope: "business", VRF: "blue", Check: "acl_binding", Direction: "in", VNI: 10008, Code: "repair_svi_acl_in", Remediation: &r},
	},
		CompleteScope: true}
	for vrf, target := range map[string]int{"tenant": 2, "blue": 4} {
		g := issueBy(t, buildIssueGroups(d), "acl_binding", vrf, 10008, "in")
		if len(g.Remediations) != 1 || !reflect.DeepEqual(g.Remediations[0].TargetDeviceIDs, []int{target}) {
			t.Fatalf("VRF hint targets leaked: %+v", g)
		}
		out := issueBy(t, buildIssueGroups(d), "acl_binding", vrf, 10008, "out")
		if len(out.Remediations) != 0 {
			t.Fatal("in hint copied to out")
		}
	}
}
func TestIssuesACLPolicyVariantsMarkAllObservedVariantsAffected(t *testing.T) {
	bound := strings.Replace(issueCompleteConfig, "  fabric forwarding mode anycast-gateway\n", "  fabric forwarding mode anycast-gateway\n  ip access-group PROTECT in\n", 1)
	a := bound + "ip access-list PROTECT\n 10 permit ip any any\n"
	b := bound + "ip access-list PROTECT\n 10 deny ip any any\n"
	g := issueBy(t, buildIssueGroups(dto.Detail{Nodes: []dto.Node{issueNode(t, 1, a), issueNode(t, 2, a), issueNode(t, 3, b)}}), "acl_policy", "tenant", 10008, "in")
	if g.Counts.Affected != 3 || g.Counts.Healthy != 0 {
		t.Fatalf("ACL majority chosen as correct: %+v", g)
	}
}
func TestIssuesOnlyNonhealthyGroupsAppear(t *testing.T) {
	n := issueNode(t, 1, "hostname standalone\n")
	for _, name := range []string{"interfaces", "port_channels"} {
		n.Dimensions[name] = dto.Evidence{State: "complete", Latest: &models.Observation{ID: 10, Data: vxlan.Observation{Status: "complete"}}}
	}
	groups := buildIssueGroups(dto.Detail{Nodes: []dto.Node{n}})
	if len(groups) != 0 {
		t.Fatalf("healthy or inapplicable checks entered problem list: %+v", groups)
	}
}

func TestIssuesUnattributedFaultCannotAssignAffectedDevices(t *testing.T) {
	d := dto.Detail{Nodes: []dto.Node{issueNode(t, 1, issueCompleteConfig), issueNode(t, 2, issueCompleteConfig)}, Findings: []dto.Finding{{Code: "unattributed", Severity: "fault", Message: "Review returned no device attribution"}}}
	g := issueBy(t, buildIssueGroups(d), "unattributed", "", 0, "")
	if g.Counts.Affected != 0 || g.Counts.Unknown != 2 || len(g.Remediations) != 0 {
		t.Fatalf("invented affected attribution: %+v", g)
	}
}

func TestIssuesRuntimeInterfaceEvidenceMustCoverConfiguredFabricInterfaces(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		rows   []vxlan.Record
		status string
	}{
		{"unrelated_only", []vxlan.Record{{"interface": "Ethernet1/49", "state": "up"}}, "unknown"},
		{"missing_nve", []vxlan.Record{{"interface": "Vlan8", "state": "up"}}, "unknown"},
		{"all_present", []vxlan.Record{{"interface": "Vlan8", "state": "up"}, {"interface": "nve1", "state": "up"}}, "healthy"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			n := issueNode(t, 1, issueCompleteConfig)
			n.Dimensions["interfaces"] = dto.Evidence{State: "complete", Latest: &models.Observation{ID: 99, Status: "complete", Data: vxlan.Observation{Status: "complete", Records: scenario.rows}}}
			d := runtimeIssueCheck("interfaces").evaluate(n)
			if d.Status != scenario.status {
				t.Fatalf("missing configured evidence: %+v", d)
			}
		})
	}
}
func TestIssuesRuntimePortChannelEvidenceMustCoverConfiguredFabricBundles(t *testing.T) {
	for _, rows := range [][]vxlan.Record{nil, {{"interface": "Po20", "flags": "SU"}}} {
		n := issueNode(t, 1, "hostname bundle\ninterface port-channel10\n vpc 10\n")
		n.Dimensions["port_channels"] = dto.Evidence{State: "complete", Latest: &models.Observation{ID: 99, Status: "complete", Data: vxlan.Observation{Status: "complete", Records: rows}}}
		d := runtimeIssueCheck("port_channels").evaluate(n)
		if d.Status != "unknown" {
			t.Fatalf("missing bundle was not unknown: %+v", d)
		}
	}
	n := issueNode(t, 1, "hostname bundle\ninterface port-channel10\n vpc 10\n")
	n.Dimensions["port_channels"] = dto.Evidence{State: "complete", Latest: &models.Observation{ID: 99, Status: "complete", Data: vxlan.Observation{Status: "complete", Records: []vxlan.Record{{"interface": "Po10", "flags": "SU"}}}}}
	if d := runtimeIssueCheck("port_channels").evaluate(n); d.Status != "healthy" {
		t.Fatalf("positively observed bundle not healthy: %+v", d)
	}
}
func TestIssuesDisabledAndAdminDownFabricInterfacesAreFaults(t *testing.T) {
	for _, state := range []string{"disabled", "admin-down", "down", "notconnect"} {
		n := issueNode(t, 1, issueCompleteConfig)
		n.Dimensions["interfaces"] = dto.Evidence{State: "complete", Latest: &models.Observation{ID: 99, Status: "complete", Data: vxlan.Observation{Status: "complete", Records: []vxlan.Record{{"interface": "Vlan8", "state": state}, {"interface": "nve1", "state": "up"}}}}}
		d := runtimeIssueCheck("interfaces").evaluate(n)
		if d.Status != "affected" {
			t.Errorf("state %s was not fault: %+v", state, d)
		}
	}
}
