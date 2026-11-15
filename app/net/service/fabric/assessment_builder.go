package fabric

import (
	"context"
	"strings"
	"time"

	vxlan "github.com/gatework/go-vxlan"
	models "github.com/gatework/motor/app/net/models/fabric"
	networkmodels "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
)

type assessmentBuilder struct {
	out                     *dto.Detail
	fabric                  models.Fabric
	now                     time.Time
	observedConfigs         vxlan.Record
	documents               []vxlan.Record
	leaves, borders, spines []string
	versions                []any
	allHealthy, fault       bool
}

func (a *assessmentBuilder) assessNode(ctx context.Context, s *Fabric, d networkmodels.Device, observations []models.Observation) error {
	var m models.Membership
	if err := s.DB.WithContext(ctx).First(&m, "device_id = ?", d.ID).Error; err != nil {
		return err
	}
	connection, connectionErr := s.deviceConnection(ctx, d, deviceaccess.Read)
	currentSourceHash := sourceHash(opsDevice(d), connection)
	n := dto.Node{DeviceID: d.ID, Name: d.Name, IP: d.IP, Role: d.DeviceRole, Version: d.Version, MembershipVersion: m.Version, State: "unknown", Model: d.HardwareModel, Platform: d.OS, Supported: connectionErr == nil && connection.Profile == "cisco_nxos", Dimensions: map[string]dto.Evidence{}, Features: []string{}}
	a.nodeEvidence(&n, d.ID, m.Version, currentSourceHash, connectionErr, observations)

	config := configRecord(n.Dimensions["configuration"])
	a.nodeConfiguration(&n, config)

	nodeHealthy := config != nil && n.Supported
	nodeFault := false
	if config != nil {
		for _, finding := range vxlan.ConfigReview(config, true) {
			nodeFault = true
			a.out.Findings = append(a.out.Findings, dto.Finding{DeviceID: d.ID, VRF: text(finding["vrf"]), VNI: number(finding["vni"]), Code: text(finding["code"]), Severity: "fault", Message: "配置完整性检查发现不一致", Evidence: finding})
		}
	}
	runtimeHealthy, runtimeFault := a.nodeRuntime(n, config)
	nodeHealthy = nodeHealthy && runtimeHealthy
	nodeFault = nodeFault || runtimeFault

	if !n.Supported {
		nodeHealthy = false
		a.out.Findings = append(a.out.Findings, dto.Finding{DeviceID: d.ID, Code: "platform_unsupported", Severity: "unknown", Message: "首版仅支持 Cisco NX-OS"})
	}
	if nodeFault {
		n.State = "fault"
		a.fault = true
	} else if nodeHealthy {
		n.State = "healthy"
	} else {
		a.allHealthy = false
	}
	if n.State != "healthy" {
		a.allHealthy = false
	}
	switch d.DeviceRole {
	case "leaf_switch":
		a.leaves = append(a.leaves, d.IP)
		a.out.LeafCount++
	case "border_switch":
		a.borders = append(a.borders, d.IP)
	case "spine_switch":
		a.spines = append(a.spines, d.IP)
	default:
		a.allHealthy = false
		a.out.Findings = append(a.out.Findings, dto.Finding{DeviceID: d.ID, Code: "role_unknown", Severity: "unknown", Message: "设备角色未确定"})
	}
	if config != nil {
		a.documents = append(a.documents, vxlan.Record{"device": vxlan.Record{"host": d.IP, "name": d.Name}, "snapshot": vxlan.Record{"dimensions": vxlan.Record{"configuration": vxlan.Record{"status": "complete", "records": []vxlan.Record{config}}}}})
	}
	a.versions = append(a.versions, []any{d.ID, d.Version, m.Version, d.IP, d.DeviceRole, currentSourceHash})
	a.out.Nodes = append(a.out.Nodes, n)
	return nil
}
func (a *assessmentBuilder) nodeEvidence(n *dto.Node, deviceID int, membershipVersion int64, currentSourceHash string, connectionErr error, observations []models.Observation) {
	local := []models.Observation{}
	for _, o := range observations {
		if o.DeviceID == deviceID {
			local = append(local, o)
		}
	}
	for _, name := range vxlan.DimensionNames() {
		age := 2 * time.Duration(a.fabric.RuntimeMinutes) * time.Minute
		if name == "configuration" {
			age = 2 * time.Duration(a.fabric.ConfigMinutes) * time.Minute
		}
		if age <= 0 {
			age = 10 * time.Minute
		}
		e := DimensionEvidence(local, name, a.now, age)
		if e.Latest != nil {
			a.versions = append(a.versions, []any{deviceID, name, e.Latest.ID, e.State, e.Stale})
			if a.out.ObservedAt == nil || e.Latest.ObservedAt.After(*a.out.ObservedAt) {
				at := e.Latest.ObservedAt
				a.out.ObservedAt = &at
			}
			if connectionErr != nil || e.Latest.SourceHash != currentSourceHash || e.Latest.MembershipVersion != membershipVersion || e.Latest.RuleVersion != a.fabric.RulesVersion {
				e.State = "unknown"
			}
		}
		n.Dimensions[name] = e
	}
}
func (a *assessmentBuilder) nodeConfiguration(n *dto.Node, config vxlan.Record) {
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
		a.observedConfigs[n.IP] = displayConfig
	}
	n.Features = texts(displayConfig["features"])
	vpc := record(displayConfig["vpc_domain"])
	n.VPCDomain = text(vpc["id"])
	keepalive := record(vpc["peer_keepalive"])
	n.VPCSource = text(keepalive["source"])
	n.VPCPeer = text(keepalive["destination"])
	n.VTEP = text(record(displayConfig["nve"])["source_interface"])
}
func (a *assessmentBuilder) nodeRuntime(n dto.Node, config vxlan.Record) (bool, bool) {
	nodeHealthy, nodeFault := true, false
	for _, name := range []string{"configuration", "nve_peers", "nve_vnis", "bgp_neighbors", "ospf_neighbors", "vpc", "interfaces", "port_channels"} {
		if !applicable(name, config) {
			continue
		}
		a.out.ApplicableCount++
		e := n.Dimensions[name]
		if e.State != "complete" {
			nodeHealthy = false
			a.out.UnknownCount++
			a.out.Findings = append(a.out.Findings, dto.Finding{DeviceID: n.DeviceID, Code: "evidence_" + name, Severity: "unknown", Message: name + " 证据缺失、失败、过期或归属版本已变化", Evidence: e})
			continue
		}
		a.out.CheckedCount++
		healthRecords := e.Latest.Data.Records
		if name == "interfaces" {
			healthRecords = fabricInterfaceRows(config, healthRecords)
		}
		if name == "port_channels" {
			healthRecords = fabricInterfaceRows(config, healthRecords)
		}
		if len(healthRecords) == 0 && (name == "nve_peers" || name == "ospf_neighbors") {
			nodeHealthy = false
			a.out.UnknownCount++
			a.out.Findings = append(a.out.Findings, dto.Finding{DeviceID: n.DeviceID, Code: "health_" + name + "_empty", Severity: "unknown", Message: name + " 未观察到邻居，尚不能证明协议健康", Evidence: e.Latest})
		}
		if bad, _ := healthRows(name, healthRecords); bad || missingExpectedRuntime(name, config, healthRecords) {
			nodeFault = true
			a.out.Findings = append(a.out.Findings, dto.Finding{DeviceID: n.DeviceID, Code: "health_" + name, Severity: "fault", Message: name + " 运行状态异常", Evidence: e.Latest})
		}
	}
	return nodeHealthy, nodeFault
}

