package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/flow"
	"github.com/gatework/firemon/policy"
	"go-admin/app/firewall/types"
)

type preparation struct {
	locations  []located
	unresolved []types.AnalysisItem
	complete   bool
}

// prepare pins every operation to the supplied immutable targets. It never loads
// a current revision, looks up a device, or treats route evidence as a live path.
func prepare(ctx context.Context, n types.Normalized, targets []types.Target, domain int, at time.Time) (preparation, error) {
	out := preparation{complete: true}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if len(n.Rows) > MaxRows || len(targets) > MaxRegions {
		return out, ErrBudget
	}
	if domain <= 0 {
		return out, fmt.Errorf("%w: domainId", ErrInput)
	}
	if at.IsZero() {
		return out, fmt.Errorf("%w: analysis time required", ErrInput)
	}
	groups := make([]map[contextKey][]types.Coverage, len(targets))
	missingTargets := []types.Target{}
	for i, t := range targets {
		coverage, e := Coverage(ctx, t)
		if e != nil {
			return out, e
		}
		groups[i] = groupCoverage(coverage, domain)
		if t.Firewall == nil {
			if len(groups[i]) > 0 {
				missingTargets = append(missingTargets, t)
			}
			groups[i] = nil
		}
	}
	budget := &calculation{}
	for _, row := range n.Rows {
		if e := ctx.Err(); e != nil {
			return out, e
		}
		if row.Status != "ready" || len(row.Issues) > 0 {
			out.complete = false
		}
		var all, used flow.Traffic
		var unresolved []types.Notice
		literal := true
		for _, terms := range [][]types.Term{row.Sources, row.Destinations, row.Services} {
			for _, v := range terms {
				if v.Kind == "name" {
					literal = false
				}
			}
		}
		if literal {
			var e error
			all, unresolved, e = resolveTraffic(ctx, row, nil, "", budget)
			if e != nil {
				return out, e
			}
		}
		beforeUnresolved := len(out.unresolved)
		for _, missing := range missingTargets {
			if err := budget.charge(ctx, 1); err != nil {
				return out, err
			}
			out.complete = false
			out.unresolved = append(out.unresolved, types.AnalysisItem{RowKey: row.Key, InstanceID: missing.InstanceID, InstanceName: missing.Name, Revision: missing.Revision, Status: "unknown", Basis: "missing_snapshot", Original: parts(all), Unknown: parts(all), PacketStatus: "unknown", PacketUnknown: parts(all), Issues: append(append([]types.Notice{}, row.Issues...), types.Notice{Code: "snapshot_missing"})})
		}
		before := len(out.locations)
		for i, t := range targets {
			for _, key := range sortedContexts(groups[i]) {
				traffic, issues, e := resolveTraffic(ctx, row, t.Firewall, key.scope, budget)
				if e != nil {
					return out, e
				}
				if len(issues) > 0 {
					out.complete = false
					out.unresolved = append(out.unresolved, types.AnalysisItem{RowKey: row.Key, InstanceID: t.InstanceID, InstanceName: t.Name, Revision: t.Revision, Scope: key.scope, RoutingScope: key.routing, Status: "unknown", Basis: "unresolved_input", PacketStatus: "unknown", Issues: joinNotices(row.Issues, issues)})
				}
				if traffic.Empty() {
					unresolved = append(unresolved, issues...)
					continue
				}
				all, e = all.Union(traffic, MaxRegions)
				if e != nil {
					return out, normalizeError(e)
				}
				locs, hit, e := locateTraffic(ctx, t, key, groups[i][key], row, traffic, at, budget)
				if e != nil {
					return out, e
				}
				for j := range locs {
					locs[j].issues = joinNotices(row.Issues, issues)
				}
				out.locations = append(out.locations, locs...)
				used, e = used.Union(hit, MaxRegions)
				if e != nil {
					return out, normalizeError(e)
				}
			}
		}
		rest, e := all.Difference(used, MaxRegions)
		if e != nil {
			return out, normalizeError(e)
		}
		if !rest.Empty() && len(missingTargets) == 0 {
			out.complete = false
			out.unresolved = append(out.unresolved, types.AnalysisItem{RowKey: row.Key, Status: "unlocated", Basis: "no_protected_zone", Original: parts(rest), Unknown: parts(rest), PacketStatus: "unknown", PacketUnknown: parts(rest), Issues: append(append([]types.Notice{}, row.Issues...), types.Notice{Code: "unlocated"})})
		}
		if len(out.locations) == before && rest.Empty() && len(out.unresolved) == beforeUnresolved {
			out.complete = false
			issues := append(append([]types.Notice{}, row.Issues...), unresolved...)
			if len(issues) == 0 {
				issues = append(issues, types.Notice{Code: "unresolved_input"})
			}
			out.unresolved = append(out.unresolved, types.AnalysisItem{RowKey: row.Key, Status: "unknown", Basis: "unresolved_input", PacketStatus: "unknown", Issues: issues})
		}
	}
	if len(n.Rows) == 0 || len(n.Issues) > 0 {
		out.complete = false
	}
	return out, nil
}

