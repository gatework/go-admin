package engine

import (
	"context"
	"fmt"
	"slices"
	"strings"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/flow"
	"github.com/gatework/motor/app/net/models/firewall/types"
)

type contextKey struct {
	scope, routing string
	domain         int
}
type zoneSpec struct {
	binding types.Binding
	zone    *fm.Zone
}

type coverageBuilder struct {
	target types.Target
	out    []types.Coverage
	seen   map[string]bool
	scans  int
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
	zones := target.Firewall.Zones()
	if len(zones) > MaxRegions || len(target.Settings.Zones) > MaxRegions || len(target.Settings.Scopes) > MaxRegions {
		return nil, BudgetErr
	}
	specs, err := selectedCoverageZones(ctx, target, zones)
	if err != nil {
		return nil, err
	}
	specs, err = appendNativeCoverageZones(ctx, zones, specs)
	if err != nil {
		return nil, err
	}
	c := coverageBuilder{target: target, out: []types.Coverage{}, seen: map[string]bool{}}
	graphKnown := true
	for _, d := range target.Firewall.Diagnostics() {
		if d.Object == "" {
			graphKnown = false
		}
	}
	for _, spec := range specs {
		if err := c.expand(ctx, spec, graphKnown); err != nil {
			return nil, err
		}
	}
	return c.out, ctx.Err()
}
func selectedCoverageZones(ctx context.Context, target types.Target, zones []*fm.Zone) ([]zoneSpec, error) {
	specs := []zoneSpec{}
	for _, b := range target.Settings.Zones {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		found := matchingCoverageZone(zones, b)
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
					binding := b
					binding.RoutingScope = vrf
					specs = append(specs, zoneSpec{binding: binding, zone: found})
					if len(specs) > MaxRegions {
						return nil, BudgetErr
					}
				}
				continue
			}
		}
		specs = append(specs, zoneSpec{binding: b, zone: found})
	}

	return specs, nil
}
func matchingCoverageZone(zones []*fm.Zone, b types.Binding) *fm.Zone {
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
	return found
}

func appendNativeCoverageZones(ctx context.Context, zones []*fm.Zone, specs []zoneSpec) ([]zoneSpec, error) {
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
			return nil, BudgetErr
		}
	}

	return specs, nil
}
func (c *coverageBuilder) add(s zoneSpec, network, origin string, id fm.ID, known bool) error {
	b := s.binding
	domain := b.DomainID
	if domain == 0 {
		domain = c.target.DomainID
		mapped := false
		for _, m := range c.target.Settings.Scopes {
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
	if c.seen[key] {
		return nil
	}
	c.seen[key] = true
	if len(c.out) >= MaxRegions {
		return BudgetErr
	}
	version := c.target.BindingsVersion
	if version == 0 {
		version = c.target.Settings.Version
	}
	c.out = append(c.out, types.Coverage{InstanceID: c.target.InstanceID, DomainID: domain, Revision: c.target.Revision, BindingsVersion: version, Zone: zone, Scope: b.Scope, RoutingScope: b.RoutingScope, Network: network, Protected: b.Protected, Origin: origin, ObjectID: string(id), Known: known})
	return nil
}

func (c *coverageBuilder) connected(s zoneSpec, zoneKnown bool) (flow.AddressSet, map[fm.ID]bool, error) {
	b := s.binding
	var original flow.AddressSet
	ifaceIDs := map[fm.ID]bool{}
	if s.zone != nil {
		for _, r := range s.zone.Record().Networks {
			set, e := flow.NewAddressSet(r)
			if e != nil {
				continue
			}
			original = original.Union(set)
			if err := c.add(s, rangeString(r), "native_zone", s.zone.ID(), zoneKnown); err != nil {
				return original, ifaceIDs, err
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
					original = original.Union(rangeSet(r))
				}
				if err := c.add(s, p.Masked().String(), "connected", iface.ID(), known); err != nil {
					return original, ifaceIDs, err
				}
			}
		}
	}

	return original, ifaceIDs, nil
}
func (c *coverageBuilder) expand(ctx context.Context, s zoneSpec, graphKnown bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	b := s.binding
	zoneKnown := s.zone != nil && graphKnown
	if s.zone != nil && s.zone.HasUnsupported() {
		zoneKnown = false
	}
	original, ifaceIDs, err := c.connected(s, zoneKnown)
	if err != nil {
		return err
	}
	for _, token := range b.Networks {
		_, ok := addressRange(strings.TrimSpace(token))
		if err := c.add(s, token, "manual", "", zoneKnown && ok); err != nil {
			return err
		}
	}
	if s.zone == nil {
		if err := c.add(s, "", "missing_zone", "", false); err != nil {
			return err
		}
		return nil
	}
	if c.target.Mode == "transparent" || !b.Protected {
		return nil
	}
	return c.staticRoutes(ctx, s, zoneKnown, original, ifaceIDs)
}

func (c *coverageBuilder) staticRoutes(ctx context.Context, s zoneSpec, zoneKnown bool, original flow.AddressSet, ifaceIDs map[fm.ID]bool) error {
	b := s.binding
	for _, route := range c.target.Firewall.Routes() {
		c.scans++
		if c.scans > 100000 {
			return BudgetErr
		}
		if err := ctx.Err(); err != nil {
			return err
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
		if err := c.add(s, r.Destination.Masked().String(), "static_route", r.ID, zoneKnown && !route.HasUnsupported()); err != nil {
			return err
		}
	}
	return nil
}

// A created instance can legitimately have no imported revision. Its absence is
// evidence to retain, rather than a malformed graph that aborts other targets.
func missingCoverage(ctx context.Context, target types.Target) ([]types.Coverage, error) {
	if len(target.Settings.Zones) > MaxRegions || len(target.Settings.Scopes) > MaxRegions {
		return nil, BudgetErr
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
		c.DomainID = missingCoverageDomain(target, binding)
		if len(binding.Networks) == 0 {
			out = append(out, c)
		} else {
			for _, network := range binding.Networks {
				c.Network = network
				out = append(out, c)
				if len(out) > MaxRegions {
					return nil, BudgetErr
				}
			}
		}
		if len(out) > MaxRegions {
			return nil, BudgetErr
		}
	}
	if len(out) > MaxRegions {
		return nil, BudgetErr
	}
	return out, ctx.Err()
}

func missingCoverageDomain(target types.Target, binding types.Binding) int {
	domain := target.DomainID
	if binding.DomainID != 0 {
		domain = binding.DomainID
	} else {
		for _, mapping := range target.Settings.Scopes {
			if mapping.RoutingScope == binding.RoutingScope {
				domain = mapping.DomainID
			}
		}
	}
	return domain
}
