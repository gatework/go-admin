package fabric

import (
	"slices"
	"sort"
	"strings"

	vxlan "github.com/gatework/go-vxlan"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
)

func buildIssueGroups(detail dto.Detail) []dto.IssueGroup {
	detail.Findings = append([]dto.Finding{}, detail.Findings...)
	scopeFindings(&detail)
	checks := []issueCheck{{scope: issueScope{check: "configuration_evidence"}, category: "evidence", title: "当前配置证据", evaluate: configOutcome}}
	for _, name := range []string{"nve_peers", "nve_vnis", "bgp_neighbors", "ospf_neighbors", "vpc", "interfaces", "port_channels"} {
		checks = append(checks, runtimeIssueCheck(name))
	}
	for _, business := range issueServices(detail.Nodes) {
		checks = append(checks, servicePresenceCheck(business), serviceConfigurationCheck(business, detail.Nodes))
		for _, direction := range []string{"in", "out"} {
			checks = append(checks, aclCheck(business, detail.Nodes, direction, false), aclCheck(business, detail.Nodes, direction, true))
		}
	}
	groups := []dto.IssueGroup{}
	consumed := map[int]bool{}
	for _, check := range checks {
		g := evaluateIssueGroup(check, detail)
		if g.Counts.Affected+g.Counts.Unknown > 0 {
			groups = append(groups, g)
			for _, index := range g.FindingIndices {
				consumed[index] = true
			}
		}
	}
	groups = append(groups, fallbackIssueGroups(detail, consumed)...)
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if a.Severity != b.Severity {
			return a.Severity == "fault"
		}
		if a.Counts.Affected != b.Counts.Affected {
			return a.Counts.Affected > b.Counts.Affected
		}
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		if a.VNI != b.VNI {
			return a.VNI < b.VNI
		}
		if a.VRF != b.VRF {
			return a.VRF < b.VRF
		}
		if a.Check != b.Check {
			return a.Check < b.Check
		}
		return a.Direction < b.Direction
	})
	return groups
}
func finishIssueGroup(g *dto.IssueGroup) {
	sort.Slice(g.Devices, func(i, j int) bool { return g.Devices[i].DeviceID < g.Devices[j].DeviceID })
	for _, d := range g.Devices {
		switch d.Status {
		case "affected":
			g.Counts.Affected++
		case "healthy":
			g.Counts.Healthy++
		case "not_applicable":
			g.Counts.NotApplicable++
		default:
			g.Counts.Unknown++
		}
	}
	if g.Counts.Affected > 0 {
		g.Severity = "fault"
	}
}
func findingMatchesIssue(f dto.Finding, s issueScope) bool {
	if s.check == "configuration_evidence" {
		return f.Code == "evidence_configuration" || f.Code == "fabric_evidence_incomplete"
	}
	if after, ok := strings.CutPrefix(s.check, "runtime_"); ok {
		name := after
		return f.Code == "evidence_"+name || f.Code == "health_"+name || f.Code == "health_"+name+"_empty"
	}
	if f.Role != "" && f.Role != "leaves" {
		return false
	}
	return f.Scope == "business" && f.VNI == s.vni && s.vni != 0 && f.VRF == s.vrf && f.Check == s.check && f.Direction == s.direction
}
func attachIssueFinding(g *dto.IssueGroup, f dto.Finding, index int, nodes []dto.Node) {
	involved := findingDevices(f, nodes)
	for i := range g.Devices {
		d := &g.Devices[i]
		if involved[d.DeviceID] || f.DeviceID == 0 && len(involved) == 0 {
			d.FindingIndices = append(d.FindingIndices, index)
		}
	}
	if f.Remediation == nil || !g.CompleteScope {
		return
	}
	r := *f.Remediation
	r.TargetDeviceIDs = []int{}
	for _, id := range f.Remediation.TargetDeviceIDs {
		for _, d := range g.Devices {
			if d.DeviceID == id && d.Status == "affected" && d.Reason != "service_context_conflict" && remediationTargetMatches(r, *g, id, nodes) {
				r.TargetDeviceIDs = append(r.TargetDeviceIDs, id)
			}
		}
	}
	sort.Ints(r.TargetDeviceIDs)
	r.TargetDeviceIDs = uniqueIssueIDs(r.TargetDeviceIDs)
	if len(r.TargetDeviceIDs) > 0 {
		for _, existing := range g.Remediations {
			if vxlan.Fingerprint(existing) == vxlan.Fingerprint(r) {
				return
			}
		}
		g.Remediations = append(g.Remediations, r)
	}
}
func uniqueIssueIDs(ids []int) []int {
	out := []int{}
	for _, id := range ids {
		if len(out) == 0 || out[len(out)-1] != id {
			out = append(out, id)
		}
	}
	return out
}
func findingDevices(f dto.Finding, nodes []dto.Node) map[int]bool {
	ids := map[int]bool{}
	if f.DeviceID != 0 {
		ids[f.DeviceID] = true
	}
	if f.Remediation != nil {
		for _, id := range f.Remediation.TargetDeviceIDs {
			ids[id] = true
		}
	}

	collectFindingDevices(f.Evidence, nodes, ids)
	return ids
}
func fallbackIssueGroups(detail dto.Detail, consumed map[int]bool) []dto.IssueGroup {
	// Unmodeled findings remain visible with conservative peer outcomes. Absence
	// of the same finding on another node is not a positive evaluation.
	groups := []dto.IssueGroup{}
	positions := map[string]int{}
	roles := map[int]map[string]bool{}
	for index, f := range detail.Findings {
		if consumed[index] {
			continue
		}
		scope := issueScope{check: f.Code, vni: f.VNI, vrf: f.VRF, direction: f.Direction}
		if strings.HasPrefix(f.Message, "in 方向") {
			scope.direction = "in"
		} else if strings.HasPrefix(f.Message, "out 方向") {
			scope.direction = "out"
		}
		groupScope := f.Scope
		if groupScope == "" {
			groupScope = "fabric"
		}
		key := vxlan.Fingerprint([]any{groupScope, scope.check, scope.vrf, scope.vni, scope.direction})
		pos, exists := positions[key]
		if !exists {
			pos = len(groups)
			positions[key] = pos
			g := newFallbackGroup(f, detail, groupScope, key, scope)
			groups = append(groups, g)
		}
		if roles[pos] == nil {
			roles[pos] = map[string]bool{}
		}
		roles[pos][f.Role] = true
		g := &groups[pos]
		g.FindingIndices = append(g.FindingIndices, index)
		applyFallbackFinding(g, f, index, detail.Nodes)
	}
	for i := range groups {
		applyFallbackRoles(&groups[i], roles[i], detail.Nodes)
		finishIssueGroup(&groups[i])
	}
	return groups
}

