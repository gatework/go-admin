package wireless

import (
	"context"
	"encoding/csv"
	"strconv"
	"strings"
	"time"

	dto "github.com/gatework/motor/app/net/service/dto/wireless"

	"github.com/gatework/motor-core/sdk/contract/actions"
	models "github.com/gatework/motor/app/net/models/wireless"
	"github.com/gatework/motor/common/csvsafe"
)

// ExportManagement provides the export management operation within the caller's authorized wireless scope.
func (s *Wireless) ExportManagement(ctx context.Context, controller int, p *actions.DataPermission) (string, error) {
	if _, err := s.controller(ctx, controller, p); err != nil {
		return "", err
	}
	var rows []models.Resource
	if err := s.db(ctx).Where("controller_id = ? AND kind = ?", controller, "access_points").Order("id").Find(&rows).Error; err != nil {
		return "", err
	}
	var result strings.Builder
	out := csv.NewWriter(&result)
	if err := out.Write([]string{"id", "version", "localAlias", "installationLocation", "owner", "notes"}); err != nil {
		return "", err
	}
	for _, r := range rows {
		if err := out.Write([]string{strconv.Itoa(r.ID), strconv.FormatInt(r.Version, 10), csvsafe.Cell(r.LocalAlias), csvsafe.Cell(r.InstallationLocation), csvsafe.Cell(r.Owner), csvsafe.Cell(r.Notes)}); err != nil {
			return "", err
		}
	}
	out.Flush()
	if err := out.Error(); err != nil {
		return "", err
	}
	return result.String(), nil
}

// Export applies the same scoped filters as the inventory, across every page.
func (s *Wireless) Export(ctx context.Context, id int, kind string, r *dto.PageReq, p *actions.DataPermission) (string, error) {
	if kind == "management" {
		return s.ExportManagement(ctx, id, p)
	}
	if kind != "access_points" && kind != "radios" && kind != "bsses" && kind != "clients" {
		return "", InvalidErr
	}
	var b strings.Builder
	w := csv.NewWriter(&b)
	if e := w.Write([]string{"id", "controllerId", "apName", "radioId", "ssid", "vlan", "band", "ip", "mac", "status", "observedAt", "coverage", "missing"}); e != nil {
		return "", e
	}
	req := *r
	req.PageSize = 200
	for page := 1; ; page++ {
		req.PageIndex = page
		rows, n, e := s.ResourcePage(ctx, id, kind, &req, p)
		if e != nil {
			return "", e
		}
		for _, row := range rows {
			f := row.Fields
			values := []string{strconv.Itoa(row.ID), strconv.Itoa(row.ControllerID), f.APName, f.RadioID, f.SSID, f.VLAN, f.Band, f.IP, f.MAC, f.Status, row.ObservedAt.Format(time.RFC3339), row.Coverage.State, strconv.FormatBool(row.Missing)}
			for i := range values {
				values[i] = csvsafe.Cell(values[i])
			}
			if e = w.Write(values); e != nil {
				return "", e
			}
		}
		if int64(page*200) >= n {
			break
		}
	}
	w.Flush()
	return b.String(), w.Error()
}
