package storage

import (
	"encoding/hex"
	"net/netip"
	"slices"

	"gorm.io/gorm"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/flow"
	models "github.com/gatework/motor/app/net/models/firewall"
)

func addressKey(a netip.Addr) string {
	if a.Is4() {
		v := a.As4()
		return hex.EncodeToString(v[:])
	}
	v := a.As16()
	return hex.EncodeToString(v[:])
}

type nativeProjection struct {
	tx       *gorm.DB
	snapshot *models.Snapshot
	ids      map[string]map[fm.ID]int
	catalogs map[fm.ID]map[string]fm.Object
}

func project(tx *gorm.DB, snapshot *models.Snapshot, firewall *fm.Firewall) error {
	data := firewall.Export()
	p := nativeProjection{tx: tx, snapshot: snapshot, ids: map[string]map[fm.ID]int{}, catalogs: nativeCatalogs(data)}
	for _, step := range []func(fm.Data) error{p.topology, p.addresses, p.policy, p.translation, p.addressRelations, p.topologyRelations, p.groupRelations, p.policyRelations, p.translationRelations} {
		if err := step(data); err != nil {
			return err
		}
	}
	return nil
}

func (p *nativeProjection) native(kind string, o fm.Object, pos int, disabled bool) models.Native {
	state := "known"
	if disabled {
		state = "disabled"
	}
	ds := []models.Diagnostic{}
	for _, v := range p.snapshot.Diagnostics {
		if v.Object == string(o.ID) && objectOwnsDiagnostic(kind, o, v, p.catalogs) {
			ds = append(ds, v)
			state = "unknown"
		}
	}
	return models.Native{SnapshotID: p.snapshot.ID, NativeKey: key(o.Scope, string(o.ID)), NativeID: string(o.ID), Name: o.Name, Scope: o.Scope, Position: pos, State: state, Diagnostics: ds}

}

func (p *nativeProjection) add(kind string, n *models.Native, row any) error {
	if err := p.tx.Create(row).Error; err != nil {
		return err
	}
	if p.ids[kind] == nil {
		p.ids[kind] = map[fm.ID]int{}
	}
	p.ids[kind][fm.ID(n.NativeID)] = n.ID
	return nil

}

func (p *nativeProjection) reference(sk string, sid fm.ID, tk string, tid fm.ID, role string, pos int) error {
	if tid == "" {
		return nil
	}
	target := p.ids[tk][tid]
	return p.tx.Create(&models.Reference{SnapshotID: p.snapshot.ID, SourceKind: sk, SourceID: p.ids[sk][sid], TargetKind: tk, TargetID: target, TargetNativeID: string(tid), Role: role, Position: pos, Resolved: target != 0}).Error

}

func (p *nativeProjection) match(kind string, id fm.ID, m fm.Match) error {
	for i, v := range m.Sources {
		if err := p.reference(kind, id, addressKind(v.Kind), v.ID, "source", i); err != nil {
			return err
		}
	}
	for i, v := range m.Destinations {
		if err := p.reference(kind, id, addressKind(v.Kind), v.ID, "destination", i); err != nil {
			return err
		}
	}
	for i, v := range m.Services {
		if err := p.reference(kind, id, serviceKind(v.Kind), v.ID, "service", i); err != nil {
			return err
		}
	}
	for i, v := range m.FromZones {
		if err := p.reference(kind, id, "zone", v, "from_zone", i); err != nil {
			return err
		}
	}
	for i, v := range m.ToZones {
		if err := p.reference(kind, id, "zone", v, "to_zone", i); err != nil {
			return err
		}
	}
	return p.reference(kind, id, "schedule", m.Schedule, "schedule", 0)

}

