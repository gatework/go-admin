package fabric

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sort"
	"strconv"

	"gorm.io/gorm"

	vxlan "github.com/gatework/go-vxlan"
	"github.com/gatework/motor-core/sdk/contract/actions"
	networkmodels "github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
)

// ChangeCandidates provides the change candidates operation within the caller's authorized fabric scope.
func (s *Fabric) ChangeCandidates(ctx context.Context, id int, r *dto.CandidatesReq, p *actions.DataPermission) (dto.Candidates, error) {
	out := dto.Candidates{Ready: []dto.Candidate{}, Unchanged: []dto.Candidate{}, Blocked: []dto.Candidate{}}
	if r == nil || r.SourceDeviceID <= 0 || r.VNI < 1 || r.VNI > 16777215 || len(r.TargetDeviceIDs) > 100 || (r.TargetDeviceIDs != nil && len(r.TargetDeviceIDs) == 0) {
		return out, InvalidInputErr
	}
	devices, err := s.RequireFullScope(ctx, id, p)
	if err != nil {
		return out, err
	}
	eligibleIDs := map[int]bool{}
	sourcePresent := false
	for _, d := range devices {
		if d.DeviceRole == "leaf_switch" {
			eligibleIDs[d.ID] = true
			sourcePresent = sourcePresent || d.ID == r.SourceDeviceID
		}
	}
	if !sourcePresent {
		return out, ScopeErr
	}
	requested, err := candidateTargets(r, eligibleIDs)
	if err != nil {
		return out, err
	}
	_, source, err := s.changeConfig(ctx, id, r.SourceDeviceID)
	if err != nil {
		return out, err
	}
	plan, err := controlledPlan(source, &dto.ChangeCreateReq{SourceDeviceID: r.SourceDeviceID, Kind: r.Kind, VNI: r.VNI, Direction: r.Direction})
	if err != nil {
		return out, err
	}
	out.SourceDeviceID = r.SourceDeviceID
	out.Kind = r.Kind
	out.VNI = r.VNI
	out.Direction = r.Direction
	if err := s.evaluateCandidates(ctx, id, devices, r, eligibleIDs, requested, source, plan, &out); err != nil {
		return out, err
	}
	return out, nil
}

// Reconstruct only the absent increment and ask the domain plan whether the
// actual state is its verified result. No device or caller-owned map is mutated.
func candidateAlreadySatisfied(plan *vxlan.ChangePlan, cfg vxlan.Record) bool {
	change := plan.Change()
	vlan := strconv.Itoa(number(change["vlan"]))
	iface := "Vlan" + vlan
	before := maps.Clone(cfg)
	interfaces := maps.Clone(record(cfg["interfaces"]))
	svi := maps.Clone(record(interfaces[iface]))
	if svi == nil {
		return false
	}
	switch text(change["kind"]) {
	case "svi_gateway_add":
		svi["addresses"] = []string{}
	case "vlan_vni_add":
		mapping := maps.Clone(record(cfg["vlan_vnis"]))
		if mapping == nil {
			return false
		}
		mapping[vlan] = nil
		before["vlan_vnis"] = mapping
	case "svi_acl":
		bindings := maps.Clone(record(svi["acl_bindings"]))
		delete(bindings, text(change["direction"]))
		svi["acl_bindings"] = bindings
	default:
		return false
	}
	interfaces[iface] = svi
	before["interfaces"] = interfaces
	return plan.Satisfied(cfg, before)
}
func candidateBlocker(change, cfg vxlan.Record) string {
	svi := record(record(cfg["interfaces"])["Vlan"+strconv.Itoa(number(change["vlan"]))])
	if svi == nil {
		return "prerequisites_missing"
	}
	switch text(change["kind"]) {
	case "svi_gateway_add":
		if text(svi["vrf"]) != text(change["vrf"]) {
			return "service_context_mismatch"
		}
		if len(texts(svi["addresses"])) > 0 {
			return "existing_gateway_conflict"
		}
	case "vlan_vni_add":
		if v := record(cfg["vlan_vnis"])[strconv.Itoa(number(change["vlan"]))]; v != nil && number(v) != number(change["vni"]) {
			return "vlan_vni_conflict"
		}
	case "svi_acl":
		if text(record(svi["acl_bindings"])[text(change["direction"])]) != "" {
			return "existing_binding_conflict"
		}
		if record(cfg["acls"])[text(change["acl"])] == nil {
			return "acl_definition_missing"
		}
		return "acl_policy_differs"
	}
	return "prerequisites_missing"
}

// Findings carry only observed affected IDs. The candidate endpoint rechecks
// every target after the operator chooses a source; a hint never authorizes writes.
func attachRemediations(out *dto.Detail) {
	if !out.CompleteScope {
		return
	}
	current := map[int]bool{}
	for _, n := range out.Nodes {
		current[n.DeviceID] = n.Role == "leaf_switch" && n.ConfigState == "current"
	}
	for _, segment := range out.Segments {
		attachSegmentRemediations(out, segment, current)
	}
}