func remediationTargetMatches(r dto.Remediation, g dto.IssueGroup, id int, nodes []dto.Node) bool {
	for _, n := range nodes {
		if n.DeviceID != id {
			continue
		}
		cfg := currentIssueConfig(n)
		if cfg == nil {
			return false
		}
		rows := vxlan.ServiceCatalog(vxlan.Record{n.IP: cfg})[g.VNI]
		if r.Kind == "vlan_vni_add" && len(rows) == 0 {
			return true
		}
		matches := []vxlan.Record{}
		for _, row := range rows {
			if text(record(row["svi"])["vrf"]) == g.VRF {
				matches = append(matches, row)
			}
		}
		if len(matches) != 1 {
			return false
		}
		svi := record(matches[0]["svi"])
		switch r.Kind {
		case "vlan_vni_add":
			if slices.Contains(texts(matches[0]["missing"]), "VLAN") {
				return true
			}
		case "svi_gateway_add":
			return svi != nil && len(texts(svi["addresses"])) == 0
		case "svi_acl":
			return svi != nil && r.Direction == g.Direction && text(record(svi["acl_bindings"])[r.Direction]) == ""
		}
	}
	return false
}

func reconcileIssueHealth(detail *dto.Detail) {
	statuses := map[int]string{}
	for _, g := range detail.IssueGroups {
		for _, d := range g.Devices {
			if d.Status == "affected" {
				statuses[d.DeviceID] = "fault"
				detail.State = "fault"
			} else if d.Status == "unknown" && statuses[d.DeviceID] != "fault" {
				statuses[d.DeviceID] = "unknown"
				if detail.State == "healthy" {
					detail.State = "unknown"
				}
			}
		}
	}
	for i := range detail.Nodes {
		n := &detail.Nodes[i]
		if n.State != "fault" {
			if state := statuses[n.DeviceID]; state != "" {
				n.State = state
			}
		}
	}
	detail.VPCGroups = VPCGroups(detail.Nodes)
}

