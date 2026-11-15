// Package grouping owns the deterministic name rules shared by migration and sync.
package grouping

import (
	"regexp"
	"strings"
)

// RegionRule describes region rule for network operations.
type RegionRule struct{ Match, Code, Name string }

// Regions is ordered: when multiple markers occur, the first rule wins.
var Regions = []RegionRule{
	{"szx9", "site.transsion-tower", "传音大厦"},
	{"szx10", "site.shenzhen-yun-tech", "深圳云科技"},
	{"szx8", "site.shenzhen-baiwangxin", "深圳百旺信"},
	{"szx11", "site.shenzhen-chuanyiyuan", "深圳传意园"},
	{"sha", "site.shanghai", "上海职场"},
	{"ckg", "site.chongqing", "重庆职场"},
	{"", "site.other", "其他区域"},
}

func identity(name, serial string) string {
	// The source normalizes absent/null names to empty, so both use serial.
	if name == "" {
		return serial
	}
	return name
}

// Region provides the region operation within the caller's authorized network scope.
func Region(name, serial string) RegionRule {
	value := strings.ToLower(identity(name, serial))
	for _, rule := range Regions {
		if strings.Contains(value, rule.Match) {
			return rule
		}
	}
	return Regions[len(Regions)-1]
}

type classificationRule struct {
	prefix, role, kind string
	pattern            *regexp.Regexp
}

var classifications = func() []classificationRule {
	rules := []classificationRule{
		{prefix: "CSW", role: "core_switch", kind: "switch"},
		{prefix: "ASW", role: "aggregate_switch", kind: "switch"},
		{prefix: "PSW", role: "poe_switch", kind: "switch"},
		{prefix: "SW", role: "access_switch", kind: "switch"},
		{prefix: "LEF", role: "leaf_switch", kind: "switch"},
		{prefix: "BDR", role: "border_switch", kind: "switch"},
		{prefix: "SPN", role: "spine_switch", kind: "switch"},
		{prefix: "FW", role: "firewall", kind: "firewall"},
		{prefix: "LB", role: "load_balancer", kind: "load_balancer"},
		{prefix: "WLC", role: "wireless_controller", kind: "wireless_controller"},
		{prefix: "AP", role: "wireless_ap", kind: "wireless_ap"},
	}
	for i := range rules {
		rules[i].pattern = regexp.MustCompile(`(?i)(?:^|-)` + rules[i].prefix + `[0-9]*(?:$|-)`)
	}
	return rules
}()

// Classification provides the classification operation within the caller's authorized network scope.
func Classification(name, serial string) (role, kind string) {
	for _, rule := range classifications {
		if rule.pattern.MatchString(identity(name, serial)) {
			return rule.role, rule.kind
		}
	}
	return "other", "other"
}
