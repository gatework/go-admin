package engine_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/flow"
	"github.com/gatework/motor/app/net/models/firewall/types"
	"github.com/gatework/motor/app/net/service/firewall/engine"
)

func TestDesignAccumulatesNativeContextsFromFixedBaseAndOmitsCommands(t *testing.T) {
	fw := graph(t, "paloalto")
	base := fw.SemanticDigest()
	n := normalized(t, row("10.0.0.2", "8.8.8.8", "tcp/1024-65535/443"), row("10.0.0.2", "10.0.0.3", "tcp/80"))
	result, err := engine.Design(context.Background(), n, []types.Target{target(fw)}, 1, time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || len(result.Items) != 1 {
		t.Fatalf("design=%+v", result)
	}
	item := result.Items[0]
	if item.Status != "verified" || len(item.Changes) != 2 || item.BaseDigest != base || item.ResultDigest == base || !item.TargetSatisfied || !item.OutsidePreserved || !item.NATUnchanged || len(item.RowKeys) != 2 || item.SettingsVersion != 7 {
		t.Fatalf("cumulative design missing proof=%+v", item)
	}
	if fw.SemanticDigest() != base || len(fw.Rules()) != 0 {
		t.Fatal("immutable base mutated")
	}
	encoded, _ := json.Marshal(result)
	for _, forbidden := range []string{"commands", "configure", "set rulebase", "commit"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("command payload leaked: %s", encoded)
		}
	}
}
func TestDesignUnprovenAndMalformedInputRemainReviewable(t *testing.T) {
	for _, tc := range []struct{ name, vendor, src, dst string }{{"unsupported-packet-view", "hillstone", "10.0.0.2", "8.8.8.8"}, {"unknown-input", "paloalto", "10.0.0.2,999.1.1.1", "8.8.8.8"}, {"inferred-ingress", "paloalto", "8.8.8.8", "10.0.0.2"}, {"unlocated", "paloalto", "172.31.0.2", "8.8.8.8"}} {
		t.Run(tc.name, func(t *testing.T) {
			fw := graph(t, tc.vendor)
			r, e := engine.Design(context.Background(), normalized(t, row(tc.src, tc.dst, "443")), []types.Target{target(fw)}, 1, time.Now())
			if e != nil {
				t.Fatal(e)
			}
			if r.Complete || len(r.Items) == 0 {
				t.Fatalf("unsafe success=%+v", r)
			}
			for _, v := range r.Items {
				if v.Status != "needs_review" || len(v.Changes) > 0 || v.TargetSatisfied {
					t.Fatalf("unproven candidate=%+v", v)
				}
			}
		})
	}
}
func TestDesignNativeDynamicAllocationDoesNotInventSessionMapping(t *testing.T) {
	data := graph(t, "paloalto", rule("allow", 1, fm.Permit)).Export()
	data.NATPools = []fm.NATPoolRecord{{ID: "pool", Ranges: []flow.AddressRange{{First: addr("203.0.113.1"), Last: addr("203.0.113.8")}}, Ports: flow.Interval{Low: 1024, High: 65535}}}
	data.DynamicNATs = []fm.DynamicNATRecord{{ID: "snat", Match: fm.Match{AnySource: true, AnyDestination: true, AnyService: true, AnyFromZone: true, AnyToZone: true}, SourceMode: fm.DynamicIPPort, DestinationMode: fm.NoTranslation, SourcePool: "pool"}}
	fw, e := fm.Seal(data)
	if e != nil {
		t.Fatal(e)
	}
	n := normalized(t, row("10.0.0.2", "8.8.8.8", "443"))
	a, e := engine.Analyze(context.Background(), n, []types.Target{target(fw)}, 1, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if len(a.Items) != 1 || len(a.Items[0].NAT) != 1 || !a.Items[0].NAT[0].PossibleAllocation || a.Items[0].PacketStatus != "unknown" {
		t.Fatalf("allocation presented as observed binding=%+v", a)
	}
	d, e := engine.Design(context.Background(), n, []types.Target{target(fw)}, 1, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if d.Complete || d.Items[0].Status != "needs_review" || len(d.Items[0].Changes) > 0 {
		t.Fatalf("NAT plan invented certainty=%+v", d)
	}
}
func TestDesignLimitsAndCancellation(t *testing.T) {
	rows := make([]types.InputRow, 101)
	for i := range rows {
		rows[i] = row("10.0.0.2", "8.8.8.8", "443")
	}
	fw := graph(t, "paloalto")
	if _, e := engine.Design(context.Background(), normalized(t, rows...), []types.Target{target(fw)}, 1, time.Now()); !errors.Is(e, engine.BudgetErr) {
		t.Fatalf("design limit=%v", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := engine.Design(ctx, normalized(t, row("10.0.0.2", "8.8.8.8", "443")), []types.Target{target(fw)}, 1, time.Now()); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}

func addr(value string) netip.Addr { return netip.MustParseAddr(value) }

func TestDesignPreservesPairedRowsWithoutCrossRowCartesianTraffic(t *testing.T) {
	fw := graph(t, "paloalto")
	n := normalized(t, row("10.0.0.2", "8.8.8.8", "tcp/80"), row("10.0.0.3", "9.9.9.9", "tcp/443"))
	draft, e := engine.Design(context.Background(), n, []types.Target{target(fw)}, 1, time.Now())
	if e != nil || !draft.Complete || len(draft.Items) != 1 || len(draft.Items[0].Changes) != 2 {
		t.Fatalf("paired draft=%+v %v", draft, e)
	}
	data := fw.Export()
	addresses := map[fm.ID]bool{}
	services := map[fm.ID]bool{}
	for _, change := range draft.Items[0].Changes {
		var semantic struct {
			Rule      fm.RuleRecord      `json:"rule"`
			Addresses []fm.AddressRecord `json:"addresses"`
			Services  []fm.ServiceRecord `json:"services"`
		}
		if e = json.Unmarshal(change.Data, &semantic); e != nil {
			t.Fatal(e)
		}
		data.Rules = append(data.Rules, semantic.Rule)
		for _, a := range semantic.Addresses {
			if !addresses[a.ID] {
				data.Addresses = append(data.Addresses, a)
				addresses[a.ID] = true
			}
		}
		for _, s := range semantic.Services {
			if !services[s.ID] {
				data.Services = append(data.Services, s)
				services[s.ID] = true
			}
		}
	}
	proposed, e := fm.Seal(data)
	if e != nil {
		t.Fatal(e)
	}
	if proposed.SemanticDigest() != draft.Items[0].ResultDigest {
		t.Fatal("semantic draft does not reproduce final cumulative graph")
	}
	for _, r := range []types.InputRow{row("10.0.0.2", "9.9.9.9", "80"), row("10.0.0.3", "8.8.8.8", "443"), row("10.0.0.2", "8.8.8.8", "443")} {
		a, e := engine.Analyze(context.Background(), normalized(t, r), []types.Target{target(proposed)}, 1, time.Now())
		if e != nil || a.Items[0].Status != "unmatched" {
			t.Fatalf("unrequested cross-row tuple allowed=%+v %v", a, e)
		}
	}
}

func TestDesignMissingRelevantSnapshotRetainsPendingTargetsAndBlocksVerification(t *testing.T) {
	imported := target(graph(t, "paloalto"))
	missing := types.Target{InstanceID: 12, Name: "not-imported", DomainID: 1, Mode: "routed", Settings: types.Settings{Version: 3}}
	n := normalized(t, row("10.0.0.2", "8.8.8.8", "443"))
	result, e := engine.Design(context.Background(), n, []types.Target{imported, missing}, 1, time.Now())
	if e != nil || result.Complete || len(result.Items) != 2 {
		t.Fatalf("missing snapshot aborted or disappeared from draft=%+v %v", result, e)
	}
	seen := map[int]bool{}
	for _, item := range result.Items {
		seen[item.InstanceID] = true
		if item.Status != "needs_review" || len(item.Changes) > 0 || item.TargetSatisfied || len(item.RowKeys) != 1 || item.RowKeys[0] != n.Rows[0].Key {
			t.Fatalf("draft verified unresolved relevant target=%+v", item)
		}
		if item.InstanceID == 12 && item.SettingsVersion != 3 {
			t.Fatalf("missing target version lost=%+v", item)
		}
	}
	if !seen[11] || !seen[12] {
		t.Fatalf("target identity lost=%+v", result)
	}
	missing.DomainID = 2
	result, e = engine.Design(context.Background(), n, []types.Target{imported, missing}, 1, time.Now())
	if e != nil || !result.Complete || len(result.Items) != 1 || result.Items[0].Status != "verified" {
		t.Fatalf("unrelated missing snapshot blocked valid domain=%+v %v", result, e)
	}
}
