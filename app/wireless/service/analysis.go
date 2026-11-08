package service

import (
	"context"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"go-admin/app/wireless/models"
	"go-admin/app/wireless/service/dto"
	"sort"
	"strings"
)

// Checks report explicit state and collection coverage; no invented RF thresholds.
func (s *Wireless) Analysis(ctx context.Context, id int, p *actions.DataPermission) (dto.WirelessAnalysis, error) {
	out := dto.WirelessAnalysis{ControllerID: id, Groups: []dto.WirelessIssueGroup{}, SSIDs: []dto.WirelessDistribution{}}
	controller, e := s.Controller(ctx, id, p)
	if e != nil {
		return out, e
	}
	out.Coverage = controller.Coverage
	var resources []models.Resource
	if e = s.db(ctx).Where("controller_id = ?", id).Order("id").Find(&resources).Error; e != nil {
		return out, e
	}
	rows, e := s.resourceViews(ctx, resources, p)
	if e != nil {
		return out, e
	}
	groups := map[string]*dto.WirelessIssueGroup{}
	ssids := map[string]int{}
	for _, r := range rows {
		check := ""
		switch r.Kind {
		case "access_points":
			check = "ap_availability"
		case "radios":
			check = "radio_availability"
		case "clients":
			check = "client_association"
		default:
			continue
		}
		key := hash([]string{check, r.Fields.SSID, r.Fields.Band})
		g := groups[key]
		if g == nil {
			g = &dto.WirelessIssueGroup{Key: key, Check: check, SSID: r.Fields.SSID, Band: r.Fields.Band, Outcomes: []dto.WirelessOutcome{}, Suggestion: dto.WirelessSuggestion{Dimensions: []string{r.Kind}, APNames: []string{}}}
			groups[key] = g
		}
		outcome := dto.WirelessOutcome{ResourceID: r.ID, Kind: r.Kind, APName: r.Fields.APName, Status: r.Health, Reason: r.Coverage.Reason}
		if r.ParentStatus == "orphan" {
			outcome.Reason = "ap_parent_unresolved"
		}
		if r.Missing {
			outcome.Reason = "missing_from_complete_observation"
		}
		g.Outcomes = append(g.Outcomes, outcome)
		switch r.Health {
		case "affected":
			g.Affected++
		case "healthy":
			g.Healthy++
		case "notApplicable":
			g.NotApplicable++
		default:
			g.Unknown++
		}
		if r.Kind == "clients" && !r.Missing {
			ssids[r.Fields.SSID]++
		}
	}
	for _, g := range groups {
		out.Groups = append(out.Groups, *g)
	}
	sort.Slice(out.Groups, func(i, j int) bool {
		a, b := out.Groups[i], out.Groups[j]
		return strings.Join([]string{a.Check, a.SSID, a.Band}, "|") < strings.Join([]string{b.Check, b.SSID, b.Band}, "|")
	})
	for name, count := range ssids {
		out.SSIDs = append(out.SSIDs, dto.WirelessDistribution{SSID: name, Count: count})
	}
	sort.Slice(out.SSIDs, func(i, j int) bool { return out.SSIDs[i].SSID < out.SSIDs[j].SSID })
	return out, nil
}