func (p *nativeProjection) ranges(kind string, id fm.ID, rs []flow.AddressRange) error {
	for i, r := range rs {
		if !r.Valid() {
			continue
		}
		row := models.AddressRange{SnapshotID: p.snapshot.ID, ObjectKind: kind, ObjectID: p.ids[kind][id], Position: i, Family: r.First.BitLen(), First: addressKey(r.First), Last: addressKey(r.Last)}
		if err := p.tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil

}

func addressKind(k fm.Kind) string {
	switch k {
	case fm.AddressObject:
		return "address"
	case fm.AddressGroupObject:
		return "address_group"
	default:
		return "unknown"
	}
}

func serviceKind(k fm.Kind) string {
	switch k {
	case fm.ServiceObject:
		return "service"
	case fm.ServiceGroupObject:
		return "service_group"
	default:
		return "unknown"
	}
}

func natKind(k fm.Kind) string {
	switch k {
	case fm.StaticNATObject:
		return "static_nat"
	case fm.DynamicNATObject:
		return "dynamic_nat"
	default:
		return "unknown"
	}
}

func (p *nativeProjection) topology(d fm.Data) error {
	for i, r := range d.Interfaces {
		row := models.Interface{Native: p.native("interface", r.Object, i, r.Disabled), Record: r}
		if err := p.add("interface", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.Routes {
		row := models.Route{Native: p.native("route", r.Object, i, false), Record: r}
		if err := p.add("route", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.Zones {
		row := models.Zone{Native: p.native("zone", r.Object, i, false), Record: r}
		if err := p.add("zone", &row.Native, &row); err != nil {
			return err
		}
	}
	return nil
}

func (p *nativeProjection) addresses(d fm.Data) error {
	for i, r := range d.Addresses {
		row := models.Address{Native: p.native("address", r.Object, i, false), Record: r}
		if err := p.add("address", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.AddressGroups {
		row := models.AddressGroup{Native: p.native("address_group", r.Object, i, false), Record: r}
		if err := p.add("address_group", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.Services {
		row := models.Service{Native: p.native("service", r.Object, i, false), Record: r}
		if err := p.add("service", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.ServiceGroups {
		row := models.ServiceGroup{Native: p.native("service_group", r.Object, i, false), Record: r}
		if err := p.add("service_group", &row.Native, &row); err != nil {
			return err
		}
	}
	return nil
}

func (p *nativeProjection) policy(d fm.Data) error {
	for i, r := range d.Schedules {
		row := models.Schedule{Native: p.native("schedule", r.Object, i, false), Record: r}
		if err := p.add("schedule", &row.Native, &row); err != nil {
			return err
		}
	}
	for _, r := range d.Rules {
		row := models.Rule{Native: p.native("rule", r.Object, r.Position, r.Disabled), Record: r}
		if err := p.add("rule", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.RuleBindings {
		row := models.RuleBinding{Native: p.native("rule_binding", r.Object, i, false), Record: r}
		if err := p.add("rule_binding", &row.Native, &row); err != nil {
			return err
		}
	}
	return nil
}

func (p *nativeProjection) translation(d fm.Data) error {
	for _, r := range d.StaticNATs {
		row := models.StaticNAT{Native: p.native("static_nat", r.Object, r.Order.Position, r.Disabled), Record: r}
		if err := p.add("static_nat", &row.Native, &row); err != nil {
			return err
		}
	}
	for _, r := range d.DynamicNATs {
		row := models.DynamicNAT{Native: p.native("dynamic_nat", r.Object, r.Order.Position, r.Disabled), Record: r}
		if err := p.add("dynamic_nat", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.NATPools {
		row := models.NATPool{Native: p.native("nat_pool", r.Object, i, false), Record: r}
		if err := p.add("nat_pool", &row.Native, &row); err != nil {
			return err
		}
	}
	return nil
}

func (p *nativeProjection) addressRelations(d fm.Data) error {
	for _, r := range d.Addresses {
		if err := p.ranges("address", r.ID, r.Ranges); err != nil {
			return err
		}
		if err := p.reference("address", r.ID, natKind(r.VIP.Kind), r.VIP.ID, "vip", 0); err != nil {
			return err
		}
	}
	for _, r := range d.Zones {
		if err := p.ranges("zone", r.ID, r.Networks); err != nil {
			return err
		}
	}
	return nil
}

func (p *nativeProjection) topologyRelations(d fm.Data) error {
	for _, r := range d.Interfaces {
		if err := p.reference("interface", r.ID, "zone", r.Zone, "zone", 0); err != nil {
			return err
		}
		var networks []flow.AddressRange
		for _, p := range r.Addresses {
			rg, err := flow.PrefixRange(p)
			if err != nil {
				return err
			}
			networks = append(networks, rg)
		}
		if err := p.ranges("interface", r.ID, networks); err != nil {
			return err
		}
	}
	for _, r := range d.Routes {
		if err := p.reference("route", r.ID, "interface", r.Interface, "interface", 0); err != nil {
			return err
		}
		rg, e := flow.PrefixRange(r.Destination)
		if e == nil {
			if e = p.ranges("route", r.ID, []flow.AddressRange{rg}); e != nil {
				return e
			}
		}
	}
	return nil
}

func (p *nativeProjection) groupRelations(d fm.Data) error {
	for _, r := range d.AddressGroups {
		for i, v := range r.Members {
			if err := p.reference("address_group", r.ID, addressKind(v.Kind), v.ID, "member", i); err != nil {
				return err
			}
		}
	}
	for _, r := range d.ServiceGroups {
		for i, v := range r.Members {
			if err := p.reference("service_group", r.ID, serviceKind(v.Kind), v.ID, "member", i); err != nil {
				return err
			}
		}
	}
	for _, r := range d.Services {
		for i, v := range r.Terms {
			row := models.ServiceTerm{SnapshotID: p.snapshot.ID, ObjectID: p.ids["service"][r.ID], Position: i, ProtocolLow: v.Protocol.Low, ProtocolHigh: v.Protocol.High, SourceLow: v.SourcePort.Low, SourceHigh: v.SourcePort.High, DestinationLow: v.DestinationPort.Low, DestinationHigh: v.DestinationPort.High, ICMPTypeLow: v.ICMPType.Low, ICMPTypeHigh: v.ICMPType.High, ICMPCodeLow: v.ICMPCode.Low, ICMPCodeHigh: v.ICMPCode.High}
			if err := p.tx.Create(&row).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *nativeProjection) policyRelations(d fm.Data) error {
	for _, r := range d.Rules {
		if err := p.match("rule", r.ID, r.Match); err != nil {
			return err
		}
	}
	for _, r := range d.RuleBindings {
		if err := p.reference("rule_binding", r.ID, "interface", r.Interface, "interface", 0); err != nil {
			return err
		}
		if err := p.reference("rule_binding", r.ID, "zone", r.Zone, "zone", 0); err != nil {
			return err
		}
		for i, rule := range d.Rules {
			if rule.Scope == r.Scope && rule.RuleSet == r.RuleSet {
				if err := p.reference("rule_binding", r.ID, "rule", rule.ID, "rule_set", i); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (p *nativeProjection) translationRelations(d fm.Data) error {
	for _, r := range d.StaticNATs {
		var mappingRanges []flow.AddressRange
		for _, mapping := range append(append([]fm.Mapping{}, r.Source...), r.Destination...) {
			mappingRanges = append(mappingRanges, mapping.Original, mapping.Translated)
		}
		if err := p.ranges("static_nat", r.ID, mappingRanges); err != nil {
			return err
		}
		if err := p.match("static_nat", r.ID, r.Match); err != nil {
			return err
		}
		if err := p.reference("static_nat", r.ID, "interface", r.Interface, "interface", 0); err != nil {
			return err
		}
		if err := p.reference("static_nat", r.ID, addressKind(r.SourceVIP.Kind), r.SourceVIP.ID, "source_vip", 0); err != nil {
			return err
		}
	}
	for _, r := range d.DynamicNATs {
		var mappingRanges []flow.AddressRange
		for _, mapping := range r.Destination {
			mappingRanges = append(mappingRanges, mapping.Original, mapping.Translated)
		}
		if err := p.ranges("dynamic_nat", r.ID, mappingRanges); err != nil {
			return err
		}
		if err := p.match("dynamic_nat", r.ID, r.Match); err != nil {
			return err
		}
		for _, v := range []struct {
			kind string
			id   fm.ID
			role string
		}{{"rule", r.Rule, "rule"}, {"nat_pool", r.SourcePool, "source_pool"}, {"nat_pool", r.DestinationPool, "destination_pool"}, {"interface", r.Interface, "interface"}} {
			if err := p.reference("dynamic_nat", r.ID, v.kind, v.id, v.role, 0); err != nil {
				return err
			}
		}
	}
	for _, r := range d.NATPools {
		if err := p.ranges("nat_pool", r.ID, r.Ranges); err != nil {
			return err
		}
		if err := p.reference("nat_pool", r.ID, "interface", r.Interface, "interface", 0); err != nil {
			return err
		}
	}
	return nil
}

func nativeCatalogs(d fm.Data) map[fm.ID]map[string]fm.Object {
	out := map[fm.ID]map[string]fm.Object{}
	add := func(kind string, o fm.Object) {
		if out[o.ID] == nil {
			out[o.ID] = map[string]fm.Object{}
		}
		out[o.ID][kind] = o
	}
	for _, r := range d.Interfaces {
		add("interface", r.Object)
	}
	for _, r := range d.Routes {
		add("route", r.Object)
	}
	for _, r := range d.Zones {
		add("zone", r.Object)
	}
	for _, r := range d.Addresses {
		add("address", r.Object)
	}
	for _, r := range d.AddressGroups {
		add("address_group", r.Object)
	}
	for _, r := range d.Services {
		add("service", r.Object)
	}
	for _, r := range d.ServiceGroups {
		add("service_group", r.Object)
	}
	for _, r := range d.Schedules {
		add("schedule", r.Object)
	}
	for _, r := range d.Rules {
		add("rule", r.Object)
	}
	for _, r := range d.RuleBindings {
		add("rule_binding", r.Object)
	}
	for _, r := range d.StaticNATs {
		add("static_nat", r.Object)
	}
	for _, r := range d.DynamicNATs {
		add("dynamic_nat", r.Object)
	}
	for _, r := range d.NATPools {
		add("nat_pool", r.Object)
	}
	return out
}

// Firemon diagnostics carry an untyped native ID. A shared ID is insufficient
// ownership evidence: retain ambiguous diagnostics only on the snapshot.
func objectOwnsDiagnostic(kind string, o fm.Object, d models.Diagnostic, catalogs map[fm.ID]map[string]fm.Object) bool {
	candidates := catalogs[o.ID]
	if d.Code == "unsupported" {
		return slices.Contains(o.Unsupported, d.Message)
	}
	if len(candidates) == 1 {
		return true
	}
	var kinds []string
	switch d.Code {
	case "unresolved-fqdn", "invalid-vip-reference":
		kinds = []string{"address"}
	case "recurring-schedule":
		kinds = []string{"schedule"}
	case "unknown-action":
		kinds = []string{"rule"}
	case "missing-binding":
		kinds = []string{"rule_binding"}
	case "unresolved-route":
		kinds = []string{"route"}
	case "invalid-source-vip-reference":
		kinds = []string{"static_nat"}
	case "missing-rule", "policy-nat-mismatch", "missing-pool", "unknown-translation", "missing-allocation", "invalid-interface-selection":
		kinds = []string{"dynamic_nat"}
	case "unknown-mapping", "unknown-port-mapping":
		kinds = []string{"static_nat", "dynamic_nat"}
	case "missing-interface":
		kinds = []string{"route", "nat_pool", "static_nat", "dynamic_nat"}
	case "missing-zone":
		kinds = []string{"interface", "rule", "static_nat", "dynamic_nat"}
	case "unsupported-match", "extended-match", "missing-schedule":
		kinds = []string{"rule", "static_nat", "dynamic_nat"}
	case "missing-address", "missing-address-group":
		kinds = []string{"address_group", "rule", "static_nat", "dynamic_nat"}
	case "missing-service", "missing-service-group":
		kinds = []string{"service_group", "rule", "static_nat", "dynamic_nat"}
	case "group-cycle":
		switch d.Message {
		case "cyclic address group":
			kinds = []string{"address_group"}
		case "cyclic service group":
			kinds = []string{"service_group"}
		}
	}
	owner := ""
	for _, candidate := range kinds {
		if _, exists := candidates[candidate]; exists {
			if owner != "" {
				return false
			}
			owner = candidate
		}
	}
	return owner == kind
}
