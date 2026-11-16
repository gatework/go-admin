package wireless

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/gatework/motor-core/sdk/contract/actions"
	coredto "github.com/gatework/motor-core/sdk/contract/dto"
	network "github.com/gatework/motor/app/net/models/inventory"
	models "github.com/gatework/motor/app/net/models/wireless"
	ndto "github.com/gatework/motor/app/net/service/dto/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/wireless"
	"github.com/gatework/motor/app/net/service/inventory"
	"github.com/gatework/motor/common/database"
)

// Groups provides the groups operation within the caller's authorized wireless scope.
func (s *Wireless) Groups(ctx context.Context, p *actions.DataPermission) (ndto.DeviceGroupTree, error) {
	g := inventory.DeviceGroup{Service: s.Service}
	return g.TreeForDeviceType(ctx, p, "wireless_controller")
}

// Switches provides the switches operation within the caller's authorized wireless scope.
func (s *Wireless) Switches(ctx context.Context, r *dto.PageReq, p *actions.DataPermission) ([]dto.Switch, int64, error) {
	if e := page(r); e != nil {
		return nil, 0, e
	}
	q := s.devices(ctx, p).Where("device_type = ? AND status = ?", "switch", "active")
	if r.Keyword != "" {
		like := " LIKE ? " + database.LikeEscape
		q = q.Where("LOWER(name)"+like+" OR ip"+like, database.LikeContains(strings.ToLower(r.Keyword)), database.LikeContains(r.Keyword))
	}
	var n int64
	if e := q.Count(&n).Error; e != nil {
		return nil, 0, e
	}
	var rows []network.Device
	if e := q.Order("id").Scopes(coredto.Paginate(r.PageSize, r.PageIndex)).Find(&rows).Error; e != nil {
		return nil, 0, e
	}
	out := []dto.Switch{}
	for _, d := range rows {
		out = append(out, dto.Switch{ID: d.ID, Name: d.Name, IP: d.IP})
	}
	return out, n, nil
}

// Clients provides the clients operation within the caller's authorized wireless scope.
func (s *Wireless) Clients(ctx context.Context, r *dto.PageReq, p *actions.DataPermission) ([]dto.Resource, int64, error) {
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

// APDetail provides the a p detail operation within the caller's authorized wireless scope.
func (s *Wireless) APDetail(ctx context.Context, id, apID int, p *actions.DataPermission) (dto.APDetail, error) {
	out := dto.APDetail{Neighbors: []dto.Neighbor{}, Radios: []dto.Resource{}, BSSes: []dto.Resource{}, Clients: []dto.Resource{}, Counts: map[string]int64{}, NeighborCoverage: dto.Coverage{Name: "lldp", Status: "not_requested", State: "unknown", Reason: "not_requested"}}
	d, e := s.controller(ctx, id, p)
	if e != nil {
		return out, e
	}
	var ap models.Resource
	if e = s.db(ctx).Where("id = ? AND controller_id = ? AND kind = ?", apID, id, "access_points").First(&ap).Error; e != nil {
		return out, ScopeErr
	}
	views, e := s.resourceViews(ctx, []models.Resource{ap}, p)
	if e != nil {
		return out, e
	}
	out.Resource = views[0]
	name := text(ap.Facts["name"])
	if e := s.apChildren(ctx, id, ap, p, &out); e != nil {
		return out, e
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
	out.NeighborCoverage = neighborCoverage(ap, state, source(d, c))
	for _, r := range state.Records {
		n := dto.Neighbor{EvidenceID: hash([]any{state.Version, r}), APName: name, SystemName: text(r["remote_name"]), ManagementIP: text(r["management_address"]), LocalInterface: text(r["local_interface"]), PortID: text(r["remote_interface"]), ChassisID: text(r["remote_chassis_id"]), Candidates: []dto.Switch{}}
		var devices []network.Device
		if n.ManagementIP != "" {
			if e := s.devices(ctx, p).Where("device_type = ? AND ip = ?", "switch", n.ManagementIP).Find(&devices).Error; e != nil {
				return out, e
			}
		}
		for _, sw := range devices {
			n.Candidates = append(n.Candidates, dto.Switch{ID: sw.ID, Name: sw.Name, IP: sw.IP})
		}
		out.Neighbors = append(out.Neighbors, n)
	}
	return out, nil
}

// Associate provides the associate operation within the caller's authorized wireless scope.
func (s *Wireless) Associate(ctx context.Context, id, ap int, r dto.NeighborReq, p *actions.DataPermission) (dto.APDetail, error) {
	var result dto.APDetail
	if r.Version < 1 || !utf8.ValidString(r.Interface) || len([]rune(r.Interface)) > 255 || strings.ContainsFunc(r.Interface, unicode.IsControl) {
		return result, InvalidErr
	}
	e := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		scoped := *s
		scoped.DB = tx
		var lock models.Resource
		if e := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND controller_id = ?", ap, id).First(&lock).Error; e != nil {
			return ScopeErr
		}
		detail, e := scoped.APDetail(ctx, id, ap, p)
		if e != nil {
			return e
		}
		if detail.Resource.Version != r.Version || detail.NeighborCoverage.Version != r.ObservationVersion || detail.NeighborCoverage.State != "complete" {
			return ConflictErr
		}
		found := false
		for _, n := range detail.Neighbors {
			found = found || n.EvidenceID == r.EvidenceID
		}
		if !found {
			return ConflictErr
		}
		if e := scoped.validateNeighbor(ctx, r, p); e != nil {
			return e
		}
		q := tx.Model(&models.Resource{}).Where("id = ? AND version = ?", ap, r.Version).Updates(map[string]any{"neighbor_device_id": r.DeviceID, "neighbor_interface": strings.TrimSpace(r.Interface), "neighbor_evidence_id": r.EvidenceID, "neighbor_version": r.ObservationVersion, "version": gorm.Expr("version + 1")})
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return ConflictErr
		}
		return scoped.audit(tx, id, ap, "neighbor", r.Version+1)
	})
	if e != nil {
		return result, e
	}
	return s.APDetail(ctx, id, ap, p)
}

