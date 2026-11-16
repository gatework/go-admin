package fabric

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"time"

	"gorm.io/gorm"

	vxlan "github.com/gatework/go-vxlan"
	"github.com/gatework/motor-core/sdk/contract/actions"
	models "github.com/gatework/motor/app/net/models/fabric"
	networkmodels "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
)

// EndpointQuery describes endpoint query for fabric operations.
type EndpointQuery struct {
	Kind   string
	Value  string
	Prefix netip.Prefix
}

// ParseEndpointQuery decodes and validates endpoint query input.
func ParseEndpointQuery(value string) (EndpointQuery, error) {
	value = strings.TrimSpace(value)
	if p, err := netip.ParsePrefix(value); err == nil {
		if p.Addr().Zone() != "" {
			return EndpointQuery{}, InvalidErr
		}
		p = p.Masked()
		return EndpointQuery{Kind: "cidr", Value: p.String(), Prefix: p}, nil
	}
	if ip, err := netip.ParseAddr(value); err == nil && ip.Zone() == "" {
		return EndpointQuery{Kind: "ip", Value: ip.Unmap().String()}, nil
	}
	compact := strings.NewReplacer(".", "", ":", "", "-", "").Replace(value)
	if len(compact) == 12 {
		parts := []string{}
		for i := 0; i < 12; i += 2 {
			parts = append(parts, compact[i:i+2])
		}
		if mac, err := net.ParseMAC(strings.Join(parts, ":")); err == nil {
			return EndpointQuery{Kind: "mac", Value: strings.ToLower(mac.String())}, nil
		}
	}
	return EndpointQuery{}, InvalidErr
}

// Matches provides the matches operation within the caller's authorized fabric scope.
func (q EndpointQuery) Matches(e dto.Endpoint, vrf string) bool {
	if vrf != "" && e.VRF != vrf {
		return false
	}
	switch q.Kind {
	case "mac":
		return e.MAC == q.Value
	case "ip":
		return e.IP == q.Value
	case "cidr":
		ip, err := netip.ParseAddr(e.IP)
		return err == nil && q.Prefix.Contains(ip)
	}
	return false
}
func normalizeMAC(value string) string {
	q, err := ParseEndpointQuery(value)
	if err == nil && q.Kind == "mac" {
		return q.Value
	}
	return ""
}
func normalizeIP(value string) string {
	ip, err := netip.ParseAddr(value)
	if err != nil || ip.Zone() != "" {
		return ""
	}
	return ip.Unmap().String()
}
func endpointVRF(config vxlan.Record, vlan int) string {
	return text(record(record(config["interfaces"])["Vlan"+text(vlan)])["vrf"])
}
func endpointRows(e models.Observation, name string, config vxlan.Record, state string) []dto.Endpoint {
	out := []dto.Endpoint{}
	for _, r := range e.Data.Records {
		out = append(out, endpointRecord(e, r, name, config, state)...)
	}
	return out
}

// Endpoints provides the endpoints operation within the caller's authorized fabric scope.
func (s *Fabric) Endpoints(ctx context.Context, r *dto.PageReq, p *actions.DataPermission) ([]dto.Endpoint, int64, error) {
	if err := r.Validate(); err != nil {
		return nil, 0, err
	}
	query, err := ParseEndpointQuery(r.Keyword)
	if err != nil {
		return nil, 0, err
	}
	var fabrics []models.Fabric
	db := s.DB.WithContext(ctx)
	if r.FabricID > 0 {
		db = db.Where("id = ?", r.FabricID)
	}
	if err := db.Order("id").Find(&fabrics).Error; err != nil {
		return nil, 0, err
	}
	out := []dto.Endpoint{}
	seen := map[string]bool{}
	peerProof := map[int]dto.Node{}
	for _, f := range fabrics {
		devices, _, err := s.VisibleMembers(ctx, f.ID, p)
		if errors.Is(err, NotFoundErr) {
			continue
		}
		if err != nil {
			return nil, 0, err
		}
		for _, d := range devices {
			localRows, sourceEvidence, err := s.deviceEndpoints(ctx, f, d, peerProof)
			if err != nil {
				return nil, 0, err
			}
			for _, item := range correlateLocalEndpointPorts(localRows, sourceEvidence) {
				if !query.Matches(item, r.VRF) {
					continue
				}
				key := vxlan.Fingerprint(item)
				if !seen[key] {
					seen[key] = true
					out = append(out, item)
				}
			}
		}

	}
	classifyEndpointCandidates(out, peerProof)
	count := int64(len(out))
	start, end := pageBounds(len(out), r)
	return out[start:end], count, nil
}