func reviewRole(role string) string {
	switch role {
	case "leaf_switch":
		return "leaves"
	case "border_switch":
		return "borders"
	case "spine_switch":
		return "spines"
	}
	return "unknown"
}

// Resolve attribution before matching checks. A numeric VNI shared by multiple
// contexts does not identify a tenant; such evidence stays Fabric-wide.
func scopeFindings(detail *dto.Detail) {
	contexts := map[int]map[string]bool{}
	for _, service := range issueServices(detail.Nodes) {
		if contexts[service.scope.vni] == nil {
			contexts[service.scope.vni] = map[string]bool{}
		}
		contexts[service.scope.vni][service.scope.vrf] = true
	}
	for i := range detail.Findings {
		scopeFinding(&detail.Findings[i], contexts)
	}
}

func evaluateIssueGroup(check issueCheck, detail dto.Detail) dto.IssueGroup {
	groupScope := "fabric"
	if check.scope.vni != 0 {
		groupScope = "business"
	}
	g := dto.IssueGroup{Scope: groupScope, Key: vxlan.Fingerprint([]any{check.scope.check, check.scope.vrf, check.scope.vni, check.scope.direction}), Check: check.scope.check, VRF: check.scope.vrf, VNI: check.scope.vni, Direction: check.scope.direction, Category: check.category, Title: check.title, CompleteScope: detail.CompleteScope, Severity: "unknown", Devices: []dto.IssueDevice{}, FindingIndices: []int{}, Remediations: []dto.Remediation{}}
	seen := map[int]bool{}
	for _, n := range detail.Nodes {
		if !seen[n.DeviceID] {
			seen[n.DeviceID] = true
			g.Devices = append(g.Devices, check.evaluate(n))
		}
	}
	for i, f := range detail.Findings {
		if findingMatchesIssue(f, check.scope) {
			g.FindingIndices = append(g.FindingIndices, i)
			attachIssueFinding(&g, f, i, detail.Nodes)
		}
	}
	finishIssueGroup(&g)
	return g
}

func collectFindingDevices(value any, nodes []dto.Node, ids map[int]bool) {

	switch x := value.(type) {
	case []string:
		for _, host := range x {
			for _, n := range nodes {
				if n.IP == host {
					ids[n.DeviceID] = true
				}
			}
		}
	case []vxlan.Record:
		for _, r := range x {
			collectFindingDevices(r, nodes, ids)
		}
	case []any:
		for _, r := range x {
			collectFindingDevices(r, nodes, ids)
		}
	case map[string]any:
		for _, n := range nodes {
			if text(x["host"]) == n.IP || number(x["deviceId"]) == n.DeviceID {
				ids[n.DeviceID] = true
			}
		}
		for _, v := range x {
			collectFindingDevices(v, nodes, ids)
		}
	}
}

