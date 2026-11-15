package fabric

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	vxlan "github.com/gatework/go-vxlan"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/delivery"
	models "github.com/gatework/motor/app/net/models/fabric"
	network "github.com/gatework/motor/app/net/models/inventory"
	ds "github.com/gatework/motor/app/net/service/delivery"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
)

func opMap(v any) vxlan.Record { m, _ := v.(map[string]any); return m }
func opStrings(v any) []string {
	out := []string{}
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		for _, s := range x {
			if text, ok := s.(string); ok {
				out = append(out, text)
			}
		}
	}
	return out
}

// PreviewChange evaluates change without applying device changes.
func (s *Fabric) PreviewChange(ctx context.Context, id int, r *dto.ChangeCreateReq, actor int, p *actions.DataPermission) (models.Change, error) {
	var out models.Change
	if r == nil || actor <= 0 || len(r.RequestKey) < 8 || len(r.RequestKey) > 100 || len(r.TargetDeviceIDs) == 0 || len(r.TargetDeviceIDs) > 100 {
		return out, InvalidErr
	}
	devices, e := s.RequireFullScope(ctx, id, p)
	if e != nil {
		return out, e
	}
	byID, ids, e := previewDeviceScope(devices, r)
	if e != nil {
		return out, e
	}
	payload := opsHash(struct {
		Fabric    int
		Source    int
		Targets   []int
		Kind      string
		VNI       int
		Direction string
	}{id, r.SourceDeviceID, ids, r.Kind, r.VNI, r.Direction})
	e = opsDB(ctx, s).Where("create_by = ? AND request_key = ?", actor, r.RequestKey).First(&out).Error
	if e == nil {
		if out.PayloadHash != payload {
			return models.Change{}, ConflictErr
		}
		return out, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return out, e
	}
	so, config, e := s.changeConfig(ctx, id, r.SourceDeviceID)
	if e != nil {
		return out, e
	}
	plan, e := controlledPlan(config, r)
	if e != nil {
		return out, e
	}
	group, e := s.groupingHash(ctx, id)
	if e != nil {
		return out, e
	}
	selected := []network.Device{}
	for _, device := range ids {
		selected = append(selected, byID[device])
	}
	targets, e := s.opsTargets(ctx, selected)
	if e != nil {
		return out, e
	}
	ct, dt, e := s.previewTargets(ctx, id, targets, config, plan)
	if e != nil {
		return out, e
	}
	out = models.Change{FabricID: id, RequestKey: r.RequestKey, PayloadHash: payload, CreateBy: actor, SourceDeviceID: r.SourceDeviceID, Kind: r.Kind, VNI: r.VNI, Direction: r.Direction, Status: "preview", Version: 1, GroupingHash: group, PlanHash: opsHash(plan.Change()), Plan: plan.Change(), SourceObservationID: so.ID, SourceHash: so.SourceHash, SourceConfigHash: opsHash(config), Targets: ct}
	for _, d := range devices {
		out.Authorization = append(out.Authorization, d.ID)
	}
	sort.Ints(out.Authorization)
	e = opsDB(ctx, s).Transaction(func(tx *gorm.DB) error {
		order := delivery.Order{Kind: "fabric", FabricID: id, RequestNo: "FB-" + uuid.NewString(), RequestKey: "fabric:" + opsHash(r.RequestKey), PayloadHash: payload, Title: fmt.Sprintf("Fabric %d %s VNI %d", id, r.Kind, r.VNI), Status: "preview", Version: 1, DeviceCount: len(dt), CreateBy: actor, Targets: dt}
		if e := tx.Create(&order).Error; e != nil {
			return e
		}
		out.DeliveryOrderID = order.ID
		for i := range out.Targets {
			out.Targets[i].DeliveryTargetID = order.Targets[i].ID
		}
		if e := tx.Create(&out).Error; e != nil {
			return e
		}
		return tx.Create(&delivery.Audit{OrderID: order.ID, ActorID: actor, Action: "fabric_preview"}).Error
	})
	if e != nil {
		var existing models.Change
		if find := opsDB(ctx, s).Where("create_by = ? AND request_key = ?", actor, r.RequestKey).First(&existing).Error; find == nil && existing.PayloadHash == payload {
			return existing, nil
		}
	}
	return out, e
}
func controlledPlan(source vxlan.Record, r *dto.ChangeCreateReq) (*vxlan.ChangePlan, error) {
	vlan := ""
	for key, v := range opMap(source["vlan_vnis"]) {
		if fmt.Sprint(v) == strconv.Itoa(r.VNI) {
			if vlan != "" {
				return nil, InvalidErr
			}
			vlan = key
		}
	}
	vid, e := strconv.Atoi(vlan)
	if e != nil {
		return nil, InvalidErr
	}
	svi := opMap(opMap(source["interfaces"])["Vlan"+vlan])
	if svi == nil {
		return nil, InvalidErr
	}
	change := vxlan.Record{"kind": r.Kind, "vlan": vid, "vni": r.VNI}
	switch r.Kind {
	case "vlan_vni_add", "svi_gateway_add":
		addresses := opStrings(svi["addresses"])
		if len(addresses) != 1 || svi["anycast"] != true {
			return nil, InvalidErr
		}
		change["address"] = addresses[0]
		change["vrf"] = svi["vrf"]
	case "svi_acl":
		if r.Direction != "in" && r.Direction != "out" {
			return nil, InvalidErr
		}
		name, ok := opMap(svi["acl_bindings"])[r.Direction].(string)
		if !ok || name == "" {
			return nil, InvalidErr
		}
		entries := opMap(source["acls"])[name]
		policy := vxlan.ResolveACLPolicy(source, entries)
		if policy == nil {
			return nil, InvalidErr
		}
		change["acl"] = name
		change["direction"] = r.Direction
		change["entries"] = entries
		change["address_groups"] = policy["address_groups"]
		change["create_acl"] = false
		change["previous_binding"] = nil
	default:
		return nil, InvalidErr
	}
	return vxlan.NewChangePlan(change)
}

