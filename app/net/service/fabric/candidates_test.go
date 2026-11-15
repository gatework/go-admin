package fabric

import (
	"context"
	"strings"
	"testing"

	"github.com/gatework/motor/app/net/models/delivery"
	models "github.com/gatework/motor/app/net/models/fabric"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
)

func TestChangeCandidatesSelectOnlyMissingCompatibleTargets(t *testing.T) {
	s := operationsFixture(t)
	ctx := context.Background()
	d := coreDevice(t, s, "SZX9-LEF-4", "192.0.2.4", 7)
	if err := s.DB.Create(&delivery.Connection{DeviceID: d.ID, Profile: "cisco_nxos", Protocol: "ssh", Port: 22, Version: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.VisibleMembers(ctx, 1, operationsScope()); err != nil {
		t.Fatal(err)
	}
	seedConfig(t, s, 1, gatewayConfig)
	seedConfig(t, s, 2, missingGatewayConfig)
	seedConfig(t, s, 3, gatewayConfig)
	seedConfig(t, s, d.ID, strings.ReplaceAll(missingGatewayConfig, "tenant", "other"))
	r, err := s.ChangeCandidates(ctx, 1, &dto.CandidatesReq{SourceDeviceID: 1, Kind: "svi_gateway_add", VNI: 10008}, operationsScope())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Ready) != 1 || r.Ready[0].DeviceID != 2 {
		t.Fatalf("only missing target should be ready: %+v", r)
	}
	if len(r.Unchanged) != 1 || r.Unchanged[0].DeviceID != 3 {
		t.Fatalf("existing gateway not excluded: %+v", r)
	}
	if len(r.Blocked) != 1 || r.Blocked[0].DeviceID != d.ID {
		t.Fatalf("cross-service target not blocked: %+v", r)
	}
	narrowed, err := s.ChangeCandidates(ctx, 1, &dto.CandidatesReq{SourceDeviceID: 1, Kind: "svi_gateway_add", VNI: 10008, TargetDeviceIDs: []int{3}}, operationsScope())
	if err != nil || len(narrowed.Ready) != 0 || len(narrowed.Unchanged) != 1 {
		t.Fatalf("problem target scope widened: %+v %v", narrowed, err)
	}
	if _, err = s.ChangeCandidates(ctx, 1, &dto.CandidatesReq{SourceDeviceID: 1, Kind: "svi_gateway_add", VNI: 10008, TargetDeviceIDs: []int{999}}, operationsScope()); err == nil {
		t.Fatal("unknown target accepted")
	}
	if _, err = s.ChangeCandidates(ctx, 1, &dto.CandidatesReq{SourceDeviceID: 1, Kind: "svi_gateway_add", VNI: 10008, TargetDeviceIDs: []int{}}, operationsScope()); err == nil {
		t.Fatal("explicit empty issue scope widened to all targets")
	}
}

func TestChangeCandidatesACLUsesMissingDirectionAndMatchingPolicy(t *testing.T) {
	acl := "ip access-list PROTECT\n 10 permit ip any any\n"
	bound := strings.Replace(gatewayConfig, "  fabric forwarding mode anycast-gateway\n", "  fabric forwarding mode anycast-gateway\n  ip access-group PROTECT out\n", 1) + acl
	for _, tc := range []struct{ name, config, group, reason string }{
		{"missing outbound", gatewayConfig + acl, "ready", "missing_increment"},
		{"inbound does not imply outbound", strings.Replace(bound, "PROTECT out", "PROTECT in", 1), "ready", "missing_increment"},
		{"already bound", bound, "unchanged", "already_consistent"},
		{"different binding", strings.Replace(bound, "PROTECT out", "OTHER out", 1) + "ip access-list OTHER\n 10 permit ip any any\n", "blocked", "existing_binding_conflict"},
		{"ACL absent", gatewayConfig, "blocked", "acl_definition_missing"},
		{"different policy", gatewayConfig + strings.Replace(acl, "permit", "deny", 1), "blocked", "acl_policy_differs"},
		{"other VRF", strings.ReplaceAll(gatewayConfig, "tenant", "other") + acl, "blocked", "service_context_mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := operationsFixture(t)
			seedConfig(t, s, 1, bound)
			seedConfig(t, s, 2, tc.config)
			r, err := s.ChangeCandidates(context.Background(), 1, &dto.CandidatesReq{SourceDeviceID: 1, Kind: "svi_acl", VNI: 10008, Direction: "out", TargetDeviceIDs: []int{2}}, operationsScope())
			if err != nil {
				t.Fatal(err)
			}
			groups := map[string][]dto.Candidate{"ready": r.Ready, "unchanged": r.Unchanged, "blocked": r.Blocked}
			if len(r.Ready)+len(r.Unchanged)+len(r.Blocked) != 1 || len(groups[tc.group]) != 1 || groups[tc.group][0].Reason != tc.reason {
				t.Fatalf("unexpected candidate classification: %+v", r)
			}
		})
	}
}

