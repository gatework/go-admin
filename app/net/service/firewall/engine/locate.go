package engine

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/flow"
	"github.com/gatework/firemon/policy"
	"github.com/gatework/motor/app/net/models/firewall/types"
)

type endpoint struct {
	addresses        flow.AddressSet
	zone, iface      fm.ID
	known, protected bool
}
type located struct {
	target               types.Target
	row                  types.NormalizedRow
	request              policy.Request
	routing, side, basis string
	known                bool
	issues               []types.Notice
}

func coverageSet(c types.Coverage) flow.AddressSet {
	r, ok := addressRange(c.Network)
	if !ok {
		return flow.AddressSet{}
	}
	s, _ := flow.NewAddressSet(r)
	return s
}

type endpointEvidence struct {
	known, unknown flow.AddressSet
	iface          fm.ID
}

func (z *endpointEvidence) add(c types.Coverage, hit flow.AddressSet) {
	if c.Known {
		z.known = z.known.Union(hit)
	} else {
		z.unknown = z.unknown.Union(hit)
	}
	if c.Origin == "connected" {
		if z.iface != "" && z.iface != fm.ID(c.ObjectID) {
			z.iface = ""
		} else {
			z.iface = fm.ID(c.ObjectID)
		}
	}
}

func coverageEndpoints(ctx context.Context, coverage []types.Coverage, addresses flow.AddressSet, protectedOnly bool) ([]endpoint, error) {
	zones := map[fm.ID]*endpointEvidence{}
	for _, c := range coverage {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if protectedOnly && !c.Protected {
			continue
		}
		hit := addresses.Intersection(coverageSet(c))
		if hit.Empty() {
			continue
		}
		key := fm.ID(c.Zone)
		if zones[key] == nil {
			zones[key] = &endpointEvidence{}
		}
		zones[key].add(c, hit)
	}
	var ambiguous flow.AddressSet
	keys := make([]fm.ID, 0, len(zones))
	for k := range zones {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for i, a := range keys {
		for _, b := range keys[i+1:] {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			za, zb := zones[a], zones[b]
			ambiguous = ambiguous.Union(za.known.Union(za.unknown).Intersection(zb.known.Union(zb.unknown)))
		}
	}
	out := []endpoint{}
	for _, k := range keys {
		z := zones[k]
		known := z.known.Difference(ambiguous)
		unknown := z.unknown.Difference(z.known).Difference(ambiguous)
		if !known.Empty() {
			out = append(out, endpoint{addresses: known, zone: k, iface: z.iface, known: true, protected: protectedOnly})
		}
		if !unknown.Empty() {
			out = append(out, endpoint{addresses: unknown, zone: k, protected: protectedOnly})
		}
	}
	if !ambiguous.Empty() {
		out = append(out, endpoint{addresses: ambiguous, protected: protectedOnly})
	}
	return out, nil
}
func endpoints(ctx context.Context, t types.Target, key contextKey, coverage []types.Coverage, addresses flow.AddressSet) ([]endpoint, error) {
	protected, e := coverageEndpoints(ctx, coverage, addresses, true)
	if e != nil {
		return nil, e
	}
	var used flow.AddressSet
	for _, p := range protected {
		used = used.Union(p.addresses)
	}
	remaining := addresses.Difference(used)
	out := protected
	if !remaining.Empty() {
		if t.Mode == "transparent" {
			external, e := coverageEndpoints(ctx, coverage, remaining, false)
			if e != nil {
				return nil, e
			}
			for _, p := range external {
				out = append(out, p)
				remaining = remaining.Difference(p.addresses)
			}
			if !remaining.Empty() {
				out = append(out, endpoint{addresses: remaining})
			}
		} else {
			routed, err := routedEndpoints(ctx, t, key, remaining)
			if err != nil {
				return nil, err
			}
			out = append(out, routed...)
		}
	}
	if len(out) > MaxRegions {
		return nil, BudgetErr
	}
	return out, nil
}
func routedEndpoints(ctx context.Context, t types.Target, key contextKey, remaining flow.AddressSet) ([]endpoint, error) {
	var out []endpoint
	paths, e := nativeEngine().ResolveRoutes(ctx, t.Firewall, key.routing, remaining)
	if e != nil {
		return nil, normalizeError(e)
	}
	for _, p := range paths {
		ep := endpoint{addresses: p.Addresses, known: p.Known}
		if p.Zone != nil {
			ep.zone = p.Zone.ID()
			if p.Zone.Scope() != key.scope {
				ep.known = false
			}
		}
		if p.Interface != nil {
			ep.iface = p.Interface.ID()
		}
		out = append(out, ep)
	}
	return out, nil
}
func groupCoverage(c []types.Coverage, domain int) map[contextKey][]types.Coverage {
	out := map[contextKey][]types.Coverage{}
	for _, v := range c {
		if v.DomainID != domain {
			continue
		}
		key := contextKey{scope: v.Scope, routing: v.RoutingScope, domain: v.DomainID}
		out[key] = append(out[key], v)
	}
	return out
}
func sortedContexts(groups map[contextKey][]types.Coverage) []contextKey {
	keys := make([]contextKey, 0, len(groups))
	for k, c := range groups {
		protected := false
		for _, v := range c {
			protected = protected || v.Protected
		}
		if protected {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].scope != keys[j].scope {
			return keys[i].scope < keys[j].scope
		}
		return keys[i].routing < keys[j].routing
	})
	return keys
}
func locateTraffic(ctx context.Context, t types.Target, key contextKey, coverage []types.Coverage, row types.NormalizedRow, traffic flow.Traffic, at time.Time, c *calculation) ([]located, flow.Traffic, error) {
	b := locationBuilder{index: map[string]int{}}
	for _, box := range traffic.Regions() {
		srcSet, _ := flow.NewAddressSet(box.Source)
		dstSet, _ := flow.NewAddressSet(box.Destination)
		src, e := endpoints(ctx, t, key, coverage, srcSet)
		if e != nil {
			return nil, b.used, e
		}
		dst, e := endpoints(ctx, t, key, coverage, dstSet)
		if e != nil {
			return nil, b.used, e
		}
		for _, s := range src {
			for _, d := range dst {
				if err := b.addPair(ctx, t, key, row, box, s, d, at, c); err != nil {
					return nil, b.used, err
				}
			}
		}
	}
	return b.locations, b.used, nil
}

