package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	vxlan "github.com/gatework/go-vxlan"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"go-admin/app/fabric/models"
	"go-admin/app/fabric/service/dto"
	"gorm.io/gorm/clause"
)

func record(v any) vxlan.Record { m, _ := v.(map[string]any); return m }
func records(v any) []vxlan.Record {
	if rows, ok := v.([]vxlan.Record); ok {
		return rows
	}
	out := []vxlan.Record{}
	if rows, ok := v.([]any); ok {
		for _, r := range rows {
			if m := record(r); m != nil {
				out = append(out, m)
			}
		}
	}
	return out
}
func text(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
func number(v any) int { n, _ := strconv.Atoi(text(v)); return n }
func texts(v any) []string {
	if xs, ok := v.([]string); ok {
		return xs
	}
	out := []string{}
	if xs, ok := v.([]any); ok {
		for _, x := range xs {
			out = append(out, text(x))
		}
	}
	return out
}

// DimensionEvidence selects by acquisition time, protecting latest from late arrivals.
// A newer failure never erases the historical last complete observation.
func DimensionEvidence(rows []models.Observation, name string, now time.Time, maxAge time.Duration) dto.Evidence {
	out := dto.Evidence{State: "unknown"}
	for i := range rows {
		r := &rows[i]
		if r.Dimension != name {
			continue
		}
		if out.Latest == nil || r.ObservedAt.After(out.Latest.ObservedAt) || r.ObservedAt.Equal(out.Latest.ObservedAt) && r.ID > out.Latest.ID {
			out.Latest = r
		}
		if r.Status == "complete" && (out.LastComplete == nil || r.ObservedAt.After(out.LastComplete.ObservedAt) || r.ObservedAt.Equal(out.LastComplete.ObservedAt) && r.ID > out.LastComplete.ID) {
			out.LastComplete = r
		}
	}
	if out.Latest != nil {
		out.Stale = out.Latest.ObservedAt.IsZero() || out.Latest.ObservedAt.Before(now.Add(-maxAge)) || out.Latest.ObservedAt.After(now.Add(time.Minute))
		if out.Latest.Status == "complete" && !out.Stale {
			out.State = "complete"
		}
	}
	return out
}
func VPCGroups(nodes []dto.Node) []dto.VPCGroup {
	nodes = append([]dto.Node{}, nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].DeviceID < nodes[j].DeviceID })
	out := []dto.VPCGroup{}
	for i, a := range nodes {
		for _, b := range nodes[i+1:] {
			aSource, bSource := a.VPCSource, b.VPCSource
			if aSource == "" {
				aSource = a.IP
			}
			if bSource == "" {
				bSource = b.IP
			}
			if a.VPCDomain == "" || a.VPCDomain != b.VPCDomain || a.VPCPeer == "" || a.VPCPeer != bSource || b.VPCPeer != aSource {
				continue
			}
			state := "unknown"
			if a.State == "fault" || b.State == "fault" {
				state = "fault"
			} else if a.State == "healthy" && b.State == "healthy" {
				state = "healthy"
			}
			out = append(out, dto.VPCGroup{Key: fmt.Sprintf("%d:%d", a.DeviceID, b.DeviceID), Domain: a.VPCDomain, DeviceIDs: []int{a.DeviceID, b.DeviceID}, State: state})
		}
	}
	return out
}
func configRecord(e dto.Evidence) vxlan.Record {
	if e.State != "complete" || e.Latest == nil || len(e.Latest.Data.Records) != 1 || e.Latest.Data.Records[0]["review_status"] != "complete" {
		return nil
	}
	return e.Latest.Data.Records[0]
}
func applicable(name string, config vxlan.Record) bool {
	if config == nil {
		return true
	}
	switch name {
	case "vpc":
		return record(config["vpc_domain"]) != nil
	case "bgp_neighbors":
		for _, n := range vxlan.BGPNeighbors(record(config["bgp"])) {
			if record(record(n["settings"])["address_families"])["l2vpn evpn"] != nil {
				return true
			}
		}
		return false
	case "ospf_neighbors":
		return len(record(config["ospf"])) > 0
	case "nve_peers", "nve_vnis":
		return len(record(record(config["nve"])["members"])) > 0
	case "interfaces", "port_channels":
		return len(expectedFabricInterfaces(name, config)) > 0
	default:
		return true
	}
}
func healthRows(name string, rows []vxlan.Record) (bool, bool) {
	fault, known := false, true
	for _, r := range rows {
		state := strings.ToLower(text(r["state"]))
		switch name {
		case "bgp_neighbors":
			if state != "established" {
				fault = true
			}
		case "ospf_neighbors":
			if !strings.HasPrefix(state, "full") {
				fault = true
			}
		case "nve_peers", "nve_vnis":
			if state != "up" {
				fault = true
			}
		case "vpc":
			if !strings.Contains(strings.ToLower(text(r["peer_status"])), "adjacency formed") || strings.ToLower(strings.TrimSpace(text(r["keepalive_status"]))) != "peer is alive" || strings.ToLower(text(r["consistency_status"])) != "success" {
				fault = true
			}
			for _, link := range records(r["peer_links"]) {
				if strings.ToLower(text(link["state"])) != "up" {
					fault = true
				}
			}
			for _, member := range records(r["members"]) {
				if strings.ToLower(text(member["state"])) != "up" || strings.ToLower(text(member["consistency"])) != "success" {
					fault = true
				}
			}
		case "interfaces":
			switch state {
			case "up", "connected":
			case "down", "notconnect", "disabled", "admin-down":
				fault = true
			default:
				known = false
			}
		case "port_channels":
			flags := text(r["flags"])
			if !strings.Contains(flags, "U") {
				fault = true
			}
		default:
			known = false
		}
	}
	return fault, known
}

