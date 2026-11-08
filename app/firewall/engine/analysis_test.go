package engine_test

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"testing"
	"time"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/flow"
	"go-admin/app/firewall/engine"
	"go-admin/app/firewall/types"
)

func graph(t *testing.T, vendor string, rules ...fm.RuleRecord) *fm.Firewall {
	t.Helper()
	fw, err := fm.Seal(fm.Data{Metadata: fm.Metadata{Device: "synthetic", Vendor: vendor}, Zones: []fm.ZoneRecord{{Object: fm.Object{ID: "inside", Name: "inside"}}, {Object: fm.Object{ID: "outside", Name: "outside"}}}, Interfaces: []fm.InterfaceRecord{{Object: fm.Object{ID: "in", Name: "in", Scope: "vrf"}, Zone: "inside", Addresses: []netip.Prefix{netip.MustParsePrefix("10.0.0.1/24")}}, {Object: fm.Object{ID: "out", Name: "out", Scope: "vrf"}, Zone: "outside", Addresses: []netip.Prefix{netip.MustParsePrefix("192.0.2.1/24")}}}, Routes: []fm.RouteRecord{{Object: fm.Object{ID: "connected", Scope: "vrf"}, Destination: netip.MustParsePrefix("10.0.0.0/24"), Interface: "in", Direct: true}, {Object: fm.Object{ID: "extra", Scope: "vrf"}, Destination: netip.MustParsePrefix("172.16.0.0/16"), NextHop: netip.MustParseAddr("10.0.0.254"), Interface: "in"}, {Object: fm.Object{ID: "nonrecursive", Scope: "vrf"}, Destination: netip.MustParsePrefix("172.17.0.0/16"), NextHop: netip.MustParseAddr("172.16.0.1"), Interface: "in"}, {Object: fm.Object{ID: "default", Scope: "vrf"}, Destination: netip.MustParsePrefix("0.0.0.0/0"), NextHop: netip.MustParseAddr("192.0.2.254"), Interface: "out"}}, Rules: rules})
	if err != nil {
		t.Fatal(err)
	}
	return fw
}
func target(fw *fm.Firewall) types.Target {
	return types.Target{InstanceID: 11, Name: "wall", Revision: "rev-1", DomainID: 1, Mode: "routed", BindingsVersion: 7, Firewall: fw, Settings: types.Settings{Version: 7, Zones: []types.Binding{{Zone: "inside", RoutingScope: "vrf", DomainID: 1, Protected: true}, {Zone: "outside", RoutingScope: "vrf", DomainID: 1}}}}
}
func rule(id string, position int, action fm.Action) fm.RuleRecord {
	return fm.RuleRecord{Object: fm.Object{ID: fm.ID(id), Name: id}, Position: position, RuleSet: "security", Action: action, Log: fm.Irrelevant, Match: fm.Match{AnySource: true, AnyDestination: true, AnyService: true, AnyFromZone: true, AnyToZone: true}}
}
func normalized(t *testing.T, rows ...types.InputRow) types.Normalized {
	t.Helper()
	n, e := engine.Normalize(types.Input{Rows: rows})
	if e != nil {
		t.Fatal(e)
	}
	return n
}
func row(src, dst, srv string) types.InputRow {
	return types.InputRow{Source: src, Destination: dst, Service: srv}
}