// Collections provides the collections operation within the caller's authorized wireless scope.
func (s *Wireless) Collections(ctx context.Context, id int, r *dto.PageReq, p *actions.DataPermission) ([]models.Collection, int64, error) {
	if page(r) != nil {
		return nil, 0, InvalidErr
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
	e := q.Order("id DESC").Scopes(coredto.Paginate(r.PageSize, r.PageIndex)).Find(&rows).Error
	return rows, n, e
}

// Collection provides the collection operation within the caller's authorized wireless scope.
func (s *Wireless) Collection(ctx context.Context, id, job int, p *actions.DataPermission) (models.Collection, error) {
	var r models.Collection
	if _, e := s.controller(ctx, id, p); e != nil {
		return r, e
	}
	e := s.db(ctx).Where("id = ? AND controller_id = ?", job, id).First(&r).Error
	return r, e
}

// Client provides the client operation within the caller's authorized wireless scope.
func (s *Wireless) Client(ctx context.Context, id, resourceID int, p *actions.DataPermission) (dto.Resource, error) {
	var out dto.Resource
	if _, e := s.controller(ctx, id, p); e != nil {
		return out, e
	}
	var row models.Resource
	if e := s.db(ctx).Where("id = ? AND controller_id = ? AND kind = ?", resourceID, id, "clients").First(&row).Error; e != nil {
		return out, ScopeErr
	}
	rows, e := s.resourceViews(ctx, []models.Resource{row}, p)
	if e != nil {
		return out, e
	}
	return rows[0], nil
}

func (s *Wireless) apChildren(ctx context.Context, id int, ap models.Resource, p *actions.DataPermission, out *dto.APDetail) error {
	name := text(ap.Facts["name"])
	for _, kind := range []string{"radios", "bsses", "clients"} {
		rows, _, e := s.Resources(ctx, id, kind, &dto.PageReq{PageSize: 200, AP: name, ExactAP: true}, p)
		if e != nil {
			return e
		}
		scopedRows := rows[:0]
		for _, row := range rows {
			if row.ParentResourceID == nil || *row.ParentResourceID == ap.ID {
				scopedRows = append(scopedRows, row)
			}
		}
		v, e := s.resourceViews(ctx, scopedRows, p)
		if e != nil {
			return e
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
			return e
		}
		for _, row := range filterResources(all, &dto.PageReq{AP: name, ExactAP: true}) {
			if row.ParentResourceID == nil || *row.ParentResourceID == ap.ID {
				out.Counts[kind]++
			}
		}
	}
	return nil
}
func neighborCoverage(ap models.Resource, state models.NeighborState, currentSource string) dto.Coverage {
	coverage := dto.Coverage{Name: "lldp", Status: state.Status, State: "unknown", Reason: "collection_" + state.Status, Version: state.Version, ObservedAt: &state.ObservedAt, LastCompleteAt: state.CompleteAt}
	if ap.Missing {
		coverage.Reason = "missing_from_complete_observation"
	} else if state.SourceHash != currentSource || ap.SourceHash != state.SourceHash {
		coverage.Reason = "source_changed"
	} else if state.Status == "complete" {
		if time.Since(state.ObservedAt) > Freshness {
			coverage.State = "stale"
			coverage.Reason = "evidence_stale"
		} else if state.ObservedAt.After(time.Now().UTC().Add(time.Second)) {
			coverage.Reason = "observation_time_unknown"
		} else {
			coverage.State = "complete"
			coverage.Reason = ""
		}
	}
	return coverage
}
func (s *Wireless) validateNeighbor(ctx context.Context, r dto.NeighborReq, p *actions.DataPermission) error {
	if r.DeviceID != nil {
		if *r.DeviceID <= 0 || strings.TrimSpace(r.Interface) == "" {
			return InvalidErr
		}
		var n int64
		if e := s.devices(ctx, p).Where("id = ? AND device_type = ? AND status = ?", *r.DeviceID, "switch", "active").Count(&n).Error; e != nil {
			return e
		}
		if n != 1 {
			return ScopeErr
		}
	} else if r.Interface != "" {
		return InvalidErr
	}
	return nil
}