func (s *Fabric) Detail(ctx context.Context, id int, p *actions.DataPermission) (dto.Detail, error) {
	out := dto.Detail{Nodes: []dto.Node{}, Segments: []dto.Segment{}, VPCGroups: []dto.VPCGroup{}, Findings: []dto.Finding{}}
	devices, full, err := s.VisibleMembers(ctx, id, p)
	if err != nil {
		return out, err
	}
	var f models.Fabric
	if err := s.Orm.WithContext(ctx).First(&f, id).Error; err != nil {
		return out, err
	}
	out.Summary = dto.Summary{Fabric: f, CompleteScope: full, MemberCount: len(devices), State: "unknown"}
	now := time.Now().UTC()
	ids := []int{}
	for _, d := range devices {
		ids = append(ids, d.Id)
	}
	observations, err := s.evidenceRows(ctx, id, ids)
	if err != nil {
		return out, err
	}
	configs := vxlan.Record{}
	observedConfigs := vxlan.Record{}
	documents := []vxlan.Record{}
	leaves, borders, spines := []string{}, []string{}, []string{}
	versions := []any{issueAssessmentVersion, f.ID, f.Version, f.RulesVersion}
	allHealthy := len(devices) > 0 && full
	fault := false
	for _, d := range devices {
		var m models.Membership
		if err := s.Orm.WithContext(ctx).First(&m, "device_id = ?", d.Id).Error; err != nil {
			return out, err
		}
		connection, connectionErr := s.deviceConnection(ctx, d)
		currentSourceHash := sourceHash(opsDevice(d), connection)
		n := dto.Node{DeviceID: d.Id, Name: d.Name, IP: d.IP, Role: d.DeviceRole, Version: d.Version, MembershipVersion: m.Version, State: "unknown", Model: d.HardwareModel, Platform: d.OS, Supported: connectionErr == nil && connection.Profile == "cisco_nxos", Dimensions: map[string]dto.Evidence{}, Features: []string{}}
		local := []models.Observation{}
		for _, o := range observations {
			if o.DeviceID == d.Id {
				local = append(local, o)
			}
		}
		for _, name := range vxlan.DimensionNames() {
			age := 2 * time.Duration(f.RuntimeMinutes) * time.Minute
			if name == "configuration" {
				age = 2 * time.Duration(f.ConfigMinutes) * time.Minute
			}
			if age <= 0 {
				age = 10 * time.Minute
			}
			e := DimensionEvidence(local, name, now, age)
			if e.Latest != nil {
				versions = append(versions, []any{d.Id, name, e.Latest.ID, e.State, e.Stale})
				if out.ObservedAt == nil || e.Latest.ObservedAt.After(*out.ObservedAt) {
					at := e.Latest.ObservedAt
					out.ObservedAt = &at
				}
				if connectionErr != nil || e.Latest.SourceHash != currentSourceHash || e.Latest.MembershipVersion != m.Version || e.Latest.RuleVersion != f.RulesVersion {
					e.State = "unknown"
				}
			}
			n.Dimensions[name] = e
		}
		config := configRecord(n.Dimensions["configuration"])
		displayConfig := config
		n.ConfigState = "current"
		if config == nil {
			displayConfig = historicalConfig(n.Dimensions["configuration"])
			n.ConfigState = "historical"
			if displayConfig == nil {
				n.ConfigState = "unknown"
			}
		}
		if displayConfig != nil {
			observedConfigs[d.IP] = displayConfig
		}
		n.Features = texts(displayConfig["features"])
		vpc := record(displayConfig["vpc_domain"])
		n.VPCDomain = text(vpc["id"])
		keepalive := record(vpc["peer_keepalive"])
		n.VPCSource = text(keepalive["source"])
		n.VPCPeer = text(keepalive["destination"])
		n.VTEP = text(record(displayConfig["nve"])["source_interface"])
		nodeHealthy := config != nil && n.Supported
		nodeFault := false
		if config != nil {
			for _, finding := range vxlan.ConfigReview(config, true) {
				nodeFault = true
				out.Findings = append(out.Findings, dto.Finding{DeviceID: d.Id, VRF: text(finding["vrf"]), VNI: number(finding["vni"]), Code: text(finding["code"]), Severity: "fault", Message: "配置完整性检查发现不一致", Evidence: finding})
			}
		}
		for _, name := range []string{"configuration", "nve_peers", "nve_vnis", "bgp_neighbors", "ospf_neighbors", "vpc", "interfaces", "port_channels"} {
			if !applicable(name, config) {
				continue
			}
			out.ApplicableCount++
			e := n.Dimensions[name]
			if e.State != "complete" {
				nodeHealthy = false
				out.UnknownCount++
				out.Findings = append(out.Findings, dto.Finding{DeviceID: d.Id, Code: "evidence_" + name, Severity: "unknown", Message: name + " 证据缺失、失败、过期或归属版本已变化", Evidence: e})
				continue
			}
			out.CheckedCount++
			healthRecords := e.Latest.Data.Records
			if name == "interfaces" {
				healthRecords = fabricInterfaceRows(config, healthRecords)
			}
			if name == "port_channels" {
				healthRecords = fabricInterfaceRows(config, healthRecords)
			}
			if len(healthRecords) == 0 && (name == "nve_peers" || name == "ospf_neighbors") {
				nodeHealthy = false
				out.UnknownCount++
				out.Findings = append(out.Findings, dto.Finding{DeviceID: d.Id, Code: "health_" + name + "_empty", Severity: "unknown", Message: name + " 未观察到邻居，尚不能证明协议健康", Evidence: e.Latest})
			}
			if bad, _ := healthRows(name, healthRecords); bad || missingExpectedRuntime(name, config, healthRecords) {
				nodeFault = true
				out.Findings = append(out.Findings, dto.Finding{DeviceID: d.Id, Code: "health_" + name, Severity: "fault", Message: name + " 运行状态异常", Evidence: e.Latest})
			}
		}
		if !n.Supported {
			nodeHealthy = false
			out.Findings = append(out.Findings, dto.Finding{DeviceID: d.Id, Code: "platform_unsupported", Severity: "unknown", Message: "首版仅支持 Cisco NX-OS"})
		}
		if nodeFault {
			n.State = "fault"
			fault = true
		} else if nodeHealthy {
			n.State = "healthy"
		} else {
			allHealthy = false
		}
		if n.State != "healthy" {
			allHealthy = false
		}
		switch d.DeviceRole {
		case "leaf_switch":
			leaves = append(leaves, d.IP)
			out.LeafCount++
		case "border_switch":
			borders = append(borders, d.IP)
		case "spine_switch":
			spines = append(spines, d.IP)
		default:
			allHealthy = false
			out.Findings = append(out.Findings, dto.Finding{DeviceID: d.Id, Code: "role_unknown", Severity: "unknown", Message: "设备角色未确定"})
		}
		if config != nil {
			configs[d.IP] = config
			documents = append(documents, vxlan.Record{"device": vxlan.Record{"host": d.IP, "name": d.Name}, "snapshot": vxlan.Record{"dimensions": vxlan.Record{"configuration": vxlan.Record{"status": "complete", "records": []vxlan.Record{config}}}}})
		}
		versions = append(versions, []any{d.Id, d.Version, m.Version, d.IP, d.DeviceRole, currentSourceHash})
		out.Nodes = append(out.Nodes, n)
	}
	if len(leaves)+len(borders)+len(spines) >= 2 {
		review, err := vxlan.ReviewFabric(documents, vxlan.Record{"fabrics": vxlan.Record{f.Code: vxlan.Record{"leaves": leaves, "borders": borders, "spines": spines}}})
		if err != nil {
			return out, err
		}
		for _, r := range records(review["findings"]) {
			severity := "fault"
			if strings.Contains(text(r["code"]), "evidence") {
				severity = "unknown"
				allHealthy = false
			} else {
				fault = true
			}
			out.Findings = append(out.Findings, dto.Finding{VNI: number(r["vni"]), Role: text(r["role"]), Code: text(r["code"]), Severity: severity, Message: text(r["title"]), Evidence: r["evidence"]})
		}
	}
	out.Segments = segmentMatrix(out.Nodes, observedConfigs)
	for _, segment := range out.Segments {
		if segment.State == "different" {
			fault = true
			out.Findings = append(out.Findings, dto.Finding{Scope: "business", VRF: segment.VRF, Check: "segment_configuration", VNI: segment.VNI, Code: "service_matrix_different", Severity: "fault", Message: "全部 Leaf 的观察网段存在缺失、冲突或配置差异；观察并集不代表正确意图", Evidence: segment})
		} else if segment.State == "unknown" {
			allHealthy = false
			out.UnknownCount++
			out.Findings = append(out.Findings, dto.Finding{Scope: "business", VRF: segment.VRF, Check: "segment_configuration", VNI: segment.VNI, Code: "service_matrix_unknown", Severity: "unknown", Message: "网段矩阵证据不足，尚不能确认全部 Leaf 一致", Evidence: segment})
		}
		for _, cell := range segment.Cells {
			if record(cell.Configuration["svi"]) == nil {
				continue
			}
			for direction, binding := range map[string]string{"in": cell.ACLIn, "out": cell.ACLOut} {
				if binding != "unbound" {
					continue
				}
				allHealthy = false
				out.UnknownCount++
				out.Findings = append(out.Findings, dto.Finding{Scope: "business", VRF: segment.VRF, Check: "acl_binding", Direction: direction, DeviceID: cell.DeviceID, VNI: segment.VNI, Code: "acl_" + direction + "_unbound", Severity: "unknown", Message: direction + " 方向 ACL 未绑定；需确认此业务的访问控制意图", Evidence: cell})
			}
		}
	}
	out.VPCGroups = VPCGroups(out.Nodes)
	paired := map[int]bool{}
	for _, pair := range out.VPCGroups {
		for _, deviceID := range pair.DeviceIDs {
			paired[deviceID] = true
		}
	}
	for _, node := range out.Nodes {
		if node.VPCDomain != "" && !paired[node.DeviceID] {
			allHealthy = false
			out.UnknownCount++
			out.Findings = append(out.Findings, dto.Finding{DeviceID: node.DeviceID, Code: "vpc_pair_unconfirmed", Severity: "unknown", Message: "vPC 双端与 peer 地址尚未形成相互确认的配对证据"})
		}
	}
	if !full {
		allHealthy = false
		out.Findings = append(out.Findings, dto.Finding{Code: "scope_partial", Severity: "unknown", Message: "仅显示有权限的成员，不能代表全 Fabric 健康"})
	}
	if fault {
		out.State = "fault"
	} else if allHealthy {
		out.State = "healthy"
	}
	attachRemediations(&out)
	scopeFindings(&out)
	out.FindingCount = len(out.Findings)
	out.IssueGroups = buildIssueGroups(out)
	out.IssueCount = len(out.IssueGroups)
	reconcileIssueHealth(&out)
	if out.ApplicableCount > 0 {
		out.EvidenceCoverage = out.CheckedCount * 100 / out.ApplicableCount
	}
	out.EvidenceKey = vxlan.Fingerprint(versions)
	scope := vxlan.Fingerprint(ids)
	encoded, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	assessment := models.Assessment{FabricID: id, EvidenceKey: out.EvidenceKey, ScopeKey: scope, RulesVersion: f.RulesVersion, CompleteScope: full, State: out.State, Result: encoded}
	if err := s.Orm.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "scope_key"}, {Name: "evidence_key"}}, DoNothing: true}).Create(&assessment).Error; err != nil {
		return out, err
	}
	if assessment.ID == 0 {
		if err := s.Orm.WithContext(ctx).Where("scope_key = ? AND evidence_key = ?", scope, out.EvidenceKey).First(&assessment).Error; err != nil {
			return out, err
		}
	}
	out.AssessmentID = assessment.ID
	return out, nil
}
func segmentMatrix(nodes []dto.Node, configs vxlan.Record) []dto.Segment {
	leafConfigs := vxlan.Record{}
	for _, n := range nodes {
		if n.Role == "leaf_switch" {
			if c := configs[n.IP]; c != nil {
				leafConfigs[n.IP] = c
			}
		}
	}
	catalog := vxlan.ServiceCatalog(leafConfigs)
	vnis := []int{}
	for vni := range catalog {
		vnis = append(vnis, vni)
	}
	sort.Ints(vnis)
	out := []dto.Segment{}
	for _, vni := range vnis {
		contexts := map[string]map[string][]vxlan.Record{}
		sameVNI := map[string]bool{}
		for _, row := range catalog[vni] {
			vrf, host := text(record(row["svi"])["vrf"]), text(row["host"])
			if contexts[vrf] == nil {
				contexts[vrf] = map[string][]vxlan.Record{}
			}
			contexts[vrf][host] = append(contexts[vrf][host], row)
			sameVNI[host] = true
		}
		vrfs := []string{}
		for vrf := range contexts {
			vrfs = append(vrfs, vrf)
		}
		sort.Strings(vrfs)
		for _, vrf := range vrfs {
			segment := dto.Segment{VRF: vrf, VNI: vni, State: "consistent", Cells: []dto.SegmentCell{}}
			fingerprints := map[string]bool{}
			for _, n := range nodes {
				if n.Role != "leaf_switch" {
					continue
				}
				cell := dto.SegmentCell{DeviceID: n.DeviceID, State: "missing", ACLIn: "unbound", ACLOut: "unbound", Configuration: map[string]any{}}
				if configs[n.IP] == nil {
					cell.ACLIn = "unknown"
					cell.ACLOut = "unknown"
					cell.State = "unknown"
				}
				matches := contexts[vrf][n.IP]
				if len(matches) == 0 && sameVNI[n.IP] {
					cell.State = "conflict"
				}
				if len(matches) == 1 {
					r := matches[0]
					svi := record(r["svi"])
					bindings := record(svi["acl_bindings"])
					cell.State = "present"
					if len(texts(r["missing"])) > 0 {
						cell.State = "incomplete"
					}
					// The node observation owns the complete configuration; cells
					// project only the business fields used by the matrix and hints.
					cell.Configuration = vxlan.Record{"host": r["host"], "vlan": r["vlan"], "svi": r["svi"], "nve": r["nve"], "evpn": r["evpn"], "missing": r["missing"]}
					if v := text(bindings["in"]); v != "" {
						cell.ACLIn = v
					}
					if v := text(bindings["out"]); v != "" {
						cell.ACLOut = v
					}
					signature := vxlan.Record{"vlan": r["vlan"], "svi": svi, "nve": vxlan.EffectiveNVEMember(record(configs[n.IP]), record(r["nve"])), "tenant": vxlan.ServiceTenant(record(configs[n.IP]), text(svi["vrf"]))}
					policies := vxlan.Record{}
					for _, direction := range []string{"in", "out"} {
						name := text(bindings[direction])
						if name != "" {
							policy := vxlan.ResolveACLPolicy(record(configs[n.IP]), record(record(configs[n.IP])["acls"])[name])
							policies[direction] = policy
							if policy == nil {
								cell.State = "unknown"
							}
						}
					}
					signature["policies"] = policies
					fingerprints[vxlan.Fingerprint(signature)] = true
				} else if len(matches) > 1 {
					cell.State = "conflict"
				}
				if n.ConfigState == "historical" || n.ConfigState == "unknown" {
					cell.State = "unknown"
				}
				if cell.State == "unknown" {
					segment.State = "unknown"
				} else if cell.State != "present" && segment.State != "unknown" {
					segment.State = "different"
				}
				segment.Cells = append(segment.Cells, cell)
			}
			if len(fingerprints) > 1 && segment.State != "unknown" {
				segment.State = "different"
			}
			out = append(out, segment)
		}
	}
	return out
}
func (s *Fabric) Assessments(ctx context.Context, id int, r *dto.PageReq, p *actions.DataPermission) ([]models.Assessment, int64, error) {
	if err := r.Validate(); err != nil {
		return nil, 0, err
	}
	devices, _, err := s.VisibleMembers(ctx, id, p)
	if err != nil {
		return nil, 0, err
	}
	ids := []int{}
	for _, d := range devices {
		ids = append(ids, d.Id)
	}
	scope := vxlan.Fingerprint(ids)
	rows := []models.Assessment{}
	var count int64
	q := s.Orm.WithContext(ctx).Model(&models.Assessment{}).Where("fabric_id = ? AND scope_key = ?", id, scope)
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	err = q.Order("id DESC").Offset((r.PageIndex - 1) * r.PageSize).Limit(r.PageSize).Find(&rows).Error
	return rows, count, err
}

