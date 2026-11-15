package wireless

import (
	"context"
	"sort"
	"strings"

	"github.com/gatework/motor-core/sdk/contract/actions"
	models "github.com/gatework/motor/app/net/models/wireless"
	dto "github.com/gatework/motor/app/net/service/dto/wireless"
)

// Analysis reports explicit device outcomes and acquisition coverage.
// Analysis provides the analysis operation within the caller's authorized wireless scope.
func (s *Wireless) Analysis(ctx context.Context, id int, p *actions.DataPermission) (dto.Analysis, error) {
	out := dto.Analysis{ControllerID: id, Groups: []dto.IssueGroup{}, SSIDs: []dto.Distribution{}}
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
	groups, ssids := analysisGroups(rows)
	for _, g := range groups {
		out.Groups = append(out.Groups, *g)
	}
	sort.Slice(out.Groups, func(i, j int) bool {
		a, b := out.Groups[i], out.Groups[j]
		return strings.Join([]string{a.Check, a.SSID, a.Band}, "|") < strings.Join([]string{b.Check, b.SSID, b.Band}, "|")
	})
	for name, count := range ssids {
		out.SSIDs = append(out.SSIDs, dto.Distribution{SSID: name, Count: count})
	}
	sort.Slice(out.SSIDs, func(i, j int) bool { return out.SSIDs[i].SSID < out.SSIDs[j].SSID })
	return out, nil
}

func analysisGroups(rows []dto.Resource) (map[string]*dto.IssueGroup, map[string]int) {
	groups := map[string]*dto.IssueGroup{}
	ssids := map[string]int{}
	for _, r := range rows {
		var check string
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
			g = &dto.IssueGroup{Key: key, Check: check, SSID: r.Fields.SSID, Band: r.Fields.Band, Outcomes: []dto.Outcome{}, Suggestion: dto.Suggestion{Dimensions: []string{r.Kind}, APNames: []string{}}}
			groups[key] = g
		}
		outcome := dto.Outcome{ResourceID: r.ID, Kind: r.Kind, APName: r.Fields.APName, Status: r.Health, Reason: r.Coverage.Reason}
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
	return groups, ssids
}