func (a *assessmentBuilder) reviewTopology() error {
	if len(a.leaves)+len(a.borders)+len(a.spines) >= 2 {
		review, err := vxlan.ReviewFabric(a.documents, vxlan.Record{"fabrics": vxlan.Record{a.fabric.Code: vxlan.Record{"leaves": a.leaves, "borders": a.borders, "spines": a.spines}}})
		if err != nil {
			return err
		}
		for _, r := range records(review["findings"]) {
			severity := "fault"
			if strings.Contains(text(r["code"]), "evidence") {
				severity = "unknown"
				a.allHealthy = false
			} else {
				a.fault = true
			}
			a.out.Findings = append(a.out.Findings, dto.Finding{VNI: number(r["vni"]), Role: text(r["role"]), Code: text(r["code"]), Severity: severity, Message: text(r["title"]), Evidence: r["evidence"]})
		}
	}
	return nil
}
func (a *assessmentBuilder) reviewSegments() {
	a.out.Segments = segmentMatrix(a.out.Nodes, a.observedConfigs)
	for _, segment := range a.out.Segments {
		switch segment.State {
		case "different":
			a.fault = true
			a.out.Findings = append(a.out.Findings, dto.Finding{Scope: "business", VRF: segment.VRF, Check: "segment_configuration", VNI: segment.VNI, Code: "service_matrix_different", Severity: "fault", Message: "全部 Leaf 的观察网段存在缺失、冲突或配置差异；观察并集不代表正确意图", Evidence: segment})
		case "unknown":
			a.allHealthy = false
			a.out.UnknownCount++
			a.out.Findings = append(a.out.Findings, dto.Finding{Scope: "business", VRF: segment.VRF, Check: "segment_configuration", VNI: segment.VNI, Code: "service_matrix_unknown", Severity: "unknown", Message: "网段矩阵证据不足，尚不能确认全部 Leaf 一致", Evidence: segment})
		}
		for _, cell := range segment.Cells {
			if record(cell.Configuration["svi"]) == nil {
				continue
			}
			for direction, binding := range map[string]string{"in": cell.ACLIn, "out": cell.ACLOut} {
				if binding != "unbound" {
					continue
				}
				a.allHealthy = false
				a.out.UnknownCount++
				a.out.Findings = append(a.out.Findings, dto.Finding{Scope: "business", VRF: segment.VRF, Check: "acl_binding", Direction: direction, DeviceID: cell.DeviceID, VNI: segment.VNI, Code: "acl_" + direction + "_unbound", Severity: "unknown", Message: direction + " 方向 ACL 未绑定；需确认此业务的访问控制意图", Evidence: cell})
			}
		}
	}
}
func (a *assessmentBuilder) reviewVPC() {
	a.out.VPCGroups = VPCGroups(a.out.Nodes)
	paired := map[int]bool{}
	for _, pair := range a.out.VPCGroups {
		for _, deviceID := range pair.DeviceIDs {
			paired[deviceID] = true
		}
	}
	for _, node := range a.out.Nodes {
		if node.VPCDomain != "" && !paired[node.DeviceID] {
			a.allHealthy = false
			a.out.UnknownCount++
			a.out.Findings = append(a.out.Findings, dto.Finding{DeviceID: node.DeviceID, Code: "vpc_pair_unconfirmed", Severity: "unknown", Message: "vPC 双端与 peer 地址尚未形成相互确认的配对证据"})
		}
	}
}