func TestCoverageUsesOriginalNetworksAndExcludesDefault(t *testing.T) {
	c, err := engine.Coverage(context.Background(), target(graph(t, "paloalto")))
	if err != nil {
		t.Fatal(err)
	}
	seenExtra := false
	for _, v := range c {
		if !v.Protected {
			continue
		}
		r := v.Network
		if r == "0.0.0.0/0" || r == "172.17.0.0/16" {
			t.Fatalf("default or recursive expansion=%+v", v)
		}
		if r == "172.16.0.0/16" {
			seenExtra = true
		}
		if v.Revision != "rev-1" || v.BindingsVersion != 7 || v.DomainID != 1 {
			t.Fatal(v)
		}
	}
	if !seenExtra {
		t.Fatalf("static route not expanded: %+v", c)
	}
}
func TestAnalysisScansBothSidesDeduplicatesAndKeepsFirstMatch(t *testing.T) {
	fw := graph(t, "paloalto", rule("deny-first", 1, fm.Deny), rule("permit-later", 2, fm.Permit))
	n := normalized(t, row("10.0.0.2", "8.8.8.8", "443"), row("8.8.8.8", "10.0.0.2", "tcp/80"), row("10.0.0.2", "10.0.0.3", "443"))
	r, err := engine.Analyze(context.Background(), n, []types.Target{target(fw)}, 1, time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Items) != 3 {
		t.Fatalf("duplicated/missing hits: %+v", r)
	}
	for i, item := range r.Items {
		if item.Status != "denied" || len(item.Trace) != 1 || item.Trace[0].ID != "deny-first" {
			t.Fatalf("first match not retained: %+v", item)
		}
		want := []string{"src", "dst", "both"}[i]
		if item.HitSide != want {
			t.Fatalf("side=%q want=%q", item.HitSide, want)
		}
		if i == 2 && item.FromZone != item.ToZone {
			t.Fatalf("same-zone flow changed=%+v", item)
		}
	}
}
func TestAnalysisRetainsUnlocatedRangeAndDomainIsolation(t *testing.T) {
	n := normalized(t, row("10.0.0.0/23", "8.8.8.8", "tcp/80"))
	r, err := engine.Analyze(context.Background(), n, []types.Target{target(graph(t, "paloalto", rule("allow", 1, fm.Permit)))}, 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r.Complete || len(r.Items) != 2 {
		t.Fatalf("partial coverage silently widened=%+v", r)
	}
	var covered, missing bool
	for _, i := range r.Items {
		if i.Status == "allowed" {
			covered = len(i.Allowed) == 1 && i.Allowed[0].Source == "10.0.0.0-10.0.0.255"
		}
		if i.Status == "unlocated" {
			missing = len(i.Unknown) == 1 && i.Unknown[0].Source == "10.0.1.0-10.0.1.255"
		}
	}
	if !covered || !missing {
		t.Fatalf("range partition incorrect: %+v", r)
	}
	r, err = engine.Analyze(context.Background(), n, []types.Target{target(graph(t, "paloalto"))}, 2, time.Now())
	if err != nil || len(r.Items) != 1 || r.Items[0].Status != "unlocated" {
		t.Fatalf("domain isolation=%+v %v", r, err)
	}
}
func TestAnalysisPartialInputAndUnknownActionRemainUnknown(t *testing.T) {
	n := normalized(t, row("10.0.0.2,999.1.1.1", "8.8.8.8", "443"))
	r, err := engine.Analyze(context.Background(), n, []types.Target{target(graph(t, "paloalto", rule("unknown", 1, fm.UnknownAction)))}, 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r.Complete || len(r.Items) == 0 || r.Items[0].Status != "unknown" || len(r.Items[0].Allowed) != 0 {
		t.Fatalf("unknown promoted=%+v", r)
	}
}
func TestEngineCancellationAndCombinationBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fw := graph(t, "paloalto")
	if _, e := engine.Coverage(ctx, target(fw)); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e := engine.Analyze(ctx, normalized(t, row("10.0.0.1", "8.8.8.8", "443")), []types.Target{target(fw)}, 1, time.Now()); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	n := normalized(t, row("10.0.0.1", "8.8.8.8", "443"))
	for i := 0; i < 4100; i++ {
		n.Rows[0].Services = append(n.Rows[0].Services, types.Term{Value: "tcp/443", Kind: "service"})
	}
	if _, e := engine.Analyze(context.Background(), n, []types.Target{target(fw)}, 1, time.Now()); !errors.Is(e, engine.ErrBudget) {
		t.Fatalf("expansion not bounded: %v", e)
	}
}
func TestAnalysisResolvesNativeGroupsWithinScopeWithoutEnumeratingHosts(t *testing.T) {
	data := graph(t, "paloalto", rule("allow", 1, fm.Permit)).Export()
	r, _ := flow.PrefixRange(netip.MustParsePrefix("10.0.0.0/8"))
	data.Addresses = []fm.AddressRecord{{Object: fm.Object{ID: "local", Name: "clients"}, Ranges: []flow.AddressRange{r}}, {Object: fm.Object{ID: "other", Name: "clients", Scope: "another"}, Ranges: []flow.AddressRange{{First: netip.MustParseAddr("192.0.2.1"), Last: netip.MustParseAddr("192.0.2.1")}}}}
	data.AddressGroups = []fm.AddressGroupRecord{{Object: fm.Object{ID: "group", Name: "employees"}, Members: []fm.AddressRef{{Kind: fm.AddressObject, ID: "local"}}}}
	data.Services = []fm.ServiceRecord{{Object: fm.Object{ID: "web", Name: "WEB"}, Terms: []fm.ServiceTerm{{Protocol: flow.Interval{Low: 6, High: 6}, SourcePort: flow.Interval{Low: 1024, High: 65535}, DestinationPort: flow.Interval{Low: 443, High: 443}, ICMPType: flow.Interval{Low: 0, High: 255}, ICMPCode: flow.Interval{Low: 0, High: 255}}}}}
	fw, e := fm.Seal(data)
	if e != nil {
		t.Fatal(e)
	}
	r2, e := engine.Analyze(context.Background(), normalized(t, row("employees", "8.8.8.8", "WEB")), []types.Target{target(fw)}, 1, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if len(r2.Items) < 1 || len(r2.Items[0].Allowed) != 1 || r2.Items[0].Allowed[0].SourcePort != "1024-65535" {
		t.Fatalf("native tuple lost=%+v", r2)
	}
}

func TestProtectionDerivesNativeRoutingScopeFromSelectedZone(t *testing.T) {
	target := target(graph(t, "paloalto", rule("allow", 1, fm.Permit)))
	target.Settings.Zones[0].RoutingScope = ""
	target.Settings.Zones[1].RoutingScope = ""
	r, e := engine.Analyze(context.Background(), normalized(t, row("10.0.0.2", "8.8.8.8", "443")), []types.Target{target}, 1, time.Now())
	if e != nil || len(r.Items) != 1 || r.Items[0].RoutingScope != "vrf" || r.Items[0].Status != "allowed" {
		t.Fatalf("native route scope omitted=%+v %v", r, e)
	}
}
func TestSimpleThousandRowsStayWithinCombinationBudget(t *testing.T) {
	rows := make([]types.InputRow, 1000)
	for i := range rows {
		rows[i] = row("10.0.0.2", "8.8.8.8", "443")
	}
	r, e := engine.Analyze(context.Background(), normalized(t, rows...), []types.Target{target(graph(t, "paloalto", rule("allow", 1, fm.Permit)))}, 1, time.Now())
	if e != nil || len(r.Items) != 1000 {
		t.Fatalf("simple row budget rejected: %d %v", len(r.Items), e)
	}
}
func TestTransparentManualBindingRetainsPolicyResultWithUnknownPacketView(t *testing.T) {
	data := graph(t, "paloalto", rule("allow", 1, fm.Permit)).Export()
	data.Routes = nil
	data.Interfaces = nil
	fw, e := fm.Seal(data)
	if e != nil {
		t.Fatal(e)
	}
	target := target(fw)
	target.Mode = "transparent"
	target.Settings.Zones[0].Networks = []string{"10.0.0.0/24"}
	target.Settings.Zones[1].Networks = []string{"8.8.8.0/24"}
	r, e := engine.Analyze(context.Background(), normalized(t, row("10.0.0.2", "8.8.8.8", "443")), []types.Target{target}, 1, time.Now())
	if e != nil || len(r.Items) != 1 || r.Items[0].Status != "allowed" || r.Items[0].PacketStatus != "unknown" || r.Items[0].PolicyView != "native_policy" || r.Complete {
		t.Fatalf("native policy coverage discarded=%+v %v", r, e)
	}
}

func TestNativeScopeDomainMappingsRemainIsolatedAndConflictsUnknown(t *testing.T) {
	target := target(graph(t, "paloalto", rule("allow", 1, fm.Permit)))
	for i := range target.Settings.Zones {
		target.Settings.Zones[i].DomainID = 0
	}
	target.Settings.Scopes = []types.ScopeBinding{{RoutingScope: "vrf", DomainID: 2}}
	n := normalized(t, row("10.0.0.2", "8.8.8.8", "443"))
	r, e := engine.Analyze(context.Background(), n, []types.Target{target}, 1, time.Now())
	if e != nil || r.Items[0].Status != "unlocated" {
		t.Fatalf("domain override leaked=%+v %v", r, e)
	}
	r, e = engine.Analyze(context.Background(), n, []types.Target{target}, 2, time.Now())
	if e != nil || r.Items[0].Status != "allowed" {
		t.Fatalf("mapped domain not located=%+v %v", r, e)
	}
	target.Settings.Scopes = []types.ScopeBinding{{RoutingScope: "vrf", DomainID: 1}, {RoutingScope: "vrf", DomainID: 2}}
	r, e = engine.Analyze(context.Background(), n, []types.Target{target}, 2, time.Now())
	if e != nil || r.Items[0].Status != "unknown" || r.Complete {
		t.Fatalf("conflicting domain mappings arbitrarily selected=%+v %v", r, e)
	}
}

func TestDisabledProtectedInterfaceCannotExpandKnownStaticCoverage(t *testing.T) {
	data := graph(t, "paloalto", rule("allow", 1, fm.Permit)).Export()
	data.Interfaces[0].Disabled = true
	data.Routes = append(data.Routes, fm.RouteRecord{Object: fm.Object{ID: "attached", Scope: "vrf"}, Destination: netip.MustParsePrefix("10.50.0.0/16"), Interface: "in"})
	fw, e := fm.Seal(data)
	if e != nil {
		t.Fatal(e)
	}
	r, e := engine.Analyze(context.Background(), normalized(t, row("10.50.0.2", "8.8.8.8", "443")), []types.Target{target(fw)}, 1, time.Now())
	if e != nil || r.Complete || len(r.Items) != 1 || len(r.Items[0].Allowed) > 0 || r.Items[0].PacketStatus != "unknown" {
		t.Fatalf("disabled ingress authorized=%+v %v", r, e)
	}
}

func TestCoverageRejectsUnboundedNativeZoneCombinations(t *testing.T) {
	data := fm.Data{Metadata: fm.Metadata{Device: "synthetic", Vendor: "paloalto"}}
	for i := 0; i < 4100; i++ {
		data.Zones = append(data.Zones, fm.ZoneRecord{Object: fm.Object{ID: fm.ID(fmt.Sprintf("zone-%d", i))}})
	}
	fw, e := fm.Seal(data)
	if e != nil {
		t.Fatal(e)
	}
	_, e = engine.Coverage(context.Background(), types.Target{Firewall: fw, DomainID: 1})
	if !errors.Is(e, engine.ErrBudget) {
		t.Fatalf("unbounded zone combinations=%v", e)
	}
}

func TestMissingSnapshotPreservesOtherTargetResultsAndUnknownEvidence(t *testing.T) {
	imported := target(graph(t, "paloalto", rule("allow", 1, fm.Permit)))
	missing := types.Target{InstanceID: 12, Name: "not-imported", DomainID: 1, Mode: "routed", Settings: types.Settings{Version: 3}}
	coverage, e := engine.Coverage(context.Background(), missing)
	if e != nil || len(coverage) != 1 || coverage[0].Known || coverage[0].Origin != "missing_snapshot" || coverage[0].InstanceID != 12 {
		t.Fatalf("missing snapshot coverage=%+v %v", coverage, e)
	}
	n := normalized(t, row("10.0.0.2", "8.8.8.8", "443"))
	result, e := engine.Analyze(context.Background(), n, []types.Target{imported, missing}, 1, time.Now())
	if e != nil || result.Complete || len(result.Items) != 2 {
		t.Fatalf("missing snapshot aborted other targets=%+v %v", result, e)
	}
	var allowed, unknown bool
	for _, item := range result.Items {
		allowed = allowed || item.InstanceID == 11 && item.Status == "allowed"
		unknown = unknown || item.InstanceID == 12 && item.Status == "unknown" && item.Basis == "missing_snapshot" && len(item.Unknown) == 1 && item.Unknown[0].DestinationPort == "443"
	}
	if !allowed || !unknown {
		t.Fatalf("missing target was dropped=%+v", result)
	}
	missing.DomainID = 2
	result, e = engine.Analyze(context.Background(), n, []types.Target{imported, missing}, 1, time.Now())
	if e != nil || !result.Complete || len(result.Items) != 1 || result.Items[0].Status != "allowed" {
		t.Fatalf("unrelated missing domain tainted analysis=%+v %v", result, e)
	}
}

func TestAnalysisMixedExplicitFamiliesRetainsCompatibleTrafficAndIssues(t *testing.T) {
	fw := graph(t, "paloalto", rule("allow", 1, fm.Permit))
	n := normalized(t, row("10.0.0.2,2001:db8::2", "8.8.8.8", "443"))
	result, e := engine.Analyze(context.Background(), n, []types.Target{target(fw)}, 1, time.Now())
	if e != nil || result.Complete {
		t.Fatalf("unpaired IPv6 silently completed=%+v %v", result, e)
	}
	var allowed, issue bool
	for _, item := range result.Items {
		allowed = allowed || item.Status == "allowed" && len(item.Allowed) == 1 && item.Allowed[0].Source == "10.0.0.2"
		for _, notice := range item.Issues {
			issue = issue || notice.Code == "address_family_mismatch" && notice.Field == "source"
		}
	}
	if !allowed || !issue {
		t.Fatalf("compatible flow or family issue lost=%+v", result)
	}
	draft, e := engine.Design(context.Background(), n, []types.Target{target(fw)}, 1, time.Now())
	if e != nil || draft.Complete {
		t.Fatalf("unpaired family verified=%+v %v", draft, e)
	}
	for _, item := range draft.Items {
		if item.Status != "needs_review" || item.TargetSatisfied {
			t.Fatalf("unpaired family verified=%+v", item)
		}
	}
	wildcard := normalized(t, row("10.0.0.2", "any", "443"))
	a, e := engine.Analyze(context.Background(), wildcard, []types.Target{target(fw)}, 1, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	for _, item := range a.Items {
		for _, notice := range item.Issues {
			if notice.Code == "address_family_mismatch" {
				t.Fatalf("wildcard demanded cross-family translation=%+v", a)
			}
		}
	}
}
func TestAnalysisUnresolvedNativeNameRetainsEveryRelevantTarget(t *testing.T) {
	data := graph(t, "paloalto", rule("allow", 1, fm.Permit)).Export()
	data.Addresses = []fm.AddressRecord{{Object: fm.Object{ID: "clients"}, Ranges: []flow.AddressRange{{First: addr("10.0.0.2"), Last: addr("10.0.0.2")}}}}
	named, e := fm.Seal(data)
	if e != nil {
		t.Fatal(e)
	}
	one := target(named)
	two := target(graph(t, "paloalto", rule("allow", 1, fm.Permit)))
	two.InstanceID = 12
	n := normalized(t, row("clients", "8.8.8.8", "443"))
	result, e := engine.Analyze(context.Background(), n, []types.Target{one, two}, 1, time.Now())
	if e != nil || result.Complete || len(result.Items) != 2 {
		t.Fatalf("unresolved target name disappeared=%+v %v", result, e)
	}
	var allowed, unknown bool
	for _, item := range result.Items {
		allowed = allowed || item.InstanceID == 11 && item.Status == "allowed"
		for _, notice := range item.Issues {
			unknown = unknown || item.InstanceID == 12 && item.Status == "unknown" && notice.Code == "unresolved_name" && notice.Token == "clients"
		}
	}
	if !allowed || !unknown {
		t.Fatalf("per-target name evidence lost=%+v", result)
	}
	draft, e := engine.Design(context.Background(), n, []types.Target{one, two}, 1, time.Now())
	if e != nil || draft.Complete || len(draft.Items) != 2 {
		t.Fatalf("unresolved named target draft=%+v %v", draft, e)
	}
	for _, item := range draft.Items {
		if item.Status != "needs_review" || len(item.Changes) > 0 || item.TargetSatisfied {
			t.Fatalf("unresolved native name verified=%+v", item)
		}
	}
}