// GetChange loads the scoped change and reports missing records.
func (s *Fabric) GetChange(ctx context.Context, id int, p *actions.DataPermission) (models.Change, error) {
	var row models.Change
	if e := opsDB(ctx, s).First(&row, id).Error; e != nil {
		return row, e
	}
	if _, e := s.RequireFullScope(ctx, row.FabricID, p); e != nil {
		return models.Change{}, e
	}
	if err := s.requireHistoricalDeviceScope(ctx, changeDeviceIDs(row), p); err != nil {
		return models.Change{}, err
	}
	return row, nil
}

// Changes provides the changes operation within the caller's authorized fabric scope.
func (s *Fabric) Changes(ctx context.Context, id int, r *dto.PageReq, p *actions.DataPermission) ([]models.Change, int64, error) {
	if r == nil {
		return nil, 0, InvalidErr
	}
	if e := r.Validate(); e != nil {
		return nil, 0, e
	}
	if _, e := s.RequireFullScope(ctx, id, p); e != nil {
		return nil, 0, e
	}
	return historicalPage(ctx, s, id, r, p, func(row models.Change) int { return row.ID }, changeDeviceIDs)
}

// ExecuteChange provides the execute change operation within the caller's authorized fabric scope.
func (s *Fabric) ExecuteChange(ctx context.Context, id int, r *dto.VersionReq, actor int, p *actions.DataPermission) (models.Change, error) {
	if actor <= 0 {
		return models.Change{}, InvalidErr
	}
	c, e := s.GetChange(ctx, id, p)
	if e != nil {
		return c, e
	}
	if r == nil || r.Version != c.Version || c.Status != "preview" {
		return c, ConflictErr
	}
	if !ds.FromEnvironment().Enabled {
		return c, ds.DisabledErr
	}
	if e = s.validateChange(ctx, c); e != nil {
		return c, e
	}
	e = opsDB(ctx, s).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&models.Change{}).Where("id = ? AND version = ? AND status = ?", id, r.Version, "preview").Updates(map[string]any{"status": "queued", "execute_by": actor, "version": gorm.Expr("version + 1")})
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return ConflictErr
		}
		if e := tx.Model(&delivery.Order{}).Where("id = ? AND kind = ?", c.DeliveryOrderID, "fabric").Updates(map[string]any{"status": "queued", "version": gorm.Expr("version + 1")}).Error; e != nil {
			return e
		}
		return tx.Create(&delivery.Audit{OrderID: c.DeliveryOrderID, ActorID: actor, Action: "fabric_execute"}).Error
	})
	if e != nil {
		return c, e
	}
	return s.GetChange(ctx, id, p)
}

