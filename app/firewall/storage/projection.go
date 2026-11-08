package storage

import (
	"encoding/hex"
	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/flow"
	"go-admin/app/firewall/models"
	"gorm.io/gorm"
	"net/netip"
)

func addressKey(a netip.Addr) string {
	if a.Is4() {
		v := a.As4()
		return hex.EncodeToString(v[:])
	}
	v := a.As16()
	return hex.EncodeToString(v[:])
}
func project(tx *gorm.DB, snap *models.Snapshot, fw *fm.Firewall) error {
	d := fw.Export()
	ids := map[string]map[fm.ID]int{}
	catalogs := nativeCatalogs(d)
	native := func(kind string, o fm.Object, pos int, disabled bool) models.Native {
		state := "known"
		if disabled {
			state = "disabled"
		}
		ds := []models.Diagnostic{}
		for _, v := range snap.Diagnostics {
			if v.Object == string(o.ID) && objectOwnsDiagnostic(kind, o, v, catalogs) {
				ds = append(ds, v)
				state = "unknown"
			}
		}
		return models.Native{SnapshotID: snap.ID, NativeKey: key(o.Scope, string(o.ID)), NativeID: string(o.ID), Name: o.Name, Scope: o.Scope, Position: pos, State: state, Diagnostics: ds}
	}
	add := func(kind string, n *models.Native, row any) error {
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		if ids[kind] == nil {
			ids[kind] = map[fm.ID]int{}
		}
		ids[kind][fm.ID(n.NativeID)] = n.ID
		return nil
	}
	for i, r := range d.Interfaces {
		_ = i
		row := models.Interface{Native: native("interface", r.Object, i, r.Disabled), Record: r}
		if err := add("interface", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.Routes {
		_ = i
		row := models.Route{Native: native("route", r.Object, i, false), Record: r}
		if err := add("route", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.Zones {
		_ = i
		row := models.Zone{Native: native("zone", r.Object, i, false), Record: r}
		if err := add("zone", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.Addresses {
		_ = i
		row := models.Address{Native: native("address", r.Object, i, false), Record: r}
		if err := add("address", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.AddressGroups {
		_ = i
		row := models.AddressGroup{Native: native("address_group", r.Object, i, false), Record: r}
		if err := add("address_group", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.Services {
		_ = i
		row := models.Service{Native: native("service", r.Object, i, false), Record: r}
		if err := add("service", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.ServiceGroups {
		_ = i
		row := models.ServiceGroup{Native: native("service_group", r.Object, i, false), Record: r}
		if err := add("service_group", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.Schedules {
		_ = i
		row := models.Schedule{Native: native("schedule", r.Object, i, false), Record: r}
		if err := add("schedule", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.Rules {
		_ = i
		row := models.Rule{Native: native("rule", r.Object, r.Position, r.Disabled), Record: r}
		if err := add("rule", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.RuleBindings {
		_ = i
		row := models.RuleBinding{Native: native("rule_binding", r.Object, i, false), Record: r}
		if err := add("rule_binding", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.StaticNATs {
		_ = i
		row := models.StaticNAT{Native: native("static_nat", r.Object, r.Order.Position, r.Disabled), Record: r}
		if err := add("static_nat", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.DynamicNATs {
		_ = i
		row := models.DynamicNAT{Native: native("dynamic_nat", r.Object, r.Order.Position, r.Disabled), Record: r}
		if err := add("dynamic_nat", &row.Native, &row); err != nil {
			return err
		}
	}
	for i, r := range d.NATPools {
		_ = i
		row := models.NATPool{Native: native("nat_pool", r.Object, i, false), Record: r}
		if err := add("nat_pool", &row.Native, &row); err != nil {
			return err
		}
	}

	ref := func(sk string, sid fm.ID, tk string, tid fm.ID, role string, pos int) error {
		if tid == "" {
			return nil
		}
		target := ids[tk][tid]
		return tx.Create(&models.Reference{SnapshotID: snap.ID, SourceKind: sk, SourceID: ids[sk][sid], TargetKind: tk, TargetID: target, TargetNativeID: string(tid), Role: role, Position: pos, Resolved: target != 0}).Error
	}
	addressKind := func(k fm.Kind) string {
		switch k {
		case fm.AddressObject:
			return "address"
		case fm.AddressGroupObject:
			return "address_group"
		}
		return "unknown"
	}
	serviceKind := func(k fm.Kind) string {
		switch k {
		case fm.ServiceObject:
			return "service"
		case fm.ServiceGroupObject:
			return "service_group"
		}
		return "unknown"
	}
	natKind := func(k fm.Kind) string {
		switch k {
		case fm.StaticNATObject:
			return "static_nat"
		case fm.DynamicNATObject:
			return "dynamic_nat"
		}
		return "unknown"
	}
	match := func(kind string, id fm.ID, m fm.Match) error {
		for i, v := range m.Sources {
			if err := ref(kind, id, addressKind(v.Kind), v.ID, "source", i); err != nil {
				return err
			}
		}
		for i, v := range m.Destinations {
			if err := ref(kind, id, addressKind(v.Kind), v.ID, "destination", i); err != nil {
				return err
			}
		}
		for i, v := range m.Services {
			if err := ref(kind, id, serviceKind(v.Kind), v.ID, "service", i); err != nil {
				return err
			}
		}
		for i, v := range m.FromZones {
			if err := ref(kind, id, "zone", v, "from_zone", i); err != nil {
				return err
			}
		}
		for i, v := range m.ToZones {
			if err := ref(kind, id, "zone", v, "to_zone", i); err != nil {
				return err
			}
		}
		return ref(kind, id, "schedule", m.Schedule, "schedule", 0)
	}
	ranges := func(kind string, id fm.ID, rs []flow.AddressRange) error {
		for i, r := range rs {
			if !r.Valid() {
				continue
			}
			row := models.AddressRange{SnapshotID: snap.ID, ObjectKind: kind, ObjectID: ids[kind][id], Position: i, Family: r.First.BitLen(), First: addressKey(r.First), Last: addressKey(r.Last)}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	}
	for _, r := range d.Addresses {
		if err := ranges("address", r.ID, r.Ranges); err != nil {
			return err
		}
		if err := ref("address", r.ID, natKind(r.VIP.Kind), r.VIP.ID, "vip", 0); err != nil {
			return err
		}
	}
	for _, r := range d.Zones {
		if err := ranges("zone", r.ID, r.Networks); err != nil {
			return err
		}
	}
	for _, r := range d.Interfaces {
		if err := ref("interface", r.ID, "zone", r.Zone, "zone", 0); err != nil {
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
		if err := ranges("interface", r.ID, networks); err != nil {
			return err
		}
	}
	for _, r := range d.Routes {
		if err := ref("route", r.ID, "interface", r.Interface, "interface", 0); err != nil {
			return err
		}
		rg, e := flow.PrefixRange(r.Destination)
		if e == nil {
			if e = ranges("route", r.ID, []flow.AddressRange{rg}); e != nil {
				return e
			}
		}
	}
	for _, r := range d.AddressGroups {
		for i, v := range r.Members {
			if err := ref("address_group", r.ID, addressKind(v.Kind), v.ID, "member", i); err != nil {
				return err
			}
		}
	}
	for _, r := range d.ServiceGroups {
		for i, v := range r.Members {
			if err := ref("service_group", r.ID, serviceKind(v.Kind), v.ID, "member", i); err != nil {
				return err
			}
		}
	}
	for _, r := range d.Services {
		for i, v := range r.Terms {
			row := models.ServiceTerm{SnapshotID: snap.ID, ObjectID: ids["service"][r.ID], Position: i, ProtocolLow: v.Protocol.Low, ProtocolHigh: v.Protocol.High, SourceLow: v.SourcePort.Low, SourceHigh: v.SourcePort.High, DestinationLow: v.DestinationPort.Low, DestinationHigh: v.DestinationPort.High, ICMPTypeLow: v.ICMPType.Low, ICMPTypeHigh: v.ICMPType.High, ICMPCodeLow: v.ICMPCode.Low, ICMPCodeHigh: v.ICMPCode.High}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
	}
	for _, r := range d.Rules {
		if err := match("rule", r.ID, r.Match); err != nil {
			return err
		}
	}
	for _, r := range d.RuleBindings {
		if err := ref("rule_binding", r.ID, "interface", r.Interface, "interface", 0); err != nil {
			return err
		}
		if err := ref("rule_binding", r.ID, "zone", r.Zone, "zone", 0); err != nil {
			return err
		}
		for i, rule := range d.Rules {
			if rule.Scope == r.Scope && rule.RuleSet == r.RuleSet {
				if err := ref("rule_binding", r.ID, "rule", rule.ID, "rule_set", i); err != nil {
					return err
				}
			}
		}
	}
	for _, r := range d.StaticNATs {
		var mappingRanges []flow.AddressRange
		for _, mapping := range append(append([]fm.Mapping{}, r.Source...), r.Destination...) {
			mappingRanges = append(mappingRanges, mapping.Original, mapping.Translated)
		}
		if err := ranges("static_nat", r.ID, mappingRanges); err != nil {
			return err
		}
		if err := match("static_nat", r.ID, r.Match); err != nil {
			return err
		}
		if err := ref("static_nat", r.ID, "interface", r.Interface, "interface", 0); err != nil {
			return err
		}
		if err := ref("static_nat", r.ID, addressKind(r.SourceVIP.Kind), r.SourceVIP.ID, "source_vip", 0); err != nil {
			return err
		}
	}
	for _, r := range d.DynamicNATs {
		var mappingRanges []flow.AddressRange
		for _, mapping := range r.Destination {
			mappingRanges = append(mappingRanges, mapping.Original, mapping.Translated)
		}
		if err := ranges("dynamic_nat", r.ID, mappingRanges); err != nil {
			return err
		}
		if err := match("dynamic_nat", r.ID, r.Match); err != nil {
			return err
		}
		for _, v := range []struct {
			kind string
			id   fm.ID
			role string
		}{{"rule", r.Rule, "rule"}, {"nat_pool", r.SourcePool, "source_pool"}, {"nat_pool", r.DestinationPool, "destination_pool"}, {"interface", r.Interface, "interface"}} {
			if err := ref("dynamic_nat", r.ID, v.kind, v.id, v.role, 0); err != nil {
				return err
			}
		}
	}
	for _, r := range d.NATPools {
		if err := ranges("nat_pool", r.ID, r.Ranges); err != nil {
			return err
		}
		if err := ref("nat_pool", r.ID, "interface", r.Interface, "interface", 0); err != nil {
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
		for _, reason := range o.Unsupported {
			if reason == d.Message {
				return true
			}
		}
		return false
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
		if d.Message == "cyclic address group" {
			kinds = []string{"address_group"}
		} else if d.Message == "cyclic service group" {
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
