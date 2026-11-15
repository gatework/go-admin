package engine

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/flow"
	"github.com/gatework/firemon/policy"
	"github.com/gatework/motor/app/net/models/firewall/types"
)

var netPrefix4 = netip.MustParsePrefix("0.0.0.0/0")
var netPrefix6 = netip.MustParsePrefix("::/0")

func nativeEngine() policy.Engine {
	return policy.Engine{Limits: policy.Limits{MaxRegions: MaxRegions, MaxCombinations: MaxRegions, MaxVisits: 100000}}
}

type calculation struct{ used int }

func (c *calculation) charge(ctx context.Context, n int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if n > MaxRegions-c.used {
		return BudgetErr
	}
	c.used += n
	return nil
}
func nativeScopes(fw *fm.Firewall, scope string) []string {
	out := []string{scope}
	if fw != nil {
		for _, z := range fw.Zones() {
			if z.Scope() == scope && !slices.Contains(out, z.Record().AddressScope) {
				out = append(out, z.Record().AddressScope)
			}
		}
	}
	if !slices.Contains(out, "") {
		out = append(out, "")
	}
	return out
}
func namedScopes(token string, allowed []string) (string, []string, bool) {
	if parts := strings.SplitN(token, "::", 2); len(parts) == 2 {
		if !slices.Contains(allowed, parts[0]) {
			return "", nil, false
		}
		return parts[1], []string{parts[0]}, true
	}
	return token, allowed, true
}
func addressRefs(ctx context.Context, fw *fm.Firewall, token, scope string) ([]fm.AddressRef, bool) {
	if fw == nil {
		return nil, false
	}
	name, scopes, ok := namedScopes(token, nativeScopes(fw, scope))
	if !ok {
		return nil, false
	}
	// A local namespace shadows global declarations. Multiple visible local
	// address-books require clarification rather than arbitrary name selection.
	var refs []fm.AddressRef
	for _, s := range scopes {
		var at []fm.AddressRef
		for _, a := range fw.Addresses() {
			if ctx.Err() != nil {
				return nil, false
			}
			if a.Scope() == s && (a.Name() == name || string(a.ID()) == name) {
				at = append(at, fm.AddressRef{Kind: fm.AddressObject, ID: a.ID()})
			}
		}
		for _, a := range fw.AddressGroups() {
			if ctx.Err() != nil {
				return nil, false
			}
			if a.Scope() == s && (a.Name() == name || string(a.ID()) == name) {
				at = append(at, fm.AddressRef{Kind: fm.AddressGroupObject, ID: a.ID()})
			}
		}
		if s == "" && len(refs) > 0 {
			continue
		}
		refs = append(refs, at...)
	}
	return refs, len(refs) == 1
}
func serviceRefs(ctx context.Context, fw *fm.Firewall, token, scope string) ([]fm.ServiceRef, bool) {
	if fw == nil {
		return nil, false
	}
	name, scopes, ok := namedScopes(token, []string{scope, ""})
	if !ok {
		return nil, false
	}
	for _, s := range scopes {
		var refs []fm.ServiceRef
		for _, a := range fw.Services() {
			if ctx.Err() != nil {
				return nil, false
			}
			if a.Scope() == s && (a.Name() == name || string(a.ID()) == name) {
				refs = append(refs, fm.ServiceRef{Kind: fm.ServiceObject, ID: a.ID()})
			}
		}
		for _, a := range fw.ServiceGroups() {
			if ctx.Err() != nil {
				return nil, false
			}
			if a.Scope() == s && (a.Name() == name || string(a.ID()) == name) {
				refs = append(refs, fm.ServiceRef{Kind: fm.ServiceGroupObject, ID: a.ID()})
			}
		}
		if len(refs) > 0 {
			return refs, len(refs) == 1
		}
	}
	return nil, false
}
func resolveAddressTerms(ctx context.Context, terms []types.Term, fw *fm.Firewall, scope, field string, c *calculation) (flow.AddressSet, []types.Notice, error) {
	var out flow.AddressSet
	var issues []types.Notice
	for _, term := range terms {
		if err := ctx.Err(); err != nil {
			return out, issues, err
		}
		var set flow.AddressSet
		known := true
		switch term.Kind {
		case "any":
			a, _ := flow.PrefixRange(netPrefix4)
			b, _ := flow.PrefixRange(netPrefix6)
			set, _ = flow.NewAddressSet(a, b)
		case "address":
			r, ok := addressRange(term.Value)
			if !ok {
				known = false
			} else {
				set, _ = flow.NewAddressSet(r)
			}
		case "name":
			refs, ok := addressRefs(ctx, fw, term.Value, scope)
			if err := ctx.Err(); err != nil {
				return out, issues, err
			}
			known = ok
			if ok {
				var e error
				set, known, e = nativeEngine().ResolveAddresses(ctx, fw, refs, false)
				if e != nil {
					return out, issues, normalizeError(e)
				}
			}
		default:
			known = false
		}
		if !known {
			issues = append(issues, types.Notice{Field: field, Token: term.Value, Code: "unresolved_name"})
			continue
		}
		out = out.Union(set)
		if len(out.Ranges()) > MaxRegions {
			return out, issues, BudgetErr
		}
	}
	return out, issues, nil
}
func serviceTerm(value string) (fm.ServiceTerm, bool) {
	full := flow.Interval{Low: 0, High: 65535}
	octet := flow.Interval{Low: 0, High: 255}
	s := fm.ServiceTerm{Protocol: octet, SourcePort: full, DestinationPort: full, ICMPType: octet, ICMPCode: octet}
	if value == "any" {
		return s, true
	}
	ps := strings.Split(value, "/")
	protocol, ok := protocolNumber(ps[0])
	if !ok {
		return s, false
	}
	s.Protocol = flow.Interval{Low: protocol, High: protocol}
	if len(ps) == 1 {
		return s, true
	}
	if len(ps) > 3 {
		return s, false
	}
	if protocol == 1 || protocol == 58 {
		s.ICMPType, ok = interval(ps[1], 255)
		if len(ps) == 3 && ok {
			s.ICMPCode, ok = interval(ps[2], 255)
		}
	} else if len(ps) == 3 {
		s.SourcePort, ok = interval(ps[1], 65535)
		if ok {
			s.DestinationPort, ok = interval(ps[2], 65535)
		}
	} else {
		s.DestinationPort, ok = interval(ps[1], 65535)
	}
	return s, ok
}
func resolveTraffic(ctx context.Context, row types.NormalizedRow, fw *fm.Firewall, scope string, c *calculation) (flow.Traffic, []types.Notice, error) {
	sources, issues, e := resolveAddressTerms(ctx, row.Sources, fw, scope, "source", c)
	if e != nil {
		return flow.Traffic{}, issues, e
	}
	dest, more, e := resolveAddressTerms(ctx, row.Destinations, fw, scope, "destination", c)
	issues = append(issues, more...)
	if e != nil {
		return flow.Traffic{}, issues, e
	}
	issues = joinNotices(issues, familyNotices(row, sources, dest))
	services, more, e := resolveServiceTerms(ctx, row.Services, fw, scope)
	issues = append(issues, more...)
	if e != nil {
		return flow.Traffic{}, issues, e
	}
	var boxes []flow.TrafficRegion
	for _, src := range sources.Ranges() {
		for _, dst := range dest.Ranges() {
			if src.First.BitLen() != dst.First.BitLen() {
				continue
			}
			for _, s := range services {
				if e := c.charge(ctx, 1); e != nil {
					return flow.Traffic{}, issues, e
				}
				boxes = append(boxes, flow.TrafficRegion{Source: src, Destination: dst, Protocol: s.Protocol, SourcePort: s.SourcePort, DestinationPort: s.DestinationPort, ICMPType: s.ICMPType, ICMPCode: s.ICMPCode})
			}
		}
	}
	traffic, e := flow.NewTraffic(MaxRegions, boxes...)
	return traffic, issues, normalizeError(e)
}
func resolveServiceTerms(ctx context.Context, terms []types.Term, fw *fm.Firewall, scope string) ([]fm.ServiceTerm, []types.Notice, error) {
	var issues []types.Notice
	var services []fm.ServiceTerm
	for _, term := range terms {
		if e := ctx.Err(); e != nil {
			return nil, issues, e
		}
		if term.Kind == "name" {
			refs, known := serviceRefs(ctx, fw, term.Value, scope)
			if err := ctx.Err(); err != nil {
				return nil, issues, err
			}
			if known {
				resolved, k, e := nativeEngine().ResolveServices(ctx, fw, refs, false)
				if e != nil {
					return nil, issues, normalizeError(e)
				}
				if k {
					services = append(services, resolved...)
					continue
				}
			}
			issues = append(issues, types.Notice{Field: "service", Token: term.Value, Code: "unresolved_name"})
			continue
		}
		if s, ok := serviceTerm(term.Value); ok {
			services = append(services, s)
		} else {
			issues = append(issues, types.Notice{Field: "service", Token: term.Value, Code: "invalid_token"})
		}
	}
	return services, issues, nil
}
func normalizeError(err error) error {
	if errors.Is(err, flow.ErrBudget) || errors.Is(err, policy.ErrBudget) {
		return fmt.Errorf("%w: %w", BudgetErr, err)
	}
	return err
}
func intervalString(i flow.Interval) string {
	if i.Low == i.High {
		return strconv.Itoa(int(i.Low))
	}
	return fmt.Sprintf("%d-%d", i.Low, i.High)
}
func parts(t flow.Traffic) []types.TrafficPart {
	out := []types.TrafficPart{}
	for _, b := range t.Regions() {
		protocol := intervalString(b.Protocol)
		if b.Protocol.Low == b.Protocol.High {
			switch b.Protocol.Low {
			case 6:
				protocol = "tcp"
			case 17:
				protocol = "udp"
			case 1:
				protocol = "icmp"
			case 58:
				protocol = "icmpv6"
			case 132:
				protocol = "sctp"
			}
		}
		out = append(out, types.TrafficPart{Source: rangeString(b.Source), Destination: rangeString(b.Destination), Protocol: protocol, SourcePort: intervalString(b.SourcePort), DestinationPort: intervalString(b.DestinationPort), ICMPType: intervalString(b.ICMPType), ICMPCode: intervalString(b.ICMPCode)})
	}
	return out
}