// locationBuilder retains each traffic box's native zone-pair association.
type locationBuilder struct {
	locations []located
	used      flow.Traffic
	index     map[string]int
}

func (b *locationBuilder) addPair(ctx context.Context, t types.Target, key contextKey, row types.NormalizedRow, box flow.TrafficRegion, s, d endpoint, at time.Time, c *calculation) error {
	if !s.protected && !d.protected {
		return nil
	}
	side := "both"
	basis := "protected_both"
	if !d.protected {
		side = "src"
		basis = "protected_source_route_destination"
	}
	if !s.protected {
		side = "dst"
		basis = "route_source_protected_destination"
	}
	for _, sr := range s.addresses.Ranges() {
		for _, dr := range d.addresses.Ranges() {
			if err := c.charge(ctx, 1); err != nil {
				return err
			}
			piece := box
			piece.Source = sr
			piece.Destination = dr
			part, e := flow.NewTraffic(MaxRegions, piece)
			if e != nil {
				return normalizeError(e)
			}
			b.used, e = b.used.Union(part, MaxRegions)
			if e != nil {
				return normalizeError(e)
			}
			id := fmt.Sprintf("%s|%s|%s|%s|%t", s.zone, d.zone, d.iface, side, s.known && d.known)
			if i, ok := b.index[id]; ok {
				b.locations[i].request.Traffic, e = b.locations[i].request.Traffic.Union(part, MaxRegions)
				if e != nil {
					return normalizeError(e)
				}
			} else {
				b.index[id] = len(b.locations)
				b.locations = append(b.locations, located{target: t, row: row, request: policy.Request{Traffic: part, Scope: key.scope, FromZone: s.zone, ToZone: d.zone, EgressInterface: d.iface, At: at}, routing: key.routing, side: side, basis: basis, known: s.known && d.known})
			}
		}
	}
	return nil
}
