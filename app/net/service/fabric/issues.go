package fabric

import (
	"fmt"
	"sort"
	"strings"

	vxlan "github.com/gatework/go-vxlan"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
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
		return evaluateACL(s, n, direction, policy, names, policies, completePolicy)
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
func evaluateACL(s serviceEvidence, n dto.Node, direction string, policy bool, names, policies map[string]bool, completePolicy bool) dto.IssueDevice {
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
}