func joinNotices(groups ...[]types.Notice) []types.Notice {
	out := []types.Notice{}
	seen := map[types.Notice]bool{}
	for _, notices := range groups {
		for _, notice := range notices {
			if !seen[notice] {
				out = append(out, notice)
				seen[notice] = true
			}
		}
	}
	return out
}
func literalAddressSet(terms []types.Term) (flow.AddressSet, bool) {
	var set flow.AddressSet
	for _, term := range terms {
		switch term.Kind {
		case "any":
			v4, _ := flow.PrefixRange(netPrefix4)
			v6, _ := flow.PrefixRange(netPrefix6)
			both, _ := flow.NewAddressSet(v4, v6)
			set = set.Union(both)
		case "address":
			r, ok := addressRange(term.Value)
			if !ok {
				return set, false
			}
			single, _ := flow.NewAddressSet(r)
			set = set.Union(single)
		default:
			return set, false
		}
	}
	return set, true
}

// A wildcard covers whichever family its counterpart supplies. Explicit ranges
// and native groups must retain every family they actually request; an IPv6
// fragment cannot disappear merely because only an IPv4 destination is given.
func familyNotices(row types.NormalizedRow, source, destination flow.AddressSet) []types.Notice {
	if source.Empty() || destination.Empty() {
		return nil
	}
	sourceFamilies, destinationFamilies := map[int]bool{}, map[int]bool{}
	for _, r := range source.Ranges() {
		sourceFamilies[r.First.BitLen()] = true
	}
	for _, r := range destination.Ranges() {
		destinationFamilies[r.First.BitLen()] = true
	}
	anySource, anyDestination := false, false
	for _, term := range row.Sources {
		anySource = anySource || term.Kind == "any"
	}
	for _, term := range row.Destinations {
		anyDestination = anyDestination || term.Kind == "any"
	}
	out := []types.Notice{}
	add := func(field string, r flow.AddressRange) {
		family := "ipv4"
		if r.First.Is6() {
			family = "ipv6"
		}
		out = append(out, types.Notice{Field: field, Token: rangeString(r), Code: "address_family_mismatch", Value: family})
	}
	if !anySource {
		for _, r := range source.Ranges() {
			if !destinationFamilies[r.First.BitLen()] {
				add("source", r)
			}
		}
	}
	if !anyDestination {
		for _, r := range destination.Ranges() {
			if !sourceFamilies[r.First.BitLen()] {
				add("destination", r)
			}
		}
	}
	return out
}