func TestChangeCandidatesKeepsEvidenceFailuresPerTarget(t *testing.T) {
	s := operationsFixture(t)
	seedConfig(t, s, 1, gatewayConfig)
	seedConfig(t, s, 2, missingGatewayConfig)
	seedConfig(t, s, 3, missingGatewayConfig)
	if err := s.DB.Model(&models.Observation{}).Where("device_id = ? AND dimension = ?", 3, "configuration").Update("status", "failed").Error; err != nil {
		t.Fatal(err)
	}
	r, err := s.ChangeCandidates(context.Background(), 1, &dto.CandidatesReq{SourceDeviceID: 1, Kind: "svi_gateway_add", VNI: 10008}, operationsScope())
	if err != nil {
		t.Fatalf("one unavailable target prevented classification of all devices: %v", err)
	}
	if len(r.Ready) != 1 || r.Ready[0].DeviceID != 2 || len(r.Blocked) != 1 || r.Blocked[0].DeviceID != 3 || r.Blocked[0].Reason != "evidence_unavailable" {
		t.Fatalf("unavailable evidence must be blocked individually: %+v", r)
	}
}

func TestRemediationHintsNeverSelectConsistentOrUnknownLeaf(t *testing.T) {
	d := dto.Detail{CompleteScope: true, Nodes: []dto.Node{{DeviceID: 1, Role: "leaf_switch", ConfigState: "current"}, {DeviceID: 2, Role: "leaf_switch", ConfigState: "current"}, {DeviceID: 3, Role: "leaf_switch", ConfigState: "historical"}}, Segments: []dto.Segment{{VNI: 10008, State: "different", Cells: []dto.SegmentCell{{DeviceID: 1, State: "present", ACLIn: "APP", ACLOut: "unbound", Configuration: map[string]any{"svi": map[string]any{"addresses": []string{"192.0.2.254/24"}}}}, {DeviceID: 2, State: "present", ACLIn: "unbound", ACLOut: "unbound", Configuration: map[string]any{"svi": map[string]any{"addresses": []string{}}}}, {DeviceID: 3, State: "unknown", ACLIn: "unbound", Configuration: map[string]any{}}}}}}
	attachRemediations(&d)
	found := map[string]bool{}
	for _, finding := range d.Findings {
		if finding.Remediation != nil {
			r := finding.Remediation
			if len(r.TargetDeviceIDs) != 1 || r.TargetDeviceIDs[0] != 2 {
				t.Fatalf("unsafe hint: %+v", r)
			}
			found[r.Kind+":"+r.Direction] = true
		}
	}
	if !found["svi_gateway_add:"] || !found["svi_acl:in"] {
		t.Fatalf("missing precise actions: %+v", d.Findings)
	}
	if found["svi_acl:out"] {
		t.Fatal("all-unbound ACL invented a policy")
	}
}