func canonicalInterface(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "ethernet", "eth")
	value = strings.ReplaceAll(value, "port-channel", "po")
	return value
}

// Only enabled modeled Fabric interfaces contribute to Fabric health. A spare
// physical port's link state does not imply a transport or service outage.
func configuredFabricInterfaces(config vxlan.Record) map[string]bool {
	relevant := map[string]bool{}
	for name, value := range record(config["interfaces"]) {
		i := record(value)
		if i["shutdown"] == true {
			continue
		}
		if i["peer_link"] == true || i["vpc_peer_link"] == true || i["vpc_id"] != nil || text(i["ospf"]) != "" || i["ospf_process"] != nil || i["unnumbered_interface"] != nil || strings.HasPrefix(strings.ToLower(name), "vlan") && i["vrf"] != nil {
			relevant[canonicalInterface(name)] = true
		}
	}
	nve := record(config["nve"])
	if name := text(nve["interface"]); name != "" && nve["shutdown"] != true {
		relevant[canonicalInterface(name)] = true
	}
	return relevant
}
func expectedFabricInterfaces(name string, config vxlan.Record) map[string]bool {
	expected := configuredFabricInterfaces(config)
	if name == "port_channels" {
		for iface := range expected {
			if !strings.HasPrefix(iface, "po") {
				delete(expected, iface)
			}
		}
	}
	return expected
}
func missingFabricInterfaceEvidence(name string, config vxlan.Record, rows []vxlan.Record) bool {
	if name != "interfaces" && name != "port_channels" {
		return false
	}
	observed := map[string]bool{}
	for _, row := range rows {
		observed[canonicalInterface(text(row["interface"]))] = true
	}
	for iface := range expectedFabricInterfaces(name, config) {
		if !observed[iface] {
			return true
		}
	}
	return false
}
func fabricInterfaceRows(config vxlan.Record, rows []vxlan.Record) []vxlan.Record {
	relevant := configuredFabricInterfaces(config)
	out := []vxlan.Record{}
	for _, row := range rows {
		if relevant[canonicalInterface(text(row["interface"]))] {
			out = append(out, row)
		}
	}
	return out
}

