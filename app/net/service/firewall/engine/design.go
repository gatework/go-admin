package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/flow"
	"github.com/gatework/firemon/plan"
	"github.com/gatework/firemon/policy"
	"github.com/gatework/firemon/vendors"
	"github.com/gatework/motor/app/net/models/firewall/types"
)

type designRequest struct {
	request policy.Request
	routing string
}

// Design retains the initial base and advances one working graph per instance.
// No rendered commands are returned. Verification describes an offline proposal,
// never an applied change or a confirmed device session.
func Design(ctx context.Context, n types.Normalized, targets []types.Target, domainID int, at time.Time) (types.DraftResult, error) {
	out := types.DraftResult{At: at, Complete: true, Items: []types.DraftItem{}}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	if len(n.Rows) > MaxDesignRows {
		return out, BudgetErr
	}
	p, e := prepare(ctx, n, targets, domainID, at)
	if e != nil {
		return out, e
	}
	byInstance := map[int][]located{}
	order := []int{}
	for _, l := range p.locations {
		if _, ok := byInstance[l.target.InstanceID]; !ok {
			order = append(order, l.target.InstanceID)
		}
		byInstance[l.target.InstanceID] = append(byInstance[l.target.InstanceID], l)
	}
	unresolvedRows := map[string][]types.Notice{}
	for _, u := range p.unresolved {
		unresolvedRows[u.RowKey] = append(unresolvedRows[u.RowKey], u.Issues...)
	}
	for _, id := range order {
		item, err := designLocatedInstance(ctx, n, byInstance[id], unresolvedRows, at)
		if err != nil {
			return out, err
		}
		if item.Status != "verified" {
			out.Complete = false
		}
		out.Items = append(out.Items, item)
	}
	// Preserve the identity and settings revision of every unavailable target.
	if len(p.unresolved) > 0 || len(order) == 0 {
		retainUnresolvedDesign(&out, p, targets)
	}

	if !p.complete {
		out.Complete = false
	}
	return out, ctx.Err()
}
func retainUnresolvedDesign(out *types.DraftResult, p preparation, targets []types.Target) {
	pending := []types.DraftItem{}
	byID := map[int]int{}
	existing := map[int]int{}
	for i, item := range out.Items {
		existing[item.InstanceID] = i
	}
	for _, u := range p.unresolved {
		if i, exists := existing[u.InstanceID]; exists {
			item := &out.Items[i]
			item.Status = "needs_review"
			item.Changes = []types.DraftChange{}
			item.ResultDigest = item.BaseDigest
			item.TargetSatisfied = false
			item.OutsidePreserved = false
			item.NATUnchanged = false
			item.Issues = joinNotices(item.Issues, u.Issues)
			if !slices.Contains(item.RowKeys, u.RowKey) {
				item.RowKeys = append(item.RowKeys, u.RowKey)
			}
			continue
		}
		i, exists := byID[u.InstanceID]
		if !exists {
			item := types.DraftItem{InstanceID: u.InstanceID, Revision: u.Revision, Status: "needs_review", Changes: []types.DraftChange{}, Issues: []types.Notice{}, RowKeys: []string{}}
			for _, target := range targets {
				if target.InstanceID == u.InstanceID {
					item.SettingsVersion = target.BindingsVersion
					if item.SettingsVersion == 0 {
						item.SettingsVersion = target.Settings.Version
					}
					break
				}
			}
			i = len(pending)
			byID[u.InstanceID] = i
			pending = append(pending, item)
		}
		if !slices.Contains(pending[i].RowKeys, u.RowKey) {
			pending[i].RowKeys = append(pending[i].RowKeys, u.RowKey)
		}
		pending[i].Issues = append(pending[i].Issues, u.Issues...)
	}
	if len(pending) == 0 && len(p.unresolved) == 0 {
		pending = append(pending, types.DraftItem{Status: "needs_review", Changes: []types.DraftChange{}, Issues: []types.Notice{{Code: "unresolved_input"}}, RowKeys: []string{}})
	}
	out.Items = append(out.Items, pending...)
	out.Complete = false
}
func designLocatedInstance(ctx context.Context, n types.Normalized, locations []located, unresolvedRows map[string][]types.Notice, at time.Time) (types.DraftItem, error) {
	var e error
	id := locations[0].target.InstanceID
	base := locations[0].target
	item := types.DraftItem{InstanceID: id, Revision: base.Revision, SettingsVersion: base.BindingsVersion, BaseDigest: base.Firewall.SemanticDigest(), ResultDigest: base.Firewall.SemanticDigest(), Status: "needs_review", Changes: []types.DraftChange{}, Issues: []types.Notice{}, RowKeys: []string{}}
	if item.SettingsVersion == 0 {
		item.SettingsVersion = base.Settings.Version
	}
	requests := []designRequest{}
	for _, l := range locations {
		var err error
		requests, err = appendDesignLocation(ctx, &item, base, l, unresolvedRows, requests)
		if err != nil {
			return item, err
		}
	}

	if !base.Firewall.Complete() {
		item.Issues = append(item.Issues, types.Notice{Code: "incomplete_snapshot"})
	}
	if len(n.Issues) > 0 {
		item.Issues = append(item.Issues, n.Issues...)
	}
	if len(item.Issues) == 0 {
		item, e = designInstance(ctx, item, base.Firewall, requests, at)
		if e != nil {
			return item, e
		}
	}
	return item, nil
}
func appendDesignLocation(ctx context.Context, item *types.DraftItem, base types.Target, l located, unresolvedRows map[string][]types.Notice, requests []designRequest) ([]designRequest, error) {
	var e error
	if !slices.Contains(item.RowKeys, l.row.Key) {
		item.RowKeys = append(item.RowKeys, l.row.Key)
	}
	if l.target.Revision != base.Revision || l.target.Firewall.SemanticDigest() != base.Firewall.SemanticDigest() {
		return requests, fmt.Errorf("%w: instance revisions differ", InputErr)
	}
	if issues := unresolvedRows[l.row.Key]; len(issues) > 0 {
		item.Issues = append(item.Issues, issues...)
	}
	items, e := evaluate(ctx, l)
	if e != nil {
		return requests, e
	}
	for _, a := range items {
		item.Issues = append(item.Issues, a.Issues...)
		if a.PacketStatus == "unknown" || len(a.Unknown) > 0 || len(a.PacketUnknown) > 0 || !l.known {
			item.Issues = append(item.Issues, types.Notice{Code: "design_evidence_unknown"})
		}
		if len(a.NAT) > 0 {
			item.Issues = append(item.Issues, types.Notice{Code: "nat_view_requires_review"})
		}
	}
	if len(item.Issues) == 0 {
		same, err := policyContextMatches(ctx, l)
		if err != nil {
			return requests, normalizeError(err)
		}
		if !same {
			item.Issues = append(item.Issues, types.Notice{Code: "native_policy_context_unproven"})
		}
	}
	merged := false
	for i, q := range requests {
		if q.routing == l.routing && q.request.SameContext(l.request) {
			requests[i].request.Traffic, e = q.request.Traffic.Union(l.request.Traffic, MaxRegions)
			if e != nil {
				return requests, normalizeError(e)
			}
			merged = true
			break
		}
	}
	if !merged {
		requests = append(requests, designRequest{request: l.request, routing: l.routing})
	}
	return requests, nil
}
func designInstance(ctx context.Context, item types.DraftItem, base *fm.Firewall, requests []designRequest, at time.Time) (types.DraftItem, error) {
	working := base
	nativeRenderer, err := vendors.Renderer(base.Metadata().Vendor)
	if err != nil {
		item.Issues = append(item.Issues, types.Notice{Code: "native_plan_unproven"})
		return item, nil //nolint:nilerr // An unsupported renderer yields an explicit unproven design, not an execution error.
	}
	planner := plan.Planner{Engine: nativeEngine(), Renderer: nativeRenderer, MaxCandidates: 128}
	changes := []types.DraftChange{}
	for _, q := range requests {
		p, e := planner.Plan(ctx, working, []policy.Request{q.request})
		if e != nil {
			if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) || errors.Is(e, flow.ErrBudget) || errors.Is(e, policy.ErrBudget) {
				return item, normalizeError(e)
			}
			item.Issues = append(item.Issues, types.Notice{Code: "native_plan_unproven"})
			return item, nil
		}
		proof := p.Proof()
		if !proof.TargetSatisfied || !proof.OutsidePreserved || !proof.NATUnchanged {
			item.Issues = append(item.Issues, types.Notice{Code: "native_plan_unproven"})
			return item, nil
		}
		for _, change := range p.Changes() {
			c, e := describeChange(working, p.After(), change)
			if e != nil {
				return item, e
			}
			changes = append(changes, c)
		}
		working = p.After()
	}
	targetOK, outsideOK, natOK, e := finalChecks(ctx, base, working, requests, at)
	if e != nil {
		if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) || errors.Is(e, flow.ErrBudget) || errors.Is(e, policy.ErrBudget) {
			return item, normalizeError(e)
		}
		item.Issues = append(item.Issues, types.Notice{Code: "final_proof_unknown"})
		return item, nil
	}
	if !targetOK || !outsideOK || !natOK {
		item.Issues = append(item.Issues, types.Notice{Code: "final_proof_unknown"})
		return item, nil
	}
	item.Status = "verified"
	item.TargetSatisfied = true
	item.OutsidePreserved = true
	item.NATUnchanged = true
	item.ResultDigest = working.SemanticDigest()
	item.Changes = changes
	return item, nil
}
func describeChange(before, after *fm.Firewall, c plan.Change) (types.DraftChange, error) {
	var kind string
	switch c.Kind {
	case plan.CreateRule:
		kind = "create_rule"
	case plan.AppendMembers:
		kind = "append_members"
	case plan.RemoveRule:
		kind = "remove_rule"
	default:
		return types.DraftChange{}, InputErr
	}
	rule := after.Rule(c.Rule)
	if rule == nil {
		return types.DraftChange{}, InputErr
	}
	data := struct {
		Rule      fm.RuleRecord      `json:"rule"`
		Addresses []fm.AddressRecord `json:"addresses"`
		Services  []fm.ServiceRecord `json:"services"`
	}{Rule: rule.Record(), Addresses: []fm.AddressRecord{}, Services: []fm.ServiceRecord{}}
	for _, a := range after.Addresses() {
		if before.Address(a.ID()) == nil {
			data.Addresses = append(data.Addresses, a.Record())
		}
	}
	for _, s := range after.Services() {
		if before.Service(s.ID()) == nil {
			data.Services = append(data.Services, s.Record())
		}
	}
	b, e := json.Marshal(data)
	return types.DraftChange{Kind: kind, RuleID: string(c.Rule), Description: kind, Data: b}, e
}
func samePolicyContext(a, b policy.Request) bool {
	a.EgressInterface = ""
	b.EgressInterface = ""
	return a.SameContext(b)
}
func finalChecks(ctx context.Context, base, after *fm.Firewall, targets []designRequest, at time.Time) (bool, bool, bool, error) {
	e := nativeEngine()
	for _, target := range targets {
		ok, err := verifyDesignTarget(ctx, after, target)
		if err != nil || !ok {
			return false, false, false, err
		}
	}

	if err := e.PreserveNAT(ctx, base, after, at); err != nil {
		return true, false, false, err
	}
	contexts, err := e.Contexts(ctx, at, base, after)
	if err != nil {
		return true, false, true, err
	}
	universe, err := flow.AllTraffic(MaxRegions)
	if err != nil {
		return true, false, true, err
	}
	for _, target := range targets {
		q := target.request
		q.Traffic = universe
		if !slices.ContainsFunc(contexts, func(c policy.Request) bool { return c.SameContext(q) }) {
			contexts = append(contexts, q)
		}
	}
	if len(contexts) > MaxRegions {
		return true, false, true, BudgetErr
	}
	for _, q := range contexts {
		ok, err := preserveDesignContext(ctx, base, after, q, targets)
		if err != nil || !ok {
			return true, false, true, err
		}
	}
	return true, true, true, nil
}
func preserveDesignContext(ctx context.Context, base, after *fm.Firewall, q policy.Request, targets []designRequest) (bool, error) {
	e := nativeEngine()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	var exempt flow.Traffic
	var err error
	for _, target := range targets {
		if samePolicyContext(q, target.request) {
			exempt, err = exempt.Union(target.request.Traffic, MaxRegions)
			if err != nil {
				return false, err
			}
		}
	}
	before, err := e.Query(ctx, base, q)
	if err != nil {
		return false, err
	}
	next, err := e.Query(ctx, after, q)
	if err != nil {
		return false, err
	}
	if !before.Complete || !next.Complete {
		return false, nil
	}
	left, err := outsidePartitions(before, exempt)
	if err != nil {
		return false, err
	}
	right, err := outsidePartitions(next, exempt)
	if err != nil {
		return false, err
	}
	for key, a := range left {
		b := right[key]
		same, err := equalTraffic(a, b)
		if err != nil {
			return false, err
		}
		if !same {
			return false, nil
		}
		delete(right, key)
	}
	for _, b := range right {
		if !b.Empty() {
			return false, nil
		}
	}
	return true, nil
}
func verifyDesignTarget(ctx context.Context, after *fm.Firewall, target designRequest) (bool, error) {
	e := nativeEngine()
	q := target.request
	r, err := e.Query(ctx, after, q)
	if err != nil {
		return false, err
	}
	covered, err := r.Allowed.Contains(q.Traffic, MaxRegions)
	if err != nil {
		return false, err
	}
	if !r.Complete || !r.Unknown.Empty() || !covered {
		return false, nil
	}
	packet, err := e.QueryPacket(ctx, after, policy.PacketRequest{Request: q, VRF: target.routing})
	if err != nil {
		return false, err
	}
	if !packet.Complete || !packet.Unknown.Empty() {
		return false, nil
	}
	var verified flow.Traffic
	for _, path := range packet.Paths {
		if !path.Policy.Complete || !path.Policy.Unknown.Empty() || !path.NAT.Exact || path.NAT.PossibleAllocation {
			return false, nil
		}
		allowed, err := path.Policy.Allowed.Contains(path.Original, MaxRegions)
		if err != nil {
			return false, err
		}
		if !allowed {
			return false, nil
		}
		verified, err = verified.Union(path.Original, MaxRegions)
		if err != nil {
			return false, err
		}
	}
	covered, err = verified.Contains(q.Traffic, MaxRegions)
	if err != nil {
		return false, err
	}
	if !covered {
		return false, nil
	}
	return true, nil
}
func outsidePartitions(r policy.Result, exempt flow.Traffic) (map[string]flow.Traffic, error) {
	out := map[string]flow.Traffic{}
	for k, t := range map[string]flow.Traffic{"allowed": r.Allowed, "denied": r.Denied, "unknown": r.Unknown, "unmatched": r.Unmatched} {
		outside, e := t.Difference(exempt, MaxRegions)
		if e != nil {
			return nil, e
		}
		out[k] = outside
	}
	for _, trace := range r.Decisions {
		key := fmt.Sprintf("rule:%s:%d:%t", trace.Rule, trace.Action, trace.Known)
		outside, e := trace.Traffic.Difference(exempt, MaxRegions)
		if e != nil {
			return nil, e
		}
		out[key], e = out[key].Union(outside, MaxRegions)
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func equalTraffic(a, b flow.Traffic) (bool, error) {
	rest, e := a.Difference(b, MaxRegions)
	if e != nil || !rest.Empty() {
		return false, e
	}
	rest, e = b.Difference(a, MaxRegions)
	return e == nil && rest.Empty(), e
}

// A packet evaluator may combine independently bound native chains (ASA ingress
// and egress ACLs, for example). A generic single-chain planner must not invent
// a security chain or claim that changing one such chain satisfies the packet.
func policyContextMatches(ctx context.Context, l located) (bool, error) {
	engine := nativeEngine()
	native, err := engine.Query(ctx, l.target.Firewall, l.request)
	if err != nil {
		return false, err
	}
	packet, err := engine.QueryPacket(ctx, l.target.Firewall, policy.PacketRequest{Request: l.request, VRF: l.routing})
	if err != nil {
		return false, err
	}
	if !packet.Complete || !packet.Unknown.Empty() {
		return false, nil
	}
	aggregated := policy.Result{Complete: true}
	for _, path := range packet.Paths {
		if path.FromZone != l.request.FromZone || path.ToZone != l.request.ToZone || !path.Policy.Complete || path.NAT.ID != "" || len(path.NAT.Steps) > 0 {
			return false, nil
		}
		aggregated.Allowed, err = aggregated.Allowed.Union(path.Policy.Allowed, MaxRegions)
		if err != nil {
			return false, err
		}
		aggregated.Denied, err = aggregated.Denied.Union(path.Policy.Denied, MaxRegions)
		if err != nil {
			return false, err
		}
		aggregated.Unknown, err = aggregated.Unknown.Union(path.Policy.Unknown, MaxRegions)
		if err != nil {
			return false, err
		}
		aggregated.Unmatched, err = aggregated.Unmatched.Union(path.Policy.Unmatched, MaxRegions)
		if err != nil {
			return false, err
		}
		aggregated.Decisions = append(aggregated.Decisions, path.Policy.Decisions...)
	}
	a, err := outsidePartitions(native, flow.Traffic{})
	if err != nil {
		return false, err
	}
	b, err := outsidePartitions(aggregated, flow.Traffic{})
	if err != nil {
		return false, err
	}
	for key, left := range a {
		same, err := equalTraffic(left, b[key])
		if err != nil || !same {
			return false, err
		}
		delete(b, key)
	}
	for _, right := range b {
		if !right.Empty() {
			return false, nil
		}
	}
	return true, nil
}