// Resolve VRF only against matching configuration acquired before this endpoint
// observation. Newer configuration cannot retroactively change historical VRF.
func endpointConfiguration(rows []models.Observation, endpoint models.Observation) vxlan.Record {
	var chosen *models.Observation
	for i := range rows {
		o := &rows[i]
		if o.Dimension != "configuration" || o.Status != "complete" || o.SourceHash != endpoint.SourceHash || o.MembershipVersion != endpoint.MembershipVersion || o.RuleVersion != endpoint.RuleVersion || o.ObservedAt.After(endpoint.ObservedAt) || len(o.Data.Records) != 1 || o.Data.Records[0]["review_status"] != "complete" {
			continue
		}
		if chosen == nil || o.ObservedAt.After(chosen.ObservedAt) {
			chosen = o
		}
	}
	if chosen == nil {
		return nil
	}
	return chosen.Data.Records[0]
}

func interfaceVPCID(config vxlan.Record, iface string) string {
	if iface == "" {
		return ""
	}
	for name, value := range record(config["interfaces"]) {
		if canonicalInterface(name) == canonicalInterface(iface) {
			return text(record(value)["vpc_id"])
		}
	}
	return ""
}
func classifyEndpointCandidates(rows []dto.Endpoint, proof map[int]dto.Node) {
	groups := map[string][]int{}
	candidates := map[string]map[string]bool{}
	candidateKeys := make([]string, len(rows))
	for i, r := range rows {
		key := vxlan.Fingerprint([]any{r.FabricID, r.VRF, r.MAC, r.IP, r.VNI})
		groups[key] = append(groups[key], i)
		identity := []any{r.FabricID, r.VRF, "ip", r.IP}
		if r.IP == "" {
			identity = []any{r.FabricID, r.VRF, "mac", r.MAC}
		}
		candidateKeys[i] = vxlan.Fingerprint(identity)
		if candidates[candidateKeys[i]] == nil {
			candidates[candidateKeys[i]] = map[string]bool{}
		}
		position := vxlan.Fingerprint([]any{r.DeviceID, r.MAC, r.VNI, r.VLAN, canonicalInterface(r.Interface), r.RemoteVTEP})
		candidates[candidateKeys[i]][position] = true
	}
	for _, indices := range groups {
		for _, i := range indices {
			rows[i].CandidateCount = len(candidates[candidateKeys[i]])
		}
		classifyEndpointGroup(rows, indices, proof)
	}
}

