package service

import (
	"context"
	"sort"
	"strings"

	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"go-admin/app/wireless/models"
	"go-admin/app/wireless/service/dto"
)

// Resources always resolves the owning controller through the caller's data scope.
func (s *Wireless) Resources(ctx context.Context, controller int, kind string, req *dto.PageReq, p *actions.DataPermission) ([]models.Resource, int64, error) {
	if !resourceKinds[kind] || page(req) != nil {
		return nil, 0, ErrInvalid
	}
	if _, err := s.controller(ctx, controller, p); err != nil {
		return nil, 0, err
	}
	var rows []models.Resource
	if err := s.db(ctx).Where("controller_id = ? AND kind = ?", controller, kind).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	filtered := filterResources(rows, req)
	switch req.Sort {
	case "", "id":
		sort.Slice(filtered, func(i, j int) bool {
			if req.Direction == "desc" {
				return filtered[i].ID > filtered[j].ID
			}
			return filtered[i].ID < filtered[j].ID
		})
	default:
		return nil, 0, ErrInvalid
	}
	out, count := slicePage(filtered, req)
	return out, count, nil
}

func filterResources(rows []models.Resource, req *dto.PageReq) []models.Resource {
	filtered := rows[:0]
	for _, row := range rows {
		if req.ExactAP && req.AP != "" && text(row.Facts["ap_name"]) != req.AP && text(row.Facts["name"]) != req.AP {
			continue
		}
		matches := func(query string, keys ...string) bool {
			if query == "" {
				return true
			}
			for _, k := range keys {
				if contains(text(row.Facts[k]), query) {
					return true
				}
			}
			return false
		}
		if !matches(req.IP, "ip_address", "ip", "client_ip", "client_ipv6", "management_ip") || !matches(req.MAC, "mac_address", "client_mac", "bssid") || !matches(req.AP, "ap_name", "name", "ap_id") || !matches(req.SSID, "ssid") || !matches(req.VLAN, "vlan", "vlan_id") || !matches(req.Band, "band") || !matches(req.Status, "status", "operational_status") {
			continue
		}
		if req.Keyword != "" && !strings.Contains(strings.ToLower(text(row.Facts)+row.LocalAlias+row.InstallationLocation+row.Owner), strings.ToLower(req.Keyword)) {
			continue
		}
		filtered = append(filtered, row)
	}

	return filtered
}
