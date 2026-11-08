package service

import (
	"fmt"
	"sort"
	"strings"

	vxlan "github.com/gatework/go-vxlan"
	"go-admin/app/fabric/service/dto"
)

const issueAssessmentVersion = "fabric-issues-v2"

type issueScope struct {
	check, vrf, direction string
	vni                   int
}
type issueCheck struct {
	scope           issueScope
	category, title string
	evaluate        func(dto.Node) dto.IssueDevice
}
type serviceEvidence struct {
	scope   issueScope
	rows    map[int][]vxlan.Record
	sameVNI map[int]bool
}

func issueOutcome(n dto.Node, status, reason, value, dimension string) dto.IssueDevice {
	d := dto.IssueDevice{DeviceID: n.DeviceID, Status: status, Reason: reason, Value: value, EvidenceIDs: []int{}, FindingIndices: []int{}}
	if e := n.Dimensions[dimension]; e.Latest != nil {
		d.EvidenceIDs = append(d.EvidenceIDs, e.Latest.ID)
		at := e.Latest.ObservedAt
		d.ObservedAt = &at
	}
	return d
}
func evidenceReason(e dto.Evidence) string {
	if e.Stale {
		return "evidence_stale"
	}
	if e.Latest == nil {
		return "evidence_missing"
	}
	if e.Latest.Status != "complete" {
		return "evidence_incomplete"
	}
	return "evidence_invalidated"
}
func currentIssueConfig(n dto.Node) vxlan.Record {
	if !n.Supported {
		return nil
	}
	return configRecord(n.Dimensions["configuration"])
}
func configOutcome(n dto.Node) dto.IssueDevice {
	if !n.Supported {
		return issueOutcome(n, "unknown", "platform_unsupported", "", "configuration")
	}
	e := n.Dimensions["configuration"]
	if currentIssueConfig(n) == nil {
		return issueOutcome(n, "unknown", evidenceReason(e), "", "configuration")
	}
	return issueOutcome(n, "healthy", "check_passed", "", "configuration")
}
func (s serviceEvidence) prerequisite(n dto.Node) (dto.IssueDevice, bool) {
	if n.Role == "spine_switch" || n.Role == "border_switch" {
		return issueOutcome(n, "not_applicable", "role_not_applicable", n.Role, ""), false
	}
	if n.Role != "leaf_switch" {
		return issueOutcome(n, "unknown", "role_unknown", n.Role, ""), false
	}
	d := configOutcome(n)
	if d.Status != "healthy" {
		return d, false
	}
	rows := s.rows[n.DeviceID]
	if len(rows) == 0 {
		if s.sameVNI[n.DeviceID] {
			return issueOutcome(n, "affected", "service_context_conflict", "", "configuration"), false
		}
		return issueOutcome(n, "affected", "service_missing", "", "configuration"), false
	}
	if len(rows) > 1 {
		return issueOutcome(n, "affected", "service_ambiguous", "", "configuration"), false
	}
	return d, true
}
func issueServices(nodes []dto.Node) []serviceEvidence {
	configs := vxlan.Record{}
	byIP := map[string]int{}
	for _, n := range nodes {
		if n.Role == "leaf_switch" {
			byIP[n.IP] = n.DeviceID
			c := currentIssueConfig(n)
			if c == nil {
				c = historicalConfig(n.Dimensions["configuration"])
			}
			if c != nil {
				configs[n.IP] = c
			}
		}
	}
	catalog := vxlan.ServiceCatalog(configs)
	out := []serviceEvidence{}
	for vni, rows := range catalog {
		contexts := map[string]bool{}
		same := map[int]bool{}
		for _, r := range rows {
			contexts[text(record(r["svi"])["vrf"])] = true
			same[byIP[text(r["host"])]] = true
		}
		for vrf := range contexts {
			business := serviceEvidence{scope: issueScope{vni: vni, vrf: vrf}, rows: map[int][]vxlan.Record{}, sameVNI: same}
			for _, r := range rows {
				if text(record(r["svi"])["vrf"]) == vrf {
					id := byIP[text(r["host"])]
					business.rows[id] = append(business.rows[id], r)
				}
			}
			out = append(out, business)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].scope.vni != out[j].scope.vni {
			return out[i].scope.vni < out[j].scope.vni
		}
		return out[i].scope.vrf < out[j].scope.vrf
	})
	return out
}
func scopeCheck(s serviceEvidence, check, category, title string, evaluate func(dto.Node) dto.IssueDevice) issueCheck {
	scope := s.scope
	scope.check = check
	return issueCheck{scope: scope, category: category, title: title, evaluate: evaluate}
}
func servicePresenceCheck(s serviceEvidence) issueCheck {
	return scopeCheck(s, "segment_presence", "service", "业务承载完整性", func(n dto.Node) dto.IssueDevice {
		d, ok := s.prerequisite(n)
		if !ok {
			return d
		}
		d.Value = serviceObservation(s.rows[n.DeviceID][0], "")
		missing := texts(s.rows[n.DeviceID][0]["missing"])
		if len(missing) > 0 {
			d.Status = "affected"
			d.Reason = "service_incomplete"
			d.Value = strings.Join(missing, ", ")
		}
		return d
	})
}
func serviceSignature(n dto.Node, r vxlan.Record) vxlan.Record {
	c := currentIssueConfig(n)
	svi := record(r["svi"])
	// ACL checks own their direction and policy, so a binding difference does not
	// turn an otherwise identical gateway into a separate configuration variant.
	sviCopy := vxlan.Record{}
	for k, v := range svi {
		if k != "acl_bindings" {
			sviCopy[k] = v
		}
	}
	return vxlan.Record{"vlan": r["vlan"], "svi": sviCopy, "nve": vxlan.EffectiveNVEMember(c, record(r["nve"])), "tenant": vxlan.ServiceTenant(c, text(svi["vrf"])), "evpn": r["evpn"], "anycast_mac": c["anycast_mac"]}
}
func serviceConfigurationCheck(s serviceEvidence, nodes []dto.Node) issueCheck {
	signatures := map[int]string{}
	variants := map[string]bool{}
	complete := true
	for _, n := range nodes {
		d, ok := s.prerequisite(n)
		if d.Status == "not_applicable" {
			continue
		}
		if !ok {
			complete = false
			continue
		}
		if len(texts(s.rows[n.DeviceID][0]["missing"])) > 0 {
			complete = false
		}
		signature := vxlan.Fingerprint(serviceSignature(n, s.rows[n.DeviceID][0]))
		signatures[n.DeviceID] = signature
		variants[signature] = true
	}
	return scopeCheck(s, "segment_configuration", "service", "业务配置一致性", func(n dto.Node) dto.IssueDevice {
		d, ok := s.prerequisite(n)
		if !ok {
			if d.Status == "affected" && d.Reason == "service_missing" {
				d.Status = "unknown"
			}
			return d
		}
		if signatures[n.DeviceID] == "" {
			d.Status = "unknown"
			d.Reason = "service_incomplete"
			return d
		}
		d.Value = serviceObservation(s.rows[n.DeviceID][0], signatures[n.DeviceID])
		if len(variants) > 1 {
			d.Status = "affected"
			d.Reason = "configuration_variant"
		} else if !complete {
			d.Status = "unknown"
			d.Reason = "comparison_incomplete"
		}
		return d
	})
}
func aclCheck(s serviceEvidence, nodes []dto.Node, direction string, policy bool) issueCheck {
	names, policies := map[string]bool{}, map[string]bool{}
	completePolicy := true
	for _, n := range nodes {
		if outcome, ok := s.prerequisite(n); !ok {
			if outcome.Status != "not_applicable" {
				completePolicy = false
			}
			continue
		}
		if record(s.rows[n.DeviceID][0]["svi"]) == nil {
			continue
		}
		name := text(record(record(s.rows[n.DeviceID][0]["svi"])["acl_bindings"])[direction])
		if name == "" {
			completePolicy = false
			continue
		}
		names[name] = true
		c := currentIssueConfig(n)
		if p := vxlan.ResolveACLPolicy(c, record(c["acls"])[name]); p != nil {
			policies[vxlan.Fingerprint(p)] = true
		} else {
			completePolicy = false
		}
	}
	check, title := "acl_binding", "ACL 挂载一致性"
	if policy {
		check, title = "acl_policy", "ACL 有序规则与依赖一致性"
	}
	c := scopeCheck(s, check, "acl", title, func(n dto.Node) dto.IssueDevice {
		d, ok := s.prerequisite(n)
		if !ok {
			if d.Status == "affected" && d.Reason == "service_missing" {
				d.Status = "unknown"
			}
			return d
		}
		svi := record(s.rows[n.DeviceID][0]["svi"])
		if svi == nil {
			d.Status = "not_applicable"
			d.Reason = "svi_not_configured"
			return d
		}
		name := text(record(svi["acl_bindings"])[direction])
		d.Value = name
		if name == "" {
			d.Status = "unknown"
			d.Reason = "acl_intent_unconfirmed"
			if !policy && len(names) > 0 {
				d.Status = "affected"
				d.Reason = "acl_unbound"
			}
			return d
		}
		if !policy {
			if len(names) > 1 {
				d.Status = "affected"
				d.Reason = "acl_binding_variant"
			}
			return d
		}
		cfg := currentIssueConfig(n)
		entries := record(cfg["acls"])[name]
		if entries == nil {
			d.Status = "affected"
			d.Reason = "acl_definition_missing"
			return d
		}
		resolved := vxlan.ResolveACLPolicy(cfg, entries)
		if resolved == nil {
			d.Status = "unknown"
			d.Reason = "acl_dependency_unresolved"
			return d
		}
		d.Value = name + " · " + vxlan.Fingerprint(resolved)[:8]
		if len(policies) > 1 {
			d.Status = "affected"
			d.Reason = "acl_policy_variant"
		} else if !completePolicy {
			d.Status = "unknown"
			d.Reason = "comparison_incomplete"
		}
		return d
	})
	c.scope.direction = direction
	return c
}
func runtimeIssueCheck(name string) issueCheck {
	return issueCheck{scope: issueScope{check: "runtime_" + name}, category: "runtime", title: name + " 运行状态", evaluate: func(n dto.Node) dto.IssueDevice {
		d := configOutcome(n)
		if d.Status != "healthy" {
			return d
		}
		cfg := currentIssueConfig(n)
		if !applicable(name, cfg) {
			return issueOutcome(n, "not_applicable", "protocol_not_configured", "", "configuration")
		}
		e := n.Dimensions[name]
		if e.State != "complete" || e.Latest == nil {
			return issueOutcome(n, "unknown", evidenceReason(e), "", name)
		}
		rows := e.Latest.Data.Records
		if name == "interfaces" || name == "port_channels" {
			rows = fabricInterfaceRows(cfg, rows)
		}
		bad, known := healthRows(name, rows)
		d = issueOutcome(n, "healthy", "check_passed", "", name)
		if bad || missingExpectedRuntime(name, cfg, rows) {
			d.Status = "affected"
			d.Reason = "runtime_fault"
		} else if !known || missingFabricInterfaceEvidence(name, cfg, rows) || len(rows) == 0 && (name == "nve_peers" || name == "ospf_neighbors") {
			d.Status = "unknown"
			d.Reason = "runtime_empty"
		}
		d.Value = runtimeObservation(rows)
		return d
	}}
}
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
	if strings.HasPrefix(s.check, "runtime_") {
		name := strings.TrimPrefix(s.check, "runtime_")
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
	var walk func(any)
	walk = func(value any) {
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
				walk(r)
			}
		case []any:
			for _, r := range x {
				walk(r)
			}
		case map[string]any:
			for _, n := range nodes {
				if text(x["host"]) == n.IP || number(x["deviceId"]) == n.DeviceID {
					ids[n.DeviceID] = true
				}
			}
			for _, v := range x {
				walk(v)
			}
		}
	}
	walk(f.Evidence)
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
			groups = append(groups, g)
		}
		if roles[pos] == nil {
			roles[pos] = map[string]bool{}
		}
		roles[pos][f.Role] = true
		g := &groups[pos]
		g.FindingIndices = append(g.FindingIndices, index)
		involved := findingDevices(f, detail.Nodes)
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
	for i := range groups {
		if !roles[i][""] {
			for j := range groups[i].Devices {
				d := &groups[i].Devices[j]
				for _, n := range detail.Nodes {
					if n.DeviceID == d.DeviceID && !roles[i][reviewRole(n.Role)] {
						d.Status = "not_applicable"
						d.Reason = "role_not_applicable"
					}
				}
			}
		}
		finishIssueGroup(&groups[i])
	}
	return groups
}