// CancelChange provides the cancel change operation within the caller's authorized fabric scope.
func (s *Fabric) CancelChange(ctx context.Context, id int, r *dto.VersionReq, actor int, p *actions.DataPermission) (models.Change, error) {
	if actor <= 0 {
		return models.Change{}, InvalidErr
	}
	c, e := s.GetChange(ctx, id, p)
	if e != nil {
		return c, e
	}
	if r == nil || r.Version != c.Version || (c.Status != "preview" && c.Status != "queued") {
		return c, ConflictErr
	}
	for i := range c.Targets {
		c.Targets[i].Status = "cancelled"
	}
	now := time.Now().UTC()
	e = opsDB(ctx, s).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&models.Change{}).Where("id = ? AND version = ? AND status IN ?", id, r.Version, []string{"preview", "queued"}).Updates(map[string]any{"status": "cancelled", "targets": opsJSON(c.Targets), "finished_at": now, "version": gorm.Expr("version + 1")})
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return ConflictErr
		}
		if e := tx.Model(&delivery.Order{}).Where("id = ? AND kind = ?", c.DeliveryOrderID, "fabric").Updates(map[string]any{"status": "cancelled", "cancel_requested": true, "finished_at": now}).Error; e != nil {
			return e
		}
		if e := tx.Model(&delivery.Target{}).Where("order_id = ? AND status = ?", c.DeliveryOrderID, "pending").Update("status", "cancelled").Error; e != nil {
			return e
		}
		return tx.Create(&delivery.Audit{OrderID: c.DeliveryOrderID, ActorID: actor, Action: "fabric_cancel"}).Error
	})
	if e != nil {
		return c, e
	}
	return s.GetChange(ctx, id, p)
}
func (s *Fabric) changeConfig(ctx context.Context, fabric, device int) (models.Observation, vxlan.Record, error) {
	var o models.Observation
	e := opsDB(ctx, s).Where("fabric_id = ? AND device_id = ? AND dimension = ?", fabric, device, "configuration").Order("observed_at DESC,id DESC").First(&o).Error
	if e != nil {
		return o, nil, e
	}
	var f models.Fabric
	if e = opsDB(ctx, s).First(&f, fabric).Error; e != nil {
		return o, nil, e
	}
	minutes := f.ConfigMinutes
	if minutes <= 0 {
		minutes = 30
	}
	if o.Status != "complete" || len(o.Data.Records) != 1 || o.Data.Records[0]["review_status"] != "complete" || o.ObservedAt.After(time.Now().UTC().Add(time.Second)) || time.Since(o.ObservedAt) > time.Duration(2*minutes)*time.Minute {
		return o, nil, UnavailableErr
	}
	var d network.Device
	var m models.Membership
	if e = opsDB(ctx, s).First(&d, device).Error; e != nil {
		return o, nil, e
	}
	c, e := s.deviceConnection(ctx, d)
	if e != nil {
		return o, nil, e
	}
	if e = opsDB(ctx, s).First(&m, "device_id = ? AND fabric_id = ?", device, fabric).Error; e != nil {
		return o, nil, e
	}
	if o.SourceHash != sourceHash(opsDevice(d), c) || o.MembershipVersion != m.Version || o.RuleVersion != m.RuleVersion || d.Status != "active" {
		return o, nil, ConflictErr
	}
	return o, o.Data.Records[0], nil
}
func (s *Fabric) validateChange(ctx context.Context, c models.Change) error {
	group, e := s.groupingHash(ctx, c.FabricID)
	if e != nil {
		return e
	}
	if group != c.GroupingHash || opsHash(c.Plan) != c.PlanHash {
		return ConflictErr
	}
	so, cfg, e := s.changeConfig(ctx, c.FabricID, c.SourceDeviceID)
	if e != nil {
		return e
	}
	if so.SourceHash != c.SourceHash || opsHash(cfg) != c.SourceConfigHash {
		return ConflictErr
	}
	expected, e := controlledPlan(cfg, &dto.ChangeCreateReq{Kind: c.Kind, VNI: c.VNI, Direction: c.Direction})
	if e != nil || opsHash(expected.Change()) != c.PlanHash {
		return ConflictErr
	}
	sourceConfig := cfg
	for _, t := range c.Targets {
		if t.Status != "pending" && t.Status != "running" {
			continue
		}
		o, cfg, e := s.changeConfig(ctx, c.FabricID, t.DeviceID)
		if e != nil {
			return e
		}
		if o.SourceHash != t.SourceHash || opsHash(cfg) != t.ConfigHash || !aclServiceContextMatches(sourceConfig, cfg, expected.Change()) {
			return ConflictErr
		}
	}
	return nil
}

