package wireless

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	wireless "github.com/gatework/go-wireless"
	"github.com/gatework/motor-core/sdk/contract/actions"
	models "github.com/gatework/motor/app/net/models/wireless"
	dto "github.com/gatework/motor/app/net/service/dto/wireless"
)

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

func safeCode(s string) string {
	if codePattern.MatchString(s) {
		return s
	}
	return ""
}
func jsonText(v any) string { b, _ := json.Marshal(v); return string(b) }
func fields(r wireless.Record) dto.Fields {
	get := func(keys ...string) string {
		for _, k := range keys {
			if v := text(r[k]); v != "" {
				return v
			}
		}
		return ""
	}
	num := func(k string) *float64 {
		v, e := strconv.ParseFloat(text(r[k]), 64)
		if e != nil {
			return nil
		}
		return &v
	}
	return dto.Fields{Name: get("name"), APID: get("ap_id"), APName: get("ap_name", "name"), RadioID: get("radio_id"), BSSID: get("bssid"), SSID: get("ssid"), VLAN: get("vlan", "vlan_id"), Band: get("band"), MAC: get("client_mac", "mac_address"), IP: get("client_ip", "management_ip", "ip_address"), IPv6: get("client_ipv6"), Model: get("model"), Serial: get("serial_id", "serial_number", "serial"), Status: get("operational_status", "status"), AdminStatus: get("admin_status"), Protocol: get("protocol"), Channel: get("channel"), ClientCount: num("client_count"), Signal: num("signal_dbm"), SNR: num("snr_db")}
}
func dimensionCoverage(state models.Dimension, src string, now time.Time) dto.Coverage {
	result := dto.Coverage{Name: state.Name, Status: "not_requested", State: "unknown", Reason: "not_requested", Version: state.Version, LastCompleteAt: state.CompleteAt}
	if state.ID == 0 {
		return result
	}
	result.Status = string(state.Latest.Status())
	result.ObservedAt = &state.ObservedAt
	// Observation JSON stores microseconds; compare projection time at the same precision.
	coverage, e := wireless.NewCoverage(wireless.CoverageOptions{Observations: wireless.Record{state.Name: wireless.Record{"latest": state.Latest.ToMap()}}, Projection: wireless.Record{state.Name: wireless.Record{"status": state.ProjectionStatus, "observed_at": state.ObservedAt.Truncate(time.Microsecond).Format(time.RFC3339Nano)}}, Dimensions: []wireless.Dimension{wireless.Dimension(state.Name)}, Now: now, StaleAfter: Freshness.Seconds()})
	if e == nil {
		dimensions, _ := coverage.ToMap()["dimensions"].(map[string]any)
		m, _ := dimensions[state.Name].(map[string]any)
		result.State = text(m["state"])
		result.Reason = text(m["reason"])
	}
	if state.SourceHash != src {
		result.State = "unknown"
		result.Reason = "source_changed"
	}
	return result
}
func resourceHealth(kind string, f dto.Fields, c dto.Coverage, missing bool, parent string) string {
	if c.Status == "unsupported" {
		return "notApplicable"
	}
	if missing || c.State != "complete" || parent == "orphan" {
		return "unknown"
	}
	switch kind {
	case "access_points":
		switch wireless.OnlineStatus(f.Status) {
		case "online":
			return "healthy"
		case "offline", "not_joined":
			return "affected"
		}
	case "radios":
		switch strings.ToLower(f.AdminStatus) {
		case "down", "disabled", "disable":
			return "affected"
		case "up", "enabled", "enable":
			return "healthy"
		}
	case "clients":
		if f.APName != "" && f.MAC != "" {
			return "healthy"
		}
	case "bsses":
		if f.SSID != "" {
			return "healthy"
		}
	}
	return "unknown"
}

// ResourcePage provides the resource page operation within the caller's authorized wireless scope.
func (s *Wireless) ResourcePage(ctx context.Context, id int, kind string, r *dto.PageReq, p *actions.DataPermission) ([]dto.Resource, int64, error) {
	rows, n, e := s.Resources(ctx, id, kind, r, p)
	if e != nil {
		return nil, 0, e
	}
	v, e := s.resourceViews(ctx, rows, p)
	return v, n, e
}