func historicalConfig(e dto.Evidence) vxlan.Record {
	if e.LastComplete == nil || len(e.LastComplete.Data.Records) != 1 || e.LastComplete.Data.Records[0]["review_status"] != "complete" {
		return nil
	}
	return e.LastComplete.Data.Records[0]
}

func missingExpectedRuntime(name string, config vxlan.Record, rows []vxlan.Record) bool {
	switch name {
	case "nve_vnis":
		observed := map[string]bool{}
		for _, r := range rows {
			observed[text(r["vni"])] = true
		}
		for vni := range record(record(config["nve"])["members"]) {
			if !observed[vni] {
				return true
			}
		}
	case "bgp_neighbors":
		observed := map[string]bool{}
		for _, r := range rows {
			observed[text(r["vrf"])+"\x00"+text(r["peer_ip"])] = true
		}
		for _, n := range vxlan.BGPNeighbors(record(config["bgp"])) {
			settings := record(n["settings"])
			if settings["shutdown"] == true || record(settings["address_families"])["l2vpn evpn"] == nil {
				continue
			}
			if !observed[text(n["vrf"])+"\x00"+text(n["peer"])] {
				return true
			}
		}
	case "vpc":
		domain := record(config["vpc_domain"])
		if domain == nil {
			return false
		}
		if len(rows) == 0 {
			return true
		}
		for _, r := range rows {
			if text(r["domain_id"]) != text(domain["id"]) {
				return true
			}
		}
	}
	return false
}
