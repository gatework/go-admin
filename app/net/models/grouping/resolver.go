// Package grouping assigns devices to groups and classifies them from their
// names, the same way in the inventory migration and in every sync.
package grouping

import (
	"regexp"
)

// FallbackCode is the group a device joins when no group's marker matches it.
const FallbackCode = "site.other"

// standardSite is a site group the inventory migration seeds, with the marker
// it seeds it with. Sync matches the markers stored on the groups, so editing
// a group changes assignment without a release; this list only seeds.
type standardSite struct{ Marker, Code, Name string }

// standardSites is in seeding order, which becomes the groups' positions.
var standardSites = []standardSite{
	{"szx9", "site.transsion-tower", "传音大厦"},
	{"szx10", "site.shenzhen-yun-tech", "深圳云科技"},
	{"szx8", "site.shenzhen-baiwangxin", "深圳百旺信"},
	{"szx11", "site.shenzhen-chuanyiyuan", "深圳传意园"},
	{"sha", "site.shanghai", "上海职场"},
	{"ckg", "site.chongqing", "重庆职场"},
	{"", FallbackCode, "其他区域"},
}

// identity is what markers and classification rules match: the device name,
// or its serial when the source has no name.
func identity(name, serial string) string {
	if name == "" {
		return serial
	}
	return name
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