func candidateTargets(r *dto.CandidatesReq, eligibleIDs map[int]bool) (map[int]bool, error) {
	requested := map[int]bool{}
	for _, target := range r.TargetDeviceIDs {
		if !eligibleIDs[target] || target == r.SourceDeviceID || requested[target] {
			return nil, InvalidInputErr
		}
		requested[target] = true
	}
	return requested, nil
}
func attachSegmentRemediations(out *dto.Detail, segment dto.Segment, current map[int]bool) {
	missingMapping, missingGateway := []int{}, []int{}
	gatewayObserved := false
	bound := map[string]bool{"in": false, "out": false}
	unbound := map[string][]int{"in": {}, "out": {}}
	for _, cell := range segment.Cells {
		if !current[cell.DeviceID] || cell.State == "unknown" || cell.State == "conflict" {
			continue
		}
		svi := record(cell.Configuration["svi"])
		if cell.State == "missing" {
			missingMapping = append(missingMapping, cell.DeviceID)
		}
		if slices.Contains(texts(cell.Configuration["missing"]), "VLAN") {
			missingMapping = append(missingMapping, cell.DeviceID)
		}
		if svi == nil {
			continue
		}
		if len(texts(svi["addresses"])) > 0 {
			gatewayObserved = true
		} else {
			missingGateway = append(missingGateway, cell.DeviceID)
		}
		for direction, binding := range map[string]string{"in": cell.ACLIn, "out": cell.ACLOut} {
			if binding != "" && binding != "unbound" {
				bound[direction] = true
			} else {
				unbound[direction] = append(unbound[direction], cell.DeviceID)
			}
		}
	}
	add := func(kind, direction, message string, ids []int) {
		if len(ids) == 0 {
			return
		}
		sort.Ints(ids)
		ids = slices.Compact(ids)
		remediation := &dto.Remediation{Kind: kind, VNI: segment.VNI, Direction: direction, TargetDeviceIDs: ids}
		check := map[string]string{"vlan_vni_add": "segment_presence", "svi_gateway_add": "segment_configuration", "svi_acl": "acl_binding"}[kind]
		out.Findings = append(out.Findings, dto.Finding{Scope: "business", VRF: segment.VRF, Check: check, Direction: direction, VNI: segment.VNI, Code: "repair_" + kind + "_" + direction, Severity: "warning", Message: message, Remediation: remediation, Evidence: map[string]any{"deviceIds": ids}})
	}
	add("vlan_vni_add", "", "部分 Leaf 缺少 VLAN/VNI 映射；选择标准设备后检查可补齐目标", missingMapping)
	if gatewayObserved {
		add("svi_gateway_add", "", "部分 Leaf 的业务 SVI 缺少网关地址", missingGateway)
	}
	for _, direction := range []string{"in", "out"} {
		if bound[direction] {
			add("svi_acl", direction, direction+" 方向已有保护策略，部分 Leaf 尚未绑定", unbound[direction])
		}
	}
}

func (s *Fabric) evaluateCandidates(ctx context.Context, id int, devices []networkmodels.Device, r *dto.CandidatesReq, eligibleIDs, requested map[int]bool, source vxlan.Record, plan *vxlan.ChangePlan, out *dto.Candidates) error {
	for _, d := range devices {
		if d.ID == r.SourceDeviceID || !eligibleIDs[d.ID] || (len(requested) > 0 && !requested[d.ID]) {
			continue
		}
		row := dto.Candidate{DeviceID: d.ID, Name: d.Name, IP: d.IP}
		_, cfg, e := s.changeConfig(ctx, id, d.ID)
		if e != nil {
			if !errors.Is(e, gorm.ErrRecordNotFound) && !errors.Is(e, ConflictErr) && !errors.Is(e, UnavailableErr) {
				return e
			}
			row.Reason = "evidence_unavailable"
			out.Blocked = append(out.Blocked, row)
			continue
		}
		if !aclServiceContextMatches(source, cfg, plan.Change()) {
			row.Reason = "service_context_mismatch"
			out.Blocked = append(out.Blocked, row)
			continue
		}
		if candidateAlreadySatisfied(plan, cfg) {
			row.Reason = "already_consistent"
			out.Unchanged = append(out.Unchanged, row)
			continue
		}
		if plan.Eligible(cfg) {
			row.Reason = "missing_increment"
			out.Ready = append(out.Ready, row)
			continue
		}
		row.Reason = candidateBlocker(plan.Change(), cfg)
		out.Blocked = append(out.Blocked, row)
	}
	return nil
}
