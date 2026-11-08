package service

import (
	"regexp"
	"strings"

	"github.com/gatework/go-net-connector/connector"
	"github.com/gatework/go-net-connector/vendors"
	"github.com/gatework/go-net-connector/vendors/dialect"
	inventory "go-admin/app/network/models"
)

var unsupportedPlatform = regexp.MustCompile(`(?i)\b(?:ios[\s_-]*xr|asa\d*|firepower|ftd|aci|catos|screenos|fortimanager|fortianalyzer|linux|windows)\b`)
var ciscoIOSModel = regexp.MustCompile(`(?i)\b(?:catalyst\s+\d|isr[\s-]*\d|(?:ws[- ]c|c)(?:29|35|37|38|45|65|92|93|94|95|96)\d{2})`)
var platformOS = []struct {
	pattern *regexp.Regexp
	family  string
}{
	{regexp.MustCompile(`(?i)\b(?:nx[\s_-]*os|nexus)\b`), "cisco"},
	{regexp.MustCompile(`(?i)\bios(?:[\s_-]*xe)?\b`), "cisco"},
	{regexp.MustCompile(`(?i)\bcomware\b`), "h3c"},
	{regexp.MustCompile(`(?i)\bvrp\b`), "huawei"},
	{regexp.MustCompile(`(?i)\bpan[\s_-]*os\b`), "palo_alto"},
	{regexp.MustCompile(`(?i)\bjunos\b`), "juniper"},
	{regexp.MustCompile(`(?i)\bfortios\b`), "fortinet"},
	{regexp.MustCompile(`(?i)\bstoneos\b`), "hillstone"},
}

// inferProfile reuses the connector inventory rules, adding local classification
// and fail-closed guards for unsupported or contradictory platform evidence.
// No connection or discovery is performed by the resolver.
func inferProfile(d inventory.Device) string {
	if unsupportedPlatform.MatchString(d.OS + " " + d.HardwareModel) {
		return ""
	}
	registry := vendors.NewBuiltinRegistry()
	in := dialect.InventoryIdentity{Vendor: d.Vendor, OS: d.OS, Model: d.HardwareModel}
	profile, _ := registry.ResolvePlatform(in)
	if strings.HasPrefix(profile, "cisco_") || strings.Contains(strings.ToLower(d.Vendor), "cisco") {
		osOnly, modelOnly := in, in
		osOnly.Model, modelOnly.OS = "", ""
		osProfile, _ := registry.ResolvePlatform(osOnly)
		modelProfile, _ := registry.ResolvePlatform(modelOnly)
		if modelProfile == "" && ciscoIOSModel.MatchString(d.HardwareModel) {
			modelProfile = "cisco_ios"
		}
		if strings.TrimSpace(d.OS) != "" && osProfile == "" || osProfile != "" && modelProfile != "" && osProfile != modelProfile {
			return ""
		}
		if osProfile != "" {
			profile = osProfile
		} else {
			profile = modelProfile
		}
	}
	if profile == "h3c" && (d.DeviceType == "wireless_controller" || d.DeviceType == "wireless_ap" || d.DeviceType == "" && (d.DeviceRole == "wireless_controller" || d.DeviceRole == "wireless_ap")) {
		profile = "h3c_wireless"
	}
	for _, os := range platformOS {
		if os.pattern.MatchString(d.OS) && profile != os.family && !strings.HasPrefix(profile, os.family+"_") {
			return ""
		}
	}
	if _, err := registry.Lookup(profile); err != nil {
		return ""
	}
	return profile
}

func connectorProfile(name string) (connector.Profile, error) {
	platform, err := vendors.NewBuiltinRegistry().Lookup(name)
	if err != nil {
		return connector.Profile{}, err
	}
	return platform.Profile(), nil
}