// Analyze returns native policy coverage and, separately, packet-view evidence.
// Firewall hits are independent observations; their order is not a path claim.
func Analyze(ctx context.Context, n types.Normalized, targets []types.Target, domainID int, at time.Time) (types.AnalysisResult, error) {
	prepared, e := prepare(ctx, n, targets, domainID, at)
	if e != nil {
		return types.AnalysisResult{}, e
	}
	out := types.AnalysisResult{Complete: prepared.complete, Items: []types.AnalysisItem{}, Issues: append([]types.Notice{}, n.Issues...)}
	for _, location := range prepared.locations {
		items, e := evaluate(ctx, location)
		if e != nil {
			return out, e
		}
		for _, item := range items {
			if item.Status == "unknown" || item.Status == "unlocated" || len(item.Unknown) > 0 || item.PacketStatus == "unknown" || len(item.PacketUnknown) > 0 || len(item.Issues) > 0 {
				out.Complete = false
			}
			out.Items = append(out.Items, item)
		}
	}
	out.Items = append(out.Items, prepared.unresolved...)
	return out, ctx.Err()
}
func itemBase(l located) types.AnalysisItem {
	return types.AnalysisItem{RowKey: l.row.Key, InstanceID: l.target.InstanceID, InstanceName: l.target.Name, Revision: l.target.Revision, Scope: l.request.Scope, RoutingScope: l.routing, FromZone: string(l.request.FromZone), ToZone: string(l.request.ToZone), HitSide: l.side, Basis: l.basis, Original: parts(l.request.Traffic), PolicyView: "native_policy", Issues: append([]types.Notice{}, l.issues...), Trace: []types.RuleTrace{}, NAT: []types.NATTrace{}}
}
func status(r policy.Result) string {
	if !r.Unknown.Empty() {
		if r.Allowed.Empty() && r.Denied.Empty() && r.Unmatched.Empty() {
			return "unknown"
		}
		return "partial"
	}
	count := 0
	result := "unmatched"
	if !r.Allowed.Empty() {
		count++
		result = "allowed"
	}
	if !r.Denied.Empty() {
		count++
		result = "denied"
	}
	if !r.Unmatched.Empty() {
		count++
		result = "unmatched"
	}
	if count > 1 {
		return "partial"
	}
	return result
}
func applyResult(item *types.AnalysisItem, r policy.Result, fw *fm.Firewall) {
	item.Status = status(r)
	item.Allowed = parts(r.Allowed)
	item.Denied = parts(r.Denied)
	item.Unmatched = parts(r.Unmatched)
	item.Unknown = parts(r.Unknown)
	item.Trace = []types.RuleTrace{}
	for _, t := range r.Decisions {
		name := ""
		if rule := fw.Rule(t.Rule); rule != nil {
			name = rule.Name()
		}
		item.Trace = append(item.Trace, types.RuleTrace{ID: string(t.Rule), Name: name, Action: action(t.Action), Certain: t.Known, Traffic: parts(t.Traffic)})
	}
}
func action(a fm.Action) string {
	switch a {
	case fm.Permit:
		return "allow"
	case fm.Deny:
		return "deny"
	case fm.Reject:
		return "reject"
	case fm.Drop:
		return "drop"
	case fm.ResetClient:
		return "reset_client"
	case fm.ResetServer:
		return "reset_server"
	case fm.ResetBoth:
		return "reset_both"
	}
	return "unknown"
}
func natKind(kind fm.Kind) string {
	switch kind {
	case fm.StaticNATObject:
		return "static_nat"
	case fm.DynamicNATObject:
		return "dynamic_nat"
	}
	return "identity"
}
func natTrace(r policy.NATRelation) types.NATTrace {
	kind := natKind(r.Kind)
	steps := make([]types.NATStep, 0, len(r.Steps))
	for _, step := range r.Steps {
		steps = append(steps, types.NATStep{ID: string(step.ID), Kind: natKind(step.Kind), Reverse: step.Reverse})
	}
	if len(steps) > 1 {
		kind = "pipeline"
	} else if len(steps) == 1 {
		kind = steps[0].Kind
	}
	return types.NATTrace{ID: string(r.ID), Kind: kind, Exact: r.Exact, PossibleAllocation: r.PossibleAllocation, Original: parts(r.Original), Translated: parts(r.Translated), Steps: steps}
}
func evaluate(ctx context.Context, l located) ([]types.AnalysisItem, error) {
	base := itemBase(l)
	if !l.known {
		base.Status = "unknown"
		base.Unknown = parts(l.request.Traffic)
		base.PacketStatus = "unknown"
		base.PacketUnknown = parts(l.request.Traffic)
		base.Issues = append(base.Issues, types.Notice{Code: "zone_evidence_unknown"})
		return []types.AnalysisItem{base}, nil
	}
	engine := nativeEngine()
	fw := l.target.Firewall
	evaluated, e := engine.Query(ctx, fw, l.request)
	if e != nil {
		return nil, normalizeError(e)
	}
	applyResult(&base, evaluated, fw)
	nat, natErr := engine.QueryNAT(ctx, fw, l.request)
	if natErr != nil && !errors.Is(natErr, policy.ErrNAT) {
		return nil, normalizeError(natErr)
	}
	if natErr == nil {
		for _, r := range nat.Relations {
			base.NAT = append(base.NAT, natTrace(r))
		}
	}
	packet, e := engine.QueryPacket(ctx, fw, policy.PacketRequest{Request: l.request, VRF: l.routing})
	if e != nil {
		if !errors.Is(e, policy.ErrNAT) {
			return nil, normalizeError(e)
		}
		base.PacketStatus = "unknown"
		base.PacketUnknown = parts(l.request.Traffic)
		base.Issues = append(base.Issues, types.Notice{Code: "packet_semantics_unavailable"})
		if natErr != nil && (len(fw.StaticNATs()) > 0 || len(fw.DynamicNATs()) > 0) {
			base.Issues = append(base.Issues, types.Notice{Code: "nat_semantics_unknown"})
		}
		return []types.AnalysisItem{base}, nil
	}
	if len(packet.Paths) == 0 && !packet.Unknown.Empty() {
		base.PacketStatus = "unknown"
		base.PacketUnknown = parts(packet.Unknown)
		base.Issues = append(base.Issues, types.Notice{Code: "packet_evidence_unknown"})
		return []types.AnalysisItem{base}, nil
	}
	var out []types.AnalysisItem
	for _, path := range packet.Paths {
		item := itemBase(l)
		item.PolicyView = "native_packet_policy"
		item.Original = parts(path.Original)
		item.FromZone = string(path.FromZone)
		item.ToZone = string(path.ToZone)
		applyResult(&item, path.Policy, fw)
		item.PacketStatus = item.Status
		if path.NAT.ID != "" || len(path.NAT.Steps) > 0 {
			item.NAT = append(item.NAT, natTrace(path.NAT))
		}
		if !path.Policy.Unknown.Empty() || !path.Policy.Complete || path.NAT.PossibleAllocation || !path.NAT.Exact {
			item.PacketUnknown = parts(path.Original)
			item.PacketStatus = "unknown"
			item.Issues = append(item.Issues, types.Notice{Code: "packet_outcome_unknown"})
		}
		if l.side == "dst" {
			item.PacketStatus = "unknown"
			item.PacketUnknown = parts(path.Original)
			item.Issues = append(item.Issues, types.Notice{Code: "ingress_inferred_from_route"})
		}
		out = append(out, item)
	}
	if !packet.Unknown.Empty() {
		item := itemBase(l)
		item.PolicyView = "native_packet_policy"
		item.Original = parts(packet.Unknown)
		item.Status = "unknown"
		item.Unknown = parts(packet.Unknown)
		item.PacketStatus = "unknown"
		item.PacketUnknown = parts(packet.Unknown)
		item.Issues = append(item.Issues, types.Notice{Code: "packet_evidence_unknown"})
		out = append(out, item)
	}
	if len(out) == 0 {
		base.Status = "unknown"
		base.Allowed = nil
		base.Denied = nil
		base.Unmatched = nil
		base.Trace = nil
		base.Unknown = parts(l.request.Traffic)
		base.PacketStatus = "unknown"
		base.PacketUnknown = parts(l.request.Traffic)
		base.Issues = append(base.Issues, types.Notice{Code: "packet_evidence_unknown"})
		out = append(out, base)
	}
	return out, ctx.Err()
}
