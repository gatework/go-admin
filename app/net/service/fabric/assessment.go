package fabric

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm/clause"

	vxlan "github.com/gatework/go-vxlan"
	"github.com/gatework/motor-core/sdk/contract/actions"
	coredto "github.com/gatework/motor-core/sdk/contract/dto"
	models "github.com/gatework/motor/app/net/models/fabric"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
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

// VPCGroups provides the v p c groups operation within the caller's authorized fabric scope.
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
		rowFault, rowKnown := healthRow(name, r)
		fault = fault || rowFault
		known = known && rowKnown
	}
	return fault, known
}

func healthRow(name string, r vxlan.Record) (bool, bool) {
	fault, known := false, true
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
	return fault, known
}

// Detail provides the detail operation within the caller's authorized fabric scope.
func (s *Fabric) Detail(ctx context.Context, id int, p *actions.DataPermission) (dto.Detail, error) {
	out := dto.Detail{Nodes: []dto.Node{}, Segments: []dto.Segment{}, VPCGroups: []dto.VPCGroup{}, Findings: []dto.Finding{}}
	devices, full, err := s.VisibleMembers(ctx, id, p)
	if err != nil {
		return out, err
	}
	var f models.Fabric
	if err := s.DB.WithContext(ctx).First(&f, id).Error; err != nil {
		return out, err
	}
	out.Summary = dto.Summary{Fabric: f, CompleteScope: full, MemberCount: len(devices), State: "unknown"}
	now := time.Now().UTC()
	ids := []int{}
	for _, d := range devices {
		ids = append(ids, d.ID)
	}
	observations, err := s.evidenceRows(ctx, id, ids)
	if err != nil {
		return out, err
	}
	a := assessmentBuilder{out: &out, fabric: f, now: now, observedConfigs: vxlan.Record{}, documents: []vxlan.Record{}, leaves: []string{}, borders: []string{}, spines: []string{}, versions: []any{issueAssessmentVersion, f.ID, f.Version, f.RulesVersion}, allHealthy: len(devices) > 0 && full}
	for _, d := range devices {
		if err := a.assessNode(ctx, s, d, observations); err != nil {
			return out, err
		}
	}
	if err := a.reviewTopology(); err != nil {
		return out, err
	}
	a.reviewSegments()
	a.reviewVPC()
	if !full {
		a.allHealthy = false
		out.Findings = append(out.Findings, dto.Finding{Code: "scope_partial", Severity: "unknown", Message: "仅显示有权限的成员，不能代表全 Fabric 健康"})
	}
	if a.fault {
		out.State = "fault"
	} else if a.allHealthy {
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
	out.EvidenceKey = vxlan.Fingerprint(a.versions)
	scope := vxlan.Fingerprint(ids)
	encoded, err := json.Marshal(out)
	if err != nil {
		return out, err
	}
	assessment := models.Assessment{FabricID: id, EvidenceKey: out.EvidenceKey, ScopeKey: scope, RulesVersion: f.RulesVersion, CompleteScope: full, State: out.State, Result: encoded}
	if err := s.DB.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "scope_key"}, {Name: "evidence_key"}}, DoNothing: true}).Create(&assessment).Error; err != nil {
		return out, err
	}
	if assessment.ID == 0 {
		if err := s.DB.WithContext(ctx).Where("scope_key = ? AND evidence_key = ?", scope, out.EvidenceKey).First(&assessment).Error; err != nil {
			return out, err
		}
	}
	out.AssessmentID = assessment.ID
	return out, nil
}

// Assessments provides the assessments operation within the caller's authorized fabric scope.
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
		ids = append(ids, d.ID)
	}
	scope := vxlan.Fingerprint(ids)
	rows := []models.Assessment{}
	var count int64
	q := s.DB.WithContext(ctx).Model(&models.Assessment{}).Where("fabric_id = ? AND scope_key = ?", id, scope)
	if err := q.Count(&count).Error; err != nil {
		return nil, 0, err
	}
	err = q.Order("id DESC").Scopes(coredto.Paginate(r.PageSize, r.PageIndex)).Find(&rows).Error
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
