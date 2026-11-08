package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/flow"
	"go-admin/app/firewall/types"
)

type contextKey struct {
	scope, routing string
	domain         int
}
type zoneSpec struct {
	binding types.Binding
	zone    *fm.Zone
}

// Coverage expands selected protected zones from a fixed copy of connected
// evidence. A static route can never bootstrap another route's next-hop match.
func Coverage(ctx context.Context, target types.Target) ([]types.Coverage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if target.Firewall == nil {
		return missingCoverage(ctx, target)
	}
	fw := target.Firewall
	zones := fw.Zones()
	routes := fw.Routes()
	scans := 0
	if len(zones) > MaxRegions || len(target.Settings.Zones) > MaxRegions || len(target.Settings.Scopes) > MaxRegions {
		return nil, ErrBudget
	}
	specs := []zoneSpec{}
	for _, b := range target.Settings.Zones {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var found *fm.Zone
		for _, z := range zones {
			if (string(z.ID()) == b.Zone || z.Name() == b.Zone) && z.Scope() == b.Scope {
				if found != nil {
					found = nil
					break
				}
				found = z
			}
		}
		if found != nil && b.RoutingScope == "" {
			routing := map[string]bool{}
			for _, iface := range found.Interfaces() {
				routing[iface.Scope()] = true
			}
			if len(routing) > 0 {
				keys := make([]string, 0, len(routing))
				for vrf := range routing {
					keys = append(keys, vrf)
				}
				slices.Sort(keys)
				for _, vrf := range keys {
					copy := b
					copy.RoutingScope = vrf
					specs = append(specs, zoneSpec{binding: copy, zone: found})
					if len(specs) > MaxRegions {
						return nil, ErrBudget
					}
				}
				continue
			}
		}
		specs = append(specs, zoneSpec{binding: b, zone: found})
	}
	// Native networks remain visible even before a zone is marked protected.
	for _, z := range zones {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		matched := false
		for _, s := range specs {
			if s.zone == z {
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		routing := map[string]bool{}
		for _, i := range z.Interfaces() {
			routing[i.Scope()] = true
		}
		if len(routing) == 0 {
			routing[""] = true
		}
		for vrf := range routing {
			specs = append(specs, zoneSpec{zone: z, binding: types.Binding{Zone: string(z.ID()), Scope: z.Scope(), RoutingScope: vrf}})
		}
		if len(specs) > MaxRegions {
			return nil, ErrBudget
		}
	}
	out := []types.Coverage{}
	seen := map[string]bool{}
	add := func(s zoneSpec, network, origin string, id fm.ID, known bool) error {
		b := s.binding
		domain := b.DomainID
		if domain == 0 {
			domain = target.DomainID
			mapped := false
			for _, m := range target.Settings.Scopes {
				if m.RoutingScope == b.RoutingScope {
					if mapped && domain != m.DomainID {
						known = false
					}
					domain = m.DomainID
					mapped = true
				}
			}
		}
		zone := b.Zone
		if s.zone != nil {
			zone = string(s.zone.ID())
		}
		key := fmt.Sprintf("%d|%s|%s|%s|%s|%s|%s|%t|%t", domain, b.Scope, b.RoutingScope, zone, network, origin, id, b.Protected, known)
		if seen[key] {
			return nil
		}
		seen[key] = true
		if len(out) >= MaxRegions {
			return ErrBudget
		}
		version := target.BindingsVersion
		if version == 0 {
			version = target.Settings.Version
		}
		out = append(out, types.Coverage{InstanceID: target.InstanceID, DomainID: domain, Revision: target.Revision, BindingsVersion: version, Zone: zone, Scope: b.Scope, RoutingScope: b.RoutingScope, Network: network, Protected: b.Protected, Origin: origin, ObjectID: string(id), Known: known})
		return nil
	}
	graphKnown := true
	for _, d := range fw.Diagnostics() {
		if d.Object == "" {
			graphKnown = false
		}
	}
	for _, s := range specs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b := s.binding
		zoneKnown := s.zone != nil && graphKnown
		if s.zone != nil && s.zone.HasUnsupported() {
			zoneKnown = false
		}
		var original flow.AddressSet
		ifaceIDs := map[fm.ID]bool{}
		if s.zone != nil {
			for _, r := range s.zone.Record().Networks {
				set, e := flow.NewAddressSet(r)
				if e != nil {
					continue
				}
				original = original.Union(set)
				if err := add(s, rangeString(r), "native_zone", s.zone.ID(), zoneKnown); err != nil {
					return nil, err
				}
			}
			for _, iface := range s.zone.Interfaces() {
				if iface.Scope() != b.RoutingScope {
					continue
				}
				known := zoneKnown && !iface.Disabled() && !iface.HasUnsupported()
				ifaceIDs[iface.ID()] = known
				for _, p := range iface.Record().Addresses {
					r, e := flow.PrefixRange(p)
					if e != nil {
						continue
					}
					if known {
						set, _ := flow.NewAddressSet(r)
						original = original.Union(set)
					}
					if err := add(s, p.Masked().String(), "connected", iface.ID(), known); err != nil {
						return nil, err
					}
				}
			}
		}
		for _, token := range b.Networks {
			_, ok := addressRange(strings.TrimSpace(token))
			if err := add(s, token, "manual", "", zoneKnown && ok); err != nil {
				return nil, err
			}
		}
		if s.zone == nil {
			if err := add(s, "", "missing_zone", "", false); err != nil {
				return nil, err
			}
			continue
		}
		if target.Mode == "transparent" || !b.Protected {
			continue
		}
		for _, route := range routes {
			scans++
			if scans > 100000 {
				return nil, ErrBudget
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			r := route.Record()
			if r.Scope != b.RoutingScope || !r.Destination.IsValid() || r.Destination.Bits() == 0 || len(r.PolicyMatch) > 0 {
				continue
			}
			// Both attached interface and next-hop must agree when both are present.
			attached := r.Interface != "" && ifaceIDs[r.Interface]
			hop := r.NextHop.IsValid() && original.ContainsAddr(r.NextHop)
			if r.NextHop.IsValid() {
				if !hop || r.Interface != "" && !attached {
					continue
				}
			} else if !attached {
				continue
			}
			if err := add(s, r.Destination.Masked().String(), "static_route", r.ID, zoneKnown && !route.HasUnsupported()); err != nil {
				return nil, err
			}
		}
	}
	return out, ctx.Err()
}

// A created instance can legitimately have no imported revision. Its absence is
// evidence to retain, rather than a malformed graph that aborts other targets.
func missingCoverage(ctx context.Context, target types.Target) ([]types.Coverage, error) {
	if len(target.Settings.Zones) > MaxRegions || len(target.Settings.Scopes) > MaxRegions {
		return nil, ErrBudget
	}
	version := target.BindingsVersion
	if version == 0 {
		version = target.Settings.Version
	}
	marker := types.Coverage{InstanceID: target.InstanceID, DomainID: target.DomainID, Revision: target.Revision, BindingsVersion: version, Origin: "missing_snapshot", Known: false}
	out := []types.Coverage{marker}
	for _, mapping := range target.Settings.Scopes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c := marker
		c.DomainID = mapping.DomainID
		c.RoutingScope = mapping.RoutingScope
		out = append(out, c)
	}
	for _, binding := range target.Settings.Zones {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c := marker
		c.Zone = binding.Zone
		c.Scope = binding.Scope
		c.RoutingScope = binding.RoutingScope
		c.Protected = binding.Protected
		if binding.DomainID != 0 {
			c.DomainID = binding.DomainID
		} else {
			for _, mapping := range target.Settings.Scopes {
				if mapping.RoutingScope == binding.RoutingScope {
					c.DomainID = mapping.DomainID
				}
			}
		}
		if len(binding.Networks) == 0 {
			out = append(out, c)
		} else {
			for _, network := range binding.Networks {
				c.Network = network
				out = append(out, c)
				if len(out) > MaxRegions {
					return nil, ErrBudget
				}
			}
		}
		if len(out) > MaxRegions {
			return nil, ErrBudget
		}
	}
	if len(out) > MaxRegions {
		return nil, ErrBudget
	}
	return out, ctx.Err()
}