// Values are compact investigation cues; raw evidence stays in observations and findings.
func serviceObservation(row vxlan.Record, signature string) string {
	svi := record(row["svi"])
	value := "VLAN " + text(row["vlan"])
	addresses := texts(svi["addresses"])
	if len(addresses) > 0 {
		value += " · " + strings.Join(addresses, ", ")
	}
	if len(signature) >= 8 {
		value += " · " + signature[:8]
	}
	return value
}
func runtimeObservation(rows []vxlan.Record) string {
	states := map[string]bool{}
	for _, r := range rows {
		for _, key := range []string{"state", "peer_status", "flags"} {
			if value := text(r[key]); value != "" {
				states[value] = true
			}
		}
	}
	ordered := []string{}
	for state := range states {
		ordered = append(ordered, state)
	}
	sort.Strings(ordered)
	if len(ordered) > 3 {
		ordered = ordered[:3]
	}
	return fmt.Sprintf("%d 条 · %s", len(rows), strings.Join(ordered, ", "))
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
			for _, missing := range texts(matches[0]["missing"]) {
				if missing == "VLAN" {
					return true
				}
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
		f := &detail.Findings[i]
		if f.Scope == "fabric" || f.VNI == 0 {
			continue
		}
		if f.Scope != "business" {
			if f.Role != "" && f.Role != "leaves" && f.VRF == "" {
				f.Scope = "fabric"
				continue
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
				continue
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
		if f.Check != "" {
			continue
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
			continue
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
}
