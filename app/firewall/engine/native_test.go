package engine_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/config"
	"github.com/gatework/firemon/flow"
	"github.com/gatework/firemon/vendors"
	"go-admin/app/firewall/engine"
	"go-admin/app/firewall/types"
)

func TestAnalysisExactPANDNATKeepsOriginalPolicyViewAndPostNATZone(t *testing.T) {
	data := graph(t, "paloalto").Export()
	data.Addresses = []fm.AddressRecord{{Object: fm.Object{ID: "public"}, Ranges: []flow.AddressRange{{First: addr("203.0.113.10"), Last: addr("203.0.113.10")}}}}
	match := fm.Match{AnySource: true, AnyDestination: false, Destinations: []fm.AddressRef{{Kind: fm.AddressObject, ID: "public"}}, AnyService: true, AnyFromZone: true, ToZones: []fm.ID{"outside"}}
	data.DynamicNATs = []fm.DynamicNATRecord{{Object: fm.Object{ID: "publish"}, Match: match, SourceMode: fm.NoTranslation, DestinationMode: fm.StaticIP, Destination: []fm.Mapping{{Original: flow.AddressRange{First: addr("203.0.113.10"), Last: addr("203.0.113.10")}, Translated: flow.AddressRange{First: addr("10.0.0.10"), Last: addr("10.0.0.10")}, Mode: fm.Constant}}}}
	match.ToZones = []fm.ID{"inside"}
	data.Rules = []fm.RuleRecord{{Object: fm.Object{ID: "permit-public"}, RuleSet: "security", Action: fm.Permit, Log: fm.Irrelevant, Match: match}}
	fw, e := fm.Seal(data)
	if e != nil {
		t.Fatal(e)
	}
	n := normalized(t, row("10.0.0.2", "203.0.113.10", "443"))
	r, e := engine.Analyze(context.Background(), n, []types.Target{target(fw)}, 1, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if !r.Complete || len(r.Items) != 1 {
		t.Fatalf("exact native packet result=%+v", r)
	}
	item := r.Items[0]
	if item.ToZone != "inside" || item.Status != "allowed" || item.PacketStatus != "allowed" || item.Allowed[0].Destination != "203.0.113.10" || item.Original[0].Destination != "203.0.113.10" || item.PolicyView != "native_packet_policy" || len(item.NAT) != 1 || !item.NAT[0].Exact || item.NAT[0].Translated[0].Destination != "10.0.0.10" {
		t.Fatalf("PAN original view/post NAT zone lost=%+v", item)
	}
	d, e := engine.Design(context.Background(), n, []types.Target{target(fw)}, 1, time.Now())
	if e != nil || d.Complete || d.Items[0].Status != "needs_review" {
		t.Fatalf("NAT view auto-planned=%+v %v", d, e)
	}
}
func TestAnalysisNativeASAConfigSeparatesWireAndRealPolicyTuple(t *testing.T) {
	raw := "interface i\n nameif inside\n security-level 100\n ip address 192.0.2.1 255.255.255.0\n!\ninterface o\n nameif outside\n security-level 0\n ip address 203.0.113.1 255.255.255.0\n!\nobject network server\n host 192.0.2.10\n nat (inside,outside) static 203.0.113.10 service tcp 443 8443\naccess-group IN in interface outside\naccess-list IN extended permit tcp any4 object server eq 443\n"
	fw, e := (vendors.Parser{}).Parse(context.Background(), config.Source{Text: raw, Complete: true}, fm.Metadata{Vendor: "asa", Device: "synthetic-asa", Software: "9.12"})
	if e != nil {
		t.Fatal(e)
	}
	if !fw.Complete() {
		t.Fatal(fw.Diagnostics())
	}
	target := types.Target{InstanceID: 22, DomainID: 1, Mode: "routed", Revision: "asa-r1", Firewall: fw, Settings: types.Settings{Zones: []types.Binding{{Zone: "outside", DomainID: 1, Protected: true, Networks: []string{"198.51.100.0/24"}}}}}
	r, e := engine.Analyze(context.Background(), normalized(t, row("198.51.100.2", "203.0.113.10", "tcp/1234/8443")), []types.Target{target}, 1, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Items) != 1 || r.Items[0].Status != "allowed" {
		t.Fatalf("native ACL not used=%+v", r)
	}
	item := r.Items[0]
	if len(item.NAT) != 1 || len(item.NAT[0].Steps) != 1 || !item.NAT[0].Steps[0].Reverse || item.NAT[0].Steps[0].Kind != "static_nat" {
		t.Fatalf("reverse native NAT step lost=%+v", item.NAT)
	}
	if item.Original[0].Destination != "203.0.113.10" || item.Original[0].DestinationPort != "8443" || item.Allowed[0].Destination != "192.0.2.10" || item.Allowed[0].DestinationPort != "443" || item.ToZone != "inside" || item.PacketStatus != "allowed" || item.PolicyView != "native_packet_policy" {
		t.Fatalf("ASA wire/ACL views conflated=%+v", item)
	}
}
func TestAnalysisNativeNameAmbiguityDoesNotWidenAndQualificationResolves(t *testing.T) {
	data := graph(t, "paloalto", rule("allow", 1, fm.Permit)).Export()
	data.Zones[0].AddressScope = "book1"
	data.Zones[1].AddressScope = "book2"
	data.Addresses = []fm.AddressRecord{{Object: fm.Object{ID: "book1-clients", Name: "clients", Scope: "book1"}, Ranges: []flow.AddressRange{{First: addr("10.0.0.2"), Last: addr("10.0.0.2")}}}, {Object: fm.Object{ID: "book2-clients", Name: "clients", Scope: "book2"}, Ranges: []flow.AddressRange{{First: addr("192.0.2.2"), Last: addr("192.0.2.2")}}}}
	fw, e := fm.Seal(data)
	if e != nil {
		t.Fatal(e)
	}
	r, e := engine.Analyze(context.Background(), normalized(t, row("clients", "8.8.8.8", "443")), []types.Target{target(fw)}, 1, time.Now())
	if e != nil || r.Complete || len(r.Items) != 1 || r.Items[0].Status != "unknown" || len(r.Items[0].Allowed) > 0 {
		t.Fatalf("ambiguous native name widened=%+v %v", r, e)
	}
	r, e = engine.Analyze(context.Background(), normalized(t, row("book1::clients", "8.8.8.8", "443")), []types.Target{target(fw)}, 1, time.Now())
	if e != nil || len(r.Items) != 1 || r.Items[0].Status != "allowed" || r.Items[0].Allowed[0].Source != "10.0.0.2" {
		t.Fatalf("qualified native name unresolved=%+v %v", r, e)
	}
}
func TestNoProtectedZoneAndOverlappingZoneRemainUnlocatedOrUnknown(t *testing.T) {
	target := target(graph(t, "paloalto", rule("allow", 1, fm.Permit)))
	target.Settings.Zones = nil
	r, e := engine.Analyze(context.Background(), normalized(t, row("10.0.0.2", "8.8.8.8", "443")), []types.Target{target}, 1, time.Now())
	if e != nil || r.Complete || len(r.Items) != 1 || r.Items[0].Status != "unlocated" {
		t.Fatalf("missing protection invented location=%+v %v", r, e)
	}
	target.Settings.Zones = []types.Binding{{Zone: "inside", RoutingScope: "vrf", DomainID: 1, Protected: true}, {Zone: "outside", RoutingScope: "vrf", DomainID: 1, Protected: true, Networks: []string{"10.0.0.0/24"}}}
	r, e = engine.Analyze(context.Background(), normalized(t, row("10.0.0.2", "8.8.8.8", "443")), []types.Target{target}, 1, time.Now())
	if e != nil || r.Complete || len(r.Items) != 1 || r.Items[0].Status != "unknown" || len(r.Items[0].Allowed) > 0 {
		t.Fatalf("overlapping zones arbitrarily selected=%+v %v", r, e)
	}
}
func TestNativeGroupExpansionHonorsCoreCombinationLimit(t *testing.T) {
	data := graph(t, "paloalto").Export()
	group := fm.AddressGroupRecord{Object: fm.Object{ID: "clients"}}
	for i := 0; i < 4100; i++ {
		id := fm.ID(fmt.Sprintf("a%d", i))
		data.Addresses = append(data.Addresses, fm.AddressRecord{Object: fm.Object{ID: id}, Ranges: []flow.AddressRange{{First: addr("10.0.0.2"), Last: addr("10.0.0.2")}}})
		group.Members = append(group.Members, fm.AddressRef{Kind: fm.AddressObject, ID: id})
	}
	data.AddressGroups = []fm.AddressGroupRecord{group}
	fw, e := fm.Seal(data)
	if e != nil {
		t.Fatal(e)
	}
	_, e = engine.Analyze(context.Background(), normalized(t, row("clients", "8.8.8.8", "443")), []types.Target{target(fw)}, 1, time.Now())
	if !errors.Is(e, engine.ErrBudget) {
		t.Fatalf("native expansion unbounded: %v", e)
	}
}

func TestDesignDoesNotVerifyOneACLWhenNativeOutboundACLStillDenies(t *testing.T) {
	raw := "interface i\n nameif inside\n security-level 100\n ip address 192.0.2.1 255.255.255.0\n!\ninterface o\n nameif outside\n security-level 0\n ip address 203.0.113.1 255.255.255.0\n!\naccess-list security extended permit tcp any4 host 192.0.2.10 eq 443\naccess-group security in interface outside\naccess-list OUT extended deny ip any4 any4\naccess-group OUT out interface inside\n"
	fw, e := (vendors.Parser{}).Parse(context.Background(), config.Source{Text: raw, Complete: true}, fm.Metadata{Vendor: "asa", Device: "synthetic-asa", Software: "9.12"})
	if e != nil {
		t.Fatal(e)
	}
	target := types.Target{InstanceID: 22, DomainID: 1, Mode: "routed", Revision: "asa-r1", Firewall: fw, Settings: types.Settings{Zones: []types.Binding{{Zone: "outside", DomainID: 1, Protected: true, Networks: []string{"198.51.100.0/24"}}}}}
	n := normalized(t, row("198.51.100.2", "192.0.2.10", "tcp/1234/443"))
	a, e := engine.Analyze(context.Background(), n, []types.Target{target}, 1, time.Now())
	if e != nil || len(a.Items) != 1 || a.Items[0].PacketStatus != "denied" {
		t.Fatalf("fixture must be blocked by outbound ACL=%+v %v", a, e)
	}
	r, e := engine.Design(context.Background(), n, []types.Target{target}, 1, time.Now())
	if e != nil || r.Complete || len(r.Items) != 1 || r.Items[0].Status != "needs_review" || len(r.Items[0].Changes) > 0 || r.Items[0].TargetSatisfied {
		t.Fatalf("single-chain proof ignored outbound ACL=%+v %v", r, e)
	}
}

func TestAnalysisNATPipelinePreservesEveryNativeStep(t *testing.T) {
	data := graph(t, "hillstone", rule("allow", 1, fm.Permit)).Export()
	data.Metadata.Software = "5.5R8"
	data.Addresses = []fm.AddressRecord{{Object: fm.Object{ID: "public"}, Ranges: []flow.AddressRange{{First: addr("203.0.113.10"), Last: addr("203.0.113.10")}}}}
	match := fm.Match{AnySource: true, Destinations: []fm.AddressRef{{Kind: fm.AddressObject, ID: "public"}}, AnyService: true, AnyFromZone: true, AnyToZone: true}
	data.NATPools = []fm.NATPoolRecord{{Object: fm.Object{ID: "pool"}, Ranges: []flow.AddressRange{{First: addr("198.51.100.1"), Last: addr("198.51.100.1")}}}}
	data.DynamicNATs = []fm.DynamicNATRecord{{Object: fm.Object{ID: "source"}, Match: match, Order: fm.NATOrder{Stage: 2, Section: "snat", Position: 1}, SourceMode: fm.DynamicIPPort, DestinationMode: fm.NoTranslation, SourcePool: "pool"}, {Object: fm.Object{ID: "destination"}, Match: match, Order: fm.NATOrder{Stage: 1, Section: "dnat", Position: 2}, SourceMode: fm.NoTranslation, DestinationMode: fm.StaticIP, Destination: []fm.Mapping{{Original: flow.AddressRange{First: addr("203.0.113.10"), Last: addr("203.0.113.10")}, Translated: flow.AddressRange{First: addr("10.0.0.10"), Last: addr("10.0.0.10")}, Mode: fm.Constant}}}}
	fw, e := fm.Seal(data)
	if e != nil {
		t.Fatal(e)
	}
	r, e := engine.Analyze(context.Background(), normalized(t, row("10.0.0.2", "203.0.113.10", "443")), []types.Target{target(fw)}, 1, time.Now())
	if e != nil || len(r.Items) != 1 || len(r.Items[0].NAT) != 1 {
		t.Fatalf("native pipeline=%+v %v", r, e)
	}
	trace := r.Items[0].NAT[0]
	if trace.Kind != "pipeline" || len(trace.Steps) != 2 || trace.Steps[0].ID != "destination" || trace.Steps[1].ID != "source" || !trace.PossibleAllocation || trace.Exact {
		t.Fatalf("pipeline mislabeled or collapsed=%+v", trace)
	}
}
