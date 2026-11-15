package grouping

import "testing"

func TestRegionRulesUseOrderedNameThenAbsentNameSerial(t *testing.T) {
	for _, test := range []struct{ name, serial, code string }{
		{"SZX9-SW01", "", "site.transsion-tower"}, {"szx10-FW1", "", "site.shenzhen-yun-tech"},
		{"SZX8", "", "site.shenzhen-baiwangxin"}, {"szx11", "", "site.shenzhen-chuanyiyuan"},
		{"SHA-AP1", "", "site.shanghai"}, {"ckg", "", "site.chongqing"},
		{"sha-szx9", "", "site.transsion-tower"}, {"ordinary", "SZX9", "site.other"},
		{"", "SZX9-SW1", "site.transsion-tower"}, {"", "", "site.other"},
	} {
		if got := Region(test.name, test.serial).Code; got != test.code {
			t.Errorf("Region(%q,%q)=%q want %q", test.name, test.serial, got, test.code)
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