// The latest configuration pointer may be newer than an endpoint-only snapshot.
// Select one bounded acquisition-time candidate, never load all configuration history.
func (s *Fabric) endpointConfigAt(ctx context.Context, endpoint models.Observation, maxAge time.Duration) (vxlan.Record, error) {
	if maxAge <= 0 {
		maxAge = time.Hour
	}
	var row models.Observation
	err := s.DB.WithContext(ctx).Where("fabric_id = ? AND device_id = ? AND dimension = ? AND status = ? AND source_hash = ? AND membership_version = ? AND rule_version = ? AND observed_at <= ? AND observed_at >= ?", endpoint.FabricID, endpoint.DeviceID, "configuration", "complete", endpoint.SourceHash, endpoint.MembershipVersion, endpoint.RuleVersion, endpoint.ObservedAt, endpoint.ObservedAt.Add(-maxAge)).Order("observed_at DESC,id DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return vxlan.Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	return endpointConfiguration([]models.Observation{row}, endpoint), nil
}

func correlateLocalEndpointPorts(rows []dto.Endpoint, evidence map[int]models.Observation) []dto.Endpoint {
	out := []dto.Endpoint{}
	for _, endpoint := range rows {
		if endpoint.Kind != "local_unknown_port" || endpoint.State != "current" || endpoint.VRF == "unknown" || endpoint.VLAN <= 0 || endpoint.VNI <= 0 {
			out = append(out, endpoint)
			continue
		}
		matched := false
		a := evidence[endpoint.EvidenceID]
		for _, port := range rows {
			b := evidence[port.EvidenceID]
			if b.Dimension != "mac_addresses" || port.State != "current" || port.Kind != "local_port" || port.Interface == "" || port.DeviceID != endpoint.DeviceID || port.MAC != endpoint.MAC || port.VRF != endpoint.VRF || port.VLAN != endpoint.VLAN || port.VNI != endpoint.VNI || a.SourceHash != b.SourceHash || a.MembershipVersion != b.MembershipVersion || a.RuleVersion != b.RuleVersion {
				continue
			}
			item := endpoint
			item.Kind = "local_port"
			item.Interface = port.Interface
			item.VPCID = port.VPCID
			out = append(out, item)
			matched = true
		}
		if !matched {
			out = append(out, endpoint)
		}
	}
	return out
}

func endpointRecord(e models.Observation, r vxlan.Record, name string, config vxlan.Record, state string) []dto.Endpoint {
	out := []dto.Endpoint{}
	vlan := number(r["vlan_id"])
	vni := number(r["vni"])
	if vlan == 0 {
		vlan = number(r["topology_id"])
	}
	if vni == 0 {
		vni = number(record(config["vlan_vnis"])[text(vlan)])
	}
	base := dto.Endpoint{FabricID: e.FabricID, DeviceID: e.DeviceID, DeviceName: name, VRF: text(r["vrf"]), IP: normalizeIP(text(r["host_ip"])), MAC: normalizeMAC(text(r["mac"])), VLAN: vlan, VNI: vni, State: state, ObservedAt: e.ObservedAt, EvidenceID: e.ID}
	if base.VRF == "" {
		base.VRF = endpointVRF(config, vlan)
	}
	if base.VRF == "" {
		base.VRF = "unknown"
	}
	if base.MAC == "" {
		return out
	}
	hops := records(r["next_hops"])
	if len(hops) == 0 {
		destination := text(r["destination"])
		remote := normalizeIP(destination)
		if i := strings.Index(destination, "("); i >= 0 && strings.HasPrefix(strings.ToLower(destination), "nve") && strings.HasSuffix(destination, ")") {
			remote = normalizeIP(destination[i+1 : len(destination)-1])
		}
		if remote != "" {
			base.Kind = "remote_vtep"
			base.RemoteVTEP = remote
		} else {
			base.Kind = "local_port"
			base.Interface = destination
			if destination == "" {
				base.Kind = "unknown"
			}
		}
		base.VPCID = interfaceVPCID(config, base.Interface)
		out = append(out, base)
		return out
	}
	for _, hop := range hops {
		item := base
		if address := normalizeIP(text(hop["address"])); address != "" {
			item.Kind = "remote_vtep"
			item.RemoteVTEP = address
		} else {
			item.Interface = text(hop["interface"])
			item.Kind = "local_port"
			if strings.HasPrefix(strings.ToLower(item.Interface), "vpc") {
				item.Kind = "unknown"
				item.VPCID = strings.TrimPrefix(strings.ToLower(item.Interface), "vpc")
			}
			if item.Interface == "" {
				item.Kind = "local_unknown_port"
			}
		}
		if item.VPCID == "" {
			item.VPCID = interfaceVPCID(config, item.Interface)
		}
		out = append(out, item)
	}
	return out
}

func (s *Fabric) deviceEndpoints(ctx context.Context, f models.Fabric, d networkmodels.Device, peerProof map[int]dto.Node) ([]dto.Endpoint, map[int]models.Observation, error) {
	observations, err := s.evidenceRows(ctx, f.ID, []int{d.ID})
	if err != nil {
		return nil, nil, err
	}
	var membership models.Membership
	if err := s.DB.WithContext(ctx).First(&membership, "device_id = ?", d.ID).Error; err != nil {
		return nil, nil, err
	}
	connection, connectionErr := s.deviceConnection(ctx, d, deviceaccess.Read)
	currentSource := sourceHash(opsDevice(d), connection)

	localRows := []dto.Endpoint{}
	sourceEvidence := map[int]models.Observation{}
	for _, dimension := range []string{"l2route_mac_ips", "l2route_macs", "mac_addresses"} {
		ev := deviceEvidence{Fabric: f, Device: d, Membership: membership, Observations: observations, CurrentSource: currentSource, ConnectionErr: connectionErr}
		rows, err := s.dimensionEndpoints(ctx, ev, dimension, peerProof, sourceEvidence)
		if err != nil {
			return nil, nil, err
		}
		localRows = append(localRows, rows...)
	}
	return localRows, sourceEvidence, nil

}

// deviceEvidence is one fabric member with what its observations are judged
// against: its membership, its stored observations, the source hash its
// device and connection have now, and whether that connection resolved.
type deviceEvidence struct {
	Fabric        models.Fabric
	Device        networkmodels.Device
	Membership    models.Membership
	Observations  []models.Observation
	CurrentSource string
	ConnectionErr error
}

func (s *Fabric) dimensionEndpoints(ctx context.Context, ev deviceEvidence, dimension string, peerProof map[int]dto.Node, sourceEvidence map[int]models.Observation) ([]dto.Endpoint, error) {
	f, d, membership, observations := ev.Fabric, ev.Device, ev.Membership, ev.Observations
	currentSource, connectionErr := ev.CurrentSource, ev.ConnectionErr
	localRows := []dto.Endpoint{}
	e := DimensionEvidence(observations, dimension, time.Now().UTC(), 2*time.Duration(f.RuntimeMinutes)*time.Minute)
	sources := []struct {
		row   *models.Observation
		state string
	}{}
	if e.Latest != nil {
		state := "partial"
		if e.State == "complete" {
			state = "current"
		}
		if e.Stale {
			state = "stale"
		}
		if connectionErr != nil || e.Latest.SourceHash != currentSource || e.Latest.MembershipVersion != membership.Version || e.Latest.RuleVersion != f.RulesVersion {
			state = "historical"
		}
		sources = append(sources, struct {
			row   *models.Observation
			state string
		}{e.Latest, state})
	}
	if e.LastComplete != nil && (e.Latest == nil || e.LastComplete.ID != e.Latest.ID) {
		sources = append(sources, struct {
			row   *models.Observation
			state string
		}{e.LastComplete, "historical"})
	}
	for _, source := range sources {
		config, err := s.endpointConfigAt(ctx, *source.row, 2*time.Duration(f.ConfigMinutes)*time.Minute)
		if err != nil {
			return nil, err
		}
		sourceEvidence[source.row.ID] = *source.row
		s.endpointPeerProof(ev, *source.row, source.state, config, peerProof)
		for _, item := range endpointRows(*source.row, d.Name, config, source.state) {
			item.FabricCode = f.Code
			localRows = append(localRows, item)
		}
	}
	return localRows, nil
}
func (s *Fabric) endpointPeerProof(ev deviceEvidence, source models.Observation, state string, config vxlan.Record, peerProof map[int]dto.Node) {
	f, d, membership, observations := ev.Fabric, ev.Device, ev.Membership, ev.Observations
	if config != nil && state == "current" {
		vpc := record(config["vpc_domain"])
		peer := record(vpc["peer_keepalive"])
		peerProof[source.ID] = dto.Node{DeviceID: d.ID, IP: d.IP, VPCDomain: text(vpc["id"]), VPCSource: text(peer["source"]), VPCPeer: text(peer["destination"])}
		proof := peerProof[source.ID]
		currentVPC := DimensionEvidence(observations, "vpc", time.Now().UTC(), 2*time.Duration(f.RuntimeMinutes)*time.Minute)
		if currentVPC.State == "complete" && currentVPC.Latest != nil {
			latest := currentVPC.Latest
			bad, _ := healthRows("vpc", latest.Data.Records)
			if !bad && len(latest.Data.Records) > 0 && !missingExpectedRuntime("vpc", config, latest.Data.Records) && latest.SourceHash == source.SourceHash && latest.MembershipVersion == membership.Version && latest.RuleVersion == f.RulesVersion {
				proof.State = "healthy"
			}
		}
		peerProof[source.ID] = proof
	}
}

func classifyEndpointGroup(rows []dto.Endpoint, indices []int, proof map[int]dto.Node) {
	if conflictingEndpointAttachments(rows, indices) {
		return
	}

	for a, i := range indices {
		r := &rows[i]
		if r.VRF == "unknown" || r.State != "current" || (r.Kind != "local_port" && r.Kind != "unknown") || r.VPCID == "" {
			continue
		}
		for _, j := range indices[a+1:] {
			classifyEndpointPair(r, &rows[j], proof)
		}
	}

}
func conflictingEndpointAttachments(rows []dto.Endpoint, indices []int) bool {
	attachments := map[int]map[string]bool{}
	for _, i := range indices {
		row := rows[i]
		if row.State != "current" || row.VPCID == "" {
			continue
		}
		if attachments[row.DeviceID] == nil {
			attachments[row.DeviceID] = map[string]bool{}
		}
		attachments[row.DeviceID][row.VPCID+":"+text(row.VLAN)] = true
	}
	conflict := false
	for _, ports := range attachments {
		if len(ports) > 1 {
			conflict = true
		}
	}
	return conflict
}
func classifyEndpointPair(r, other *dto.Endpoint, proof map[int]dto.Node) {

	if r.DeviceID == other.DeviceID || other.State != "current" || (other.Kind != "local_port" && other.Kind != "unknown") || other.VPCID != r.VPCID || other.VLAN != r.VLAN {
		return
	}
	nodes := []dto.Node{proof[r.EvidenceID], proof[other.EvidenceID]}
	pairs := VPCGroups(nodes)
	if len(pairs) != 1 || pairs[0].State != "healthy" {
		return
	}
	r.Kind = "vpc_dual_homed"
	other.Kind = "vpc_dual_homed"
	r.VPCGroupKey = pairs[0].Key
	other.VPCGroupKey = pairs[0].Key
}
