package service

import (
	"context"
	"errors"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	network "go-admin/app/network/models"
	inventory "go-admin/app/network/service"
	ndto "go-admin/app/network/service/dto"
	"go-admin/app/wireless/models"
	"go-admin/app/wireless/service/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func (s *Wireless) Groups(ctx context.Context, p *actions.DataPermission) (ndto.DeviceGroupTree, error) {
	g := inventory.DeviceGroup{Service: s.Service}
	return g.TreeForDeviceType(ctx, p, "wireless_controller")
}
func (s *Wireless) Switches(ctx context.Context, r *dto.PageReq, p *actions.DataPermission) ([]dto.WirelessSwitch, int64, error) {
	if e := page(r); e != nil {
		return nil, 0, e
	}
	q := s.devices(ctx, p).Where("device_type = ? AND status = ?", "switch", "active")
	if r.Keyword != "" {
		q = q.Where("LOWER(name) LIKE ? OR ip LIKE ?", "%"+strings.ToLower(r.Keyword)+"%", "%"+r.Keyword+"%")
	}
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	var rows []network.Device
	if e := q.Order("id").Offset((r.PageIndex - 1) * r.PageSize).Limit(r.PageSize).Find(&rows).Error; e != nil {
		return nil, 0, e
	}
	out := []dto.WirelessSwitch{}
	for _, d := range rows {
		out = append(out, dto.WirelessSwitch{ID: d.Id, Name: d.Name, IP: d.IP})
	}
	return out, n, nil
}
func (s *Wireless) Clients(ctx context.Context, r *dto.PageReq, p *actions.DataPermission) ([]dto.WirelessResource, int64, error) {
	if e := page(r); e != nil {
		return nil, 0, e
	}
	ids := s.devices(ctx, p).Select("id").Where("device_type = ?", "wireless_controller")
	var rows []models.Resource
	if e := s.db(ctx).Where("controller_id IN (?) AND kind = ?", ids, "clients").Order("id").Find(&rows).Error; e != nil {
		return nil, 0, e
	}
	rows = filterResources(rows, r)
	selected, n := slicePage(rows, r)
	v, e := s.resourceViews(ctx, selected, p)
	return v, n, e
}
func (s *Wireless) APDetail(ctx context.Context, id, apID int, p *actions.DataPermission) (dto.WirelessAPDetail, error) {
	out := dto.WirelessAPDetail{Neighbors: []dto.WirelessNeighbor{}, Radios: []dto.WirelessResource{}, BSSes: []dto.WirelessResource{}, Clients: []dto.WirelessResource{}, Counts: map[string]int64{}, NeighborCoverage: dto.WirelessCoverage{Name: "lldp", Status: "not_requested", State: "unknown", Reason: "not_requested"}}
	d, e := s.controller(ctx, id, p)
	if e != nil {
		return out, e
	}
	var ap models.Resource
	if e = s.db(ctx).Where("id = ? AND controller_id = ? AND kind = ?", apID, id, "access_points").First(&ap).Error; e != nil {
		return out, ErrScope
	}
	views, e := s.resourceViews(ctx, []models.Resource{ap}, p)
	if e != nil {
		return out, e
	}
	out.Resource = views[0]
	name := text(ap.Facts["name"])
	for _, kind := range []string{"radios", "bsses", "clients"} {
		rows, _, e := s.Resources(ctx, id, kind, &dto.PageReq{PageSize: 200, AP: name, ExactAP: true}, p)
		if e != nil {
			return out, e
		}
		scopedRows := rows[:0]
		for _, row := range rows {
			if row.ParentResourceID == nil || *row.ParentResourceID == ap.ID {
				scopedRows = append(scopedRows, row)
			}
		}
		v, e := s.resourceViews(ctx, scopedRows, p)
		if e != nil {
			return out, e
		}
		switch kind {
		case "radios":
			out.Radios = v
		case "bsses":
			out.BSSes = v
		case "clients":
			out.Clients = v
		}
		var all []models.Resource
		if e := s.db(ctx).Where("controller_id = ? AND kind = ?", id, kind).Find(&all).Error; e != nil {
			return out, e
		}
		for _, row := range filterResources(all, &dto.PageReq{AP: name, ExactAP: true}) {
			if row.ParentResourceID == nil || *row.ParentResourceID == ap.ID {
				out.Counts[kind]++
			}
		}
	}
	var state models.NeighborState
	e = s.db(ctx).Where("controller_id = ? AND ap_resource_id = ?", id, ap.ID).First(&state).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return out, nil
	}
	if e != nil {
		return out, e
	}
	c, e := s.connection(ctx, d)
	if e != nil {
		return out, e
	}
	out.NeighborCoverage = dto.WirelessCoverage{Name: "lldp", Status: state.Status, State: "unknown", Reason: "collection_" + state.Status, Version: state.Version, ObservedAt: &state.ObservedAt, LastCompleteAt: state.CompleteAt}
	if ap.Missing {
		out.NeighborCoverage.Reason = "missing_from_complete_observation"
	} else if state.SourceHash != source(d, c) || ap.SourceHash != state.SourceHash {
		out.NeighborCoverage.Reason = "source_changed"
	} else if state.Status == "complete" {
		if time.Since(state.ObservedAt) > Freshness {
			out.NeighborCoverage.State = "stale"
			out.NeighborCoverage.Reason = "evidence_stale"
		} else if state.ObservedAt.After(time.Now().UTC().Add(time.Second)) {
			out.NeighborCoverage.Reason = "observation_time_unknown"
		} else {
			out.NeighborCoverage.State = "complete"
			out.NeighborCoverage.Reason = ""
		}
	}
	for _, r := range state.Records {
		n := dto.WirelessNeighbor{EvidenceID: hash([]any{state.Version, r}), APName: name, SystemName: text(r["remote_name"]), ManagementIP: text(r["management_address"]), LocalInterface: text(r["local_interface"]), PortID: text(r["remote_interface"]), ChassisID: text(r["remote_chassis_id"]), Candidates: []dto.WirelessSwitch{}}
		var devices []network.Device
		if n.ManagementIP != "" {
			if e := s.devices(ctx, p).Where("device_type = ? AND ip = ?", "switch", n.ManagementIP).Find(&devices).Error; e != nil {
				return out, e
			}
		}
		for _, sw := range devices {
			n.Candidates = append(n.Candidates, dto.WirelessSwitch{ID: sw.Id, Name: sw.Name, IP: sw.IP})
		}
		out.Neighbors = append(out.Neighbors, n)
	}
	return out, nil
}
func (s *Wireless) Associate(ctx context.Context, id, ap int, r dto.NeighborReq, p *actions.DataPermission) (dto.WirelessAPDetail, error) {
	var result dto.WirelessAPDetail
	if r.Version < 1 || !utf8.ValidString(r.Interface) || len([]rune(r.Interface)) > 255 || strings.ContainsFunc(r.Interface, unicode.IsControl) {
		return result, ErrInvalid
	}
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		scoped := *s
		scoped.Orm = tx
		var lock models.Resource
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND controller_id = ?", ap, id).First(&lock).Error; e != nil {
			return ErrScope
		}
		detail, e := scoped.APDetail(ctx, id, ap, p)
		if e != nil {
			return e
		}
		if detail.Resource.Version != r.Version || detail.NeighborCoverage.Version != r.ObservationVersion || detail.NeighborCoverage.State != "complete" {
			return ErrConflict
		}
		found := false
		for _, n := range detail.Neighbors {
			found = found || n.EvidenceID == r.EvidenceID
		}
		if !found {
			return ErrConflict
		}
		if r.DeviceID != nil {
			if *r.DeviceID <= 0 || strings.TrimSpace(r.Interface) == "" {
				return ErrInvalid
			}
			var n int64
			if e := scoped.devices(ctx, p).Where("id = ? AND device_type = ? AND status = ?", *r.DeviceID, "switch", "active").Count(&n).Error; e != nil {
				return e
			}
			if n != 1 {
				return ErrScope
			}
		} else if r.Interface != "" {
			return ErrInvalid
		}
		q := tx.Model(&models.Resource{}).Where("id = ? AND version = ?", ap, r.Version).Updates(map[string]any{"neighbor_device_id": r.DeviceID, "neighbor_interface": strings.TrimSpace(r.Interface), "neighbor_evidence_id": r.EvidenceID, "neighbor_version": r.ObservationVersion, "version": gorm.Expr("version + 1")})
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return ErrConflict
		}
		return scoped.audit(tx, id, ap, "neighbor", r.Version+1)
	})
	if e != nil {
		return result, e
	}
	return s.APDetail(ctx, id, ap, p)
}
func (s *Wireless) Collections(ctx context.Context, id int, r *dto.PageReq, p *actions.DataPermission) ([]models.Collection, int64, error) {
	if page(r) != nil {
		return nil, 0, ErrInvalid
	}
	if _, e := s.controller(ctx, id, p); e != nil {
		return nil, 0, e
	}
	q := s.db(ctx).Model(&models.Collection{}).Where("controller_id = ?", id)
	if r.Status == "active" {
		q = q.Where("status IN ?", []string{"queued", "running"})
	} else if r.Status != "" {
		q = q.Where("status = ?", r.Status)
	}
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	rows := []models.Collection{}
	e := q.Order("id DESC").Offset((r.PageIndex - 1) * r.PageSize).Limit(r.PageSize).Find(&rows).Error
	return rows, n, e
}
func (s *Wireless) Collection(ctx context.Context, id, job int, p *actions.DataPermission) (models.Collection, error) {
	var r models.Collection
	if _, e := s.controller(ctx, id, p); e != nil {
		return r, e
	}
	e := s.db(ctx).Where("id = ? AND controller_id = ?", job, id).First(&r).Error
	return r, e
}

func (s *Wireless) Client(ctx context.Context, id, resourceID int, p *actions.DataPermission) (dto.WirelessResource, error) {
	var out dto.WirelessResource
	if _, e := s.controller(ctx, id, p); e != nil {
		return out, e
	}
	var row models.Resource
	if e := s.db(ctx).Where("id = ? AND controller_id = ? AND kind = ?", resourceID, id, "clients").First(&row).Error; e != nil {
		return out, ErrScope
	}
	rows, e := s.resourceViews(ctx, []models.Resource{row}, p)
	if e != nil {
		return out, e
	}
	return rows[0], nil
}
