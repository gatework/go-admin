package grouping

import (
	"testing"
)

func TestResolveMatchesGroupMarkersInPositionOrder(t *testing.T) {
	groups := Directory{
		{ID: 1, Code: "site.transsion-tower", Marker: "szx9", Enabled: true},
		{ID: 2, Code: "site.shanghai", Marker: "sha", Enabled: true},
		{ID: 3, Code: "site.lab", Marker: "LAB", Enabled: true},
		{ID: 4, Code: "site.closed", Marker: "old", Enabled: false},
		{ID: 5, Code: FallbackCode, Enabled: true},
	}
	five := 4
	for _, test := range []struct {
		name, serial string
		current      *int
		want         int // 0: no group
	}{
		{"SZX9-SW01", "", nil, 1},
		{"sha-szx9", "", nil, 1}, // the earlier group wins
		{"SHA-AP1", "", nil, 2},
		{"x-lab-fw1", "", nil, 3},    // a custom group attracts devices by its marker
		{"ordinary", "SZX9", nil, 5}, // a named device is matched by name only
		{"", "SZX9-SW1", nil, 1},     // a nameless one by serial
		{"", "", nil, 5},
		{"old-sw1", "", nil, 0},   // a disabled group takes no new members
		{"old-sw1", "", &five, 4}, // but keeps the ones it has
	} {
		got := groups.Resolve(test.name, test.serial, test.current)
		if (got == nil) != (test.want == 0) || got != nil && *got != test.want {
			t.Errorf("Resolve(%q, %q) = %v, want %d", test.name, test.serial, got, test.want)
		}
	}
}

func TestClassificationUsesCompleteSegmentsAndOrderedRules(t *testing.T) {
	for _, test := range []struct{ name, serial, role, kind string }{
		{"SZX9-CSW01", "", "core_switch", "switch"}, {"asw2", "", "aggregate_switch", "switch"},
		{"x-PSW123-y", "", "poe_switch", "switch"}, {"SW", "", "access_switch", "switch"},
		{"x-LEF2", "", "leaf_switch", "switch"}, {"BDR1", "", "border_switch", "switch"},
		{"SPN", "", "spine_switch", "switch"}, {"x-fw1", "", "firewall", "firewall"},
		{"LB2-x", "", "load_balancer", "load_balancer"}, {"WLC1", "", "wireless_controller", "wireless_controller"},
		{"AP11", "", "wireless_ap", "wireless_ap"}, {"x-AP2-CSW1", "", "core_switch", "switch"},
		{"LAPTOP", "", "other", "other"}, {"SWITCH", "", "other", "other"}, {"XASW1", "", "other", "other"},
		{"x_SW1", "", "other", "other"}, {"SW1x", "", "other", "other"}, {"normal", "CSW1", "other", "other"},
		{"", "SZX9-ASW01", "aggregate_switch", "switch"}, {"", "", "other", "other"},
	} {
		role, kind := Classification(test.name, test.serial)
		if role != test.role || kind != test.kind {
			t.Errorf("Classification(%q,%q)=(%q,%q) want (%q,%q)", test.name, test.serial, role, kind, test.role, test.kind)
		}
	}
}