// Controller provides the controller operation within the caller's authorized wireless scope.
func (s *Wireless) Controller(ctx context.Context, id int, p *actions.DataPermission) (dto.Controller, error) {
	out := dto.Controller{SupportedDimensions: []string{}, DefaultDimensions: []string{}, Coverage: []dto.Coverage{}, Counts: map[string]int{}, Candidates: map[string][]dto.Fields{}}
	d, e := s.controller(ctx, id, p)
	if e != nil {
		return out, e
	}
	out.Device = d
	c, e := s.connection(ctx, d)
	if e != nil {
		return out, e
	}
	out.Platform = platform(c)
	profile, e := wireless.NewProfile(out.Platform, d.HardwareModel)
	if e == nil {
		for _, n := range profile.Dimensions() {
			out.SupportedDimensions = append(out.SupportedDimensions, string(n))
			if n != wireless.LLDP {
				out.DefaultDimensions = append(out.DefaultDimensions, string(n))
			}
		}
	}
	var states []models.Dimension
	if e = s.db(ctx).Where("controller_id = ?", id).Find(&states).Error; e != nil {
		return out, e
	}
	index := map[string]models.Dimension{}
	for _, state := range states {
		index[state.Name] = state
	}
	for _, n := range wireless.SupportedDimensions() {
		state := index[string(n)]
		state.Name = string(n)
		cv := dimensionCoverage(state, source(d, c), time.Now().UTC())
		if profile != nil && !slices.Contains(profile.Dimensions(), n) {
			cv.Status = "unsupported"
			cv.Reason = "unsupported"
		}
		out.Coverage = append(out.Coverage, cv)
		if resourceKinds[string(n)] && state.Latest.Partial() {
			for _, r := range state.Latest.Records() {
				out.Candidates[string(n)] = append(out.Candidates[string(n)], fields(r))
			}
		}
	}
	var rows []models.Resource
	if e = s.db(ctx).Where("controller_id = ? AND missing = ?", id, false).Find(&rows).Error; e != nil {
		return out, e
	}
	for _, r := range rows {
		out.Counts[r.Kind]++
	}
	return out, nil
}
func (s *Wireless) resourceViews(ctx context.Context, rows []models.Resource, p *actions.DataPermission) ([]dto.Resource, error) {
	out := []dto.Resource{}
	v := resourceViewIndex{cover: map[int]map[string]dto.Coverage{}, names: map[int]string{}, parents: map[int]models.Resource{}, sources: map[int]string{}, visibleSwitches: map[int]bool{}, checkedSwitches: map[int]bool{}}
	for _, row := range rows {
		if e := v.loadController(ctx, s, row.ControllerID, p); e != nil {
			return nil, e
		}

		c, ok := v.cover[row.ControllerID][row.Kind]
		if !ok {
			c = dto.Coverage{Name: row.Kind, State: "unknown", Status: "not_requested", Reason: "not_requested"}
		}
		if row.SourceHash != v.sources[row.ControllerID] {
			c.State, c.Reason = "unknown", "source_changed"
		}
		if row.Kind != "access_points" {
			parent := models.Resource{}
			if row.ParentResourceID != nil {
				parent = v.parents[*row.ParentResourceID]
			}
			if parent.ID == 0 || parent.Missing || parent.SourceHash != row.SourceHash || text(parent.Facts["name"]) != text(row.Facts["ap_name"]) || v.cover[row.ControllerID]["access_points"].State != "complete" {
				row.ParentStatus = "orphan"
			}
		}
		if row.NeighborDeviceID != nil {
			id := *row.NeighborDeviceID
			if !v.checkedSwitches[id] {
				var count int64
				if e := s.devices(ctx, p).Where("id = ? AND device_type = ?", id, "switch").Count(&count).Error; e != nil {
					return nil, e
				}
				v.visibleSwitches[id], v.checkedSwitches[id] = count == 1, true
			}
			if !v.visibleSwitches[id] {
				row.NeighborDeviceID, row.NeighborInterface, row.NeighborEvidenceID, row.NeighborVersion = nil, "", "", 0
			}
		}
		f := fields(row.Facts)
		out = append(out, dto.Resource{Resource: row, Fields: f, Coverage: c, Health: resourceHealth(row.Kind, f, c, row.Missing, row.ParentStatus), ControllerName: v.names[row.ControllerID]})
	}
	return out, nil
}

type resourceViewIndex struct {
	cover                            map[int]map[string]dto.Coverage
	names                            map[int]string
	parents                          map[int]models.Resource
	sources                          map[int]string
	visibleSwitches, checkedSwitches map[int]bool
}

func (v *resourceViewIndex) loadController(ctx context.Context, s *Wireless, id int, p *actions.DataPermission) error {
	if v.cover[id] == nil {
		d, e := s.controller(ctx, id, p)
		if e != nil {
			return e
		}
		c, e := s.connection(ctx, d)
		if e != nil {
			return e
		}
		var states []models.Dimension
		if e := s.db(ctx).Where("controller_id = ?", id).Find(&states).Error; e != nil {
			return e
		}
		v.cover[id] = map[string]dto.Coverage{}
		for _, state := range states {
			v.cover[id][state.Name] = dimensionCoverage(state, source(d, c), time.Now().UTC())
		}
		v.names[id] = d.Name
		v.sources[id] = source(d, c)
		var aps []models.Resource
		if e := s.db(ctx).Where("controller_id = ? AND kind = ?", id, "access_points").Find(&aps).Error; e != nil {
			return e
		}
		for _, ap := range aps {
			v.parents[ap.ID] = ap
		}
	}
	return nil
}

func persistNeighbor(tx *gorm.DB, ap models.Resource, src string, o wireless.Observation, version int64, status string, records []wireless.Record) error {
	var state models.NeighborState
	e := tx.Where("controller_id = ? AND ap_resource_id = ?", ap.ControllerID, ap.ID).First(&state).Error
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	state.ControllerID, state.APResourceID, state.APName = ap.ControllerID, ap.ID, text(ap.Facts["name"])
	state.SourceHash, state.Version, state.Status, state.ObservedAt = src, version, status, *o.CompletedAt()
	if status == "complete" {
		state.Records = append([]wireless.Record{}, records...)
		state.CompleteAt, state.CompleteSourceHash = o.CompletedAt(), src
	}
	return tx.Save(&state).Error
}