func newFallbackGroup(f dto.Finding, detail dto.Detail, groupScope, key string, scope issueScope) dto.IssueGroup {
	g := dto.IssueGroup{Scope: groupScope, Key: key, Check: scope.check, VRF: scope.vrf, VNI: scope.vni, Direction: scope.direction, Category: "configuration", Title: f.Message, CompleteScope: detail.CompleteScope, Severity: "unknown", Devices: []dto.IssueDevice{}, FindingIndices: []int{}, Remediations: []dto.Remediation{}}
	if strings.Contains(f.Code, "scope") || f.Code == "role_unknown" || f.Code == "platform_unsupported" {
		g.Category = "inventory"
	}
	seen := map[int]bool{}
	for _, n := range detail.Nodes {
		if !seen[n.DeviceID] {
			seen[n.DeviceID] = true
			g.Devices = append(g.Devices, issueOutcome(n, "unknown", "check_unavailable", "", "configuration"))
		}
	}
	return g
}

func applyFallbackFinding(g *dto.IssueGroup, f dto.Finding, index int, nodes []dto.Node) {
	involved := findingDevices(f, nodes)
	for i := range g.Devices {
		d := &g.Devices[i]
		if involved[d.DeviceID] || len(involved) == 0 && f.DeviceID == 0 {
			d.FindingIndices = append(d.FindingIndices, index)
			d.Reason = "finding_observed"
			if f.Severity == "fault" && involved[d.DeviceID] {
				d.Status = "affected"
			}
		}
	}
}

func applyFallbackRoles(g *dto.IssueGroup, roles map[string]bool, nodes []dto.Node) {
	if !roles[""] {
		for j := range g.Devices {
			d := &g.Devices[j]
			for _, n := range nodes {
				if n.DeviceID == d.DeviceID && !roles[reviewRole(n.Role)] {
					d.Status = "not_applicable"
					d.Reason = "role_not_applicable"
				}
			}
		}
	}
}

func scopeFinding(f *dto.Finding, contexts map[int]map[string]bool) {
	if f.Scope == "fabric" || f.VNI == 0 {
		return
	}
	if f.Scope != "business" {
		if f.Role != "" && f.Role != "leaves" && f.VRF == "" {
			f.Scope = "fabric"
			return
		}
		if f.VRF != "" {
			f.Scope = "business"
		} else if len(contexts[f.VNI]) == 1 {
			for vrf := range contexts[f.VNI] {
				f.VRF = vrf
			}
			f.Scope = "business"
		} else {
			f.Scope = "fabric"
			return
		}
	}
	if f.Direction == "" {
		if f.Remediation != nil {
			f.Direction = f.Remediation.Direction
		}
		if strings.HasPrefix(f.Message, "in 方向") {
			f.Direction = "in"
		}
		if strings.HasPrefix(f.Message, "out 方向") {
			f.Direction = "out"
		}
	}
	classifyFinding(f)
}

func classifyFinding(f *dto.Finding) {
	if f.Check != "" {
		return
	}
	if f.Remediation != nil {
		switch f.Remediation.Kind {
		case "vlan_vni_add":
			f.Check = "segment_presence"
		case "svi_gateway_add":
			f.Check = "segment_configuration"
		case "svi_acl":
			f.Check = "acl_binding"
		}
		return
	}
	switch f.Code {
	case "service_matrix_different", "service_matrix_unknown", "fabric_vlan_mapping", "fabric_replication", "fabric_arp_suppression", "fabric_route_targets", "fabric_tenant_routing", "fabric_vrf", "fabric_anycast", "fabric_mtu", "fabric_shutdown", "fabric_anycast_mac", "fabric_gateway":
		f.Check = "segment_configuration"
	case "fabric_service_incomplete", "fabric_vlan_mapping_ambiguous":
		f.Check = "segment_presence"
	case "fabric_acl_binding", "acl_in_unbound", "acl_out_unbound":
		f.Check = "acl_binding"
	case "fabric_acl_content", "fabric_acl_undefined", "fabric_acl_dependency":
		f.Check = "acl_policy"
	}
	if f.Code == "acl_in_unbound" {
		f.Direction = "in"
	}
	if f.Code == "acl_out_unbound" {
		f.Direction = "out"
	}
}
