package fabric

import (
	"sort"

	vxlan "github.com/gatework/go-vxlan"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
)

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
		contexts, sameVNI := segmentContexts(catalog[vni])
		vrfs := []string{}
		for vrf := range contexts {
			vrfs = append(vrfs, vrf)
		}
		sort.Strings(vrfs)
		for _, vrf := range vrfs {
			segment := matrixSegment(nodes, configs, vni, vrf, contexts[vrf], sameVNI)
			out = append(out, segment)
		}
	}
	return out
}

func segmentContexts(rows []vxlan.Record) (map[string]map[string][]vxlan.Record, map[string]bool) {
	contexts := map[string]map[string][]vxlan.Record{}
	sameVNI := map[string]bool{}
	for _, row := range rows {
		vrf, host := text(record(row["svi"])["vrf"]), text(row["host"])
		if contexts[vrf] == nil {
			contexts[vrf] = map[string][]vxlan.Record{}
		}
		contexts[vrf][host] = append(contexts[vrf][host], row)
		sameVNI[host] = true
	}
	return contexts, sameVNI
}
func matrixSegment(nodes []dto.Node, configs vxlan.Record, vni int, vrf string, context map[string][]vxlan.Record, sameVNI map[string]bool) dto.Segment {
	segment := dto.Segment{VRF: vrf, VNI: vni, State: "consistent", Cells: []dto.SegmentCell{}}
	fingerprints := map[string]bool{}
	for _, n := range nodes {
		if n.Role != "leaf_switch" {
			continue
		}
		cell, fingerprint := segmentCell(n, context[n.IP], record(configs[n.IP]), sameVNI[n.IP])
		if fingerprint != "" {
			fingerprints[fingerprint] = true
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
	return segment
}
func segmentCell(n dto.Node, matches []vxlan.Record, config vxlan.Record, sameVNI bool) (dto.SegmentCell, string) {
	cell := dto.SegmentCell{DeviceID: n.DeviceID, State: "missing", ACLIn: "unbound", ACLOut: "unbound", Configuration: map[string]any{}}
	if config == nil {
		cell.ACLIn = "unknown"
		cell.ACLOut = "unknown"
		cell.State = "unknown"
	}
	if len(matches) == 0 && sameVNI {
		cell.State = "conflict"
	}
	fingerprint := ""
	if len(matches) == 1 {
		fingerprint = configureSegmentCell(&cell, matches[0], config)
	} else if len(matches) > 1 {
		cell.State = "conflict"
	}

	if n.ConfigState == "historical" || n.ConfigState == "unknown" {
		cell.State = "unknown"
	}
	return cell, fingerprint
}
func configureSegmentCell(cell *dto.SegmentCell, r, config vxlan.Record) string {
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
	signature := vxlan.Record{"vlan": r["vlan"], "svi": svi, "nve": vxlan.EffectiveNVEMember(config, record(r["nve"])), "tenant": vxlan.ServiceTenant(config, text(svi["vrf"]))}
	policies := vxlan.Record{}
	for _, direction := range []string{"in", "out"} {
		name := text(bindings[direction])
		if name != "" {
			policy := vxlan.ResolveACLPolicy(config, record(config["acls"])[name])
			policies[direction] = policy
			if policy == nil {
				cell.State = "unknown"
			}
		}
	}
	signature["policies"] = policies
	return vxlan.Fingerprint(signature)
}