// ACL names and matching rules do not establish tenant identity. Bind the source
// and target SVI to the same observed VRF and tenant routing declarations.
func aclServiceContextMatches(source, target, change vxlan.Record) bool {
	if change["kind"] != "svi_acl" {
		return true
	}
	vlan := text(change["vlan"])
	if number(record(source["vlan_vnis"])[vlan]) != number(change["vni"]) || number(record(target["vlan_vnis"])[vlan]) != number(change["vni"]) {
		return false
	}
	a := record(record(source["interfaces"])["Vlan"+vlan])
	b := record(record(target["interfaces"])["Vlan"+vlan])
	vrf := text(a["vrf"])
	if vrf == "" || vrf != text(b["vrf"]) {
		return false
	}
	at, bt := vxlan.ServiceTenant(source, vrf), vxlan.ServiceTenant(target, vrf)
	return at != nil && bt != nil && opsHash(at) == opsHash(bt)
}

func changeDeviceIDs(c models.Change) []int {
	ids := []int{c.SourceDeviceID}
	for _, t := range c.Targets {
		ids = append(ids, t.DeviceID)
	}
	return ids
}
func (s *Fabric) requireHistoricalDeviceScope(ctx context.Context, ids []int, p *actions.DataPermission) error {
	unique := map[int]bool{}
	for _, id := range ids {
		if id <= 0 {
			return ScopeErr
		}
		unique[id] = true
	}
	ids = ids[:0]
	for id := range unique {
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return ScopeErr
	}
	var count int64
	if err := deviceScope(s.DB.WithContext(ctx), p).Where("id IN ?", ids).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return ScopeErr
	}
	return nil
}

func (s *Fabric) previewTargets(ctx context.Context, id int, targets []models.CollectionTarget, config vxlan.Record, plan *vxlan.ChangePlan) ([]models.ChangeTarget, []delivery.Target, error) {
	ct := make([]models.ChangeTarget, 0, len(targets))
	dt := make([]delivery.Target, 0, len(targets))
	for _, t := range targets {
		if t.Status != "pending" {
			return nil, nil, UnavailableErr
		}
		o, cfg, e := s.changeConfig(ctx, id, t.DeviceID)
		if e != nil {
			return nil, nil, e
		}
		if !plan.Eligible(cfg) || !aclServiceContextMatches(config, cfg, plan.Change()) {
			return nil, nil, InvalidErr
		}
		commands := plan.Commands()
		script := strings.Join(commands, "\n")
		ct = append(ct, models.ChangeTarget{DeviceID: t.DeviceID, Status: "pending", Commands: commands, SourceHash: t.SourceHash, ConfigHash: opsHash(cfg), ObservationID: o.ID, Device: t.Device, Connection: t.Connection})
		dt = append(dt, delivery.Target{DeviceID: t.DeviceID, Device: t.Device, Connection: t.Connection, ConnectionVersion: t.Connection.Version, Script: script, ScriptSHA256: opsHash(script), ScriptLineCount: len(commands), Status: "pending"})
	}
	return ct, dt, nil
}

func previewDeviceScope(devices []network.Device, r *dto.ChangeCreateReq) (map[int]network.Device, []int, error) {
	byID := map[int]network.Device{}
	for _, d := range devices {
		byID[d.ID] = d
	}
	source, ok := byID[r.SourceDeviceID]
	if !ok || !strings.EqualFold(source.DeviceRole, "leaf_switch") {
		return nil, nil, ScopeErr
	}
	ids := append([]int{}, r.TargetDeviceIDs...)
	sort.Ints(ids)
	for i, device := range ids {
		d, ok := byID[device]
		if !ok || device == r.SourceDeviceID || !strings.EqualFold(d.DeviceRole, "leaf_switch") || (i > 0 && ids[i-1] == device) {
			return nil, nil, InvalidErr
		}
	}
	return byID, ids, nil
}
