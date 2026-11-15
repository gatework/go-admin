package fabric

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/gatework/motor/common/worker"

	"gorm.io/gorm"

	vxlan "github.com/gatework/go-vxlan"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/delivery"
	models "github.com/gatework/motor/app/net/models/fabric"
	network "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/models/lease"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
)

//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var opsInvalidErr = InvalidInputErr

//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var opsConflictErr = ConflictErr

//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var opsScopeErr = ScopeErr

func collectionDimensions(mode string) []string {
	if mode == "configuration" {
		return []string{"configuration"}
	}
	d := vxlan.DimensionNames()
	if mode == "runtime" {
		return d[1:]
	}
	return d
}
func opsDevice(d network.Device) delivery.DeviceSnapshot {
	return delivery.DeviceSnapshot{ID: d.ID, Name: d.Name, Hostname: d.Hostname, IP: d.IP, Vendor: d.Vendor, Model: d.HardwareModel}
}

// CreateCollection creates collection within the caller's authorized scope.
func (s *Fabric) CreateCollection(ctx context.Context, id int, r *dto.CollectionCreateReq, actor int, p *actions.DataPermission) (models.Collection, error) {
	if actor <= 0 || r == nil || len(r.RequestKey) < 8 || len(r.RequestKey) > 100 || (r.Mode != "all" && r.Mode != "runtime" && r.Mode != "configuration") {
		return models.Collection{}, opsInvalidErr
	}
	devices, e := s.RequireFullScope(ctx, id, p)
	if e != nil {
		return models.Collection{}, e
	}
	job, err := s.createCollection(ctx, id, r, actor, devices)
	if err != nil {
		return models.Collection{}, err
	}
	if err := s.requireHistoricalDeviceScope(ctx, collectionDeviceIDs(job), p); err != nil {
		return models.Collection{}, err
	}
	return job, nil
}

// Collections provides the collections operation within the caller's authorized fabric scope.
func (s *Fabric) Collections(ctx context.Context, id int, r *dto.PageReq, p *actions.DataPermission) ([]models.Collection, int64, error) {
	if r == nil {
		return nil, 0, InvalidInputErr
	}
	if e := r.Validate(); e != nil {
		return nil, 0, e
	}
	if _, e := s.RequireFullScope(ctx, id, p); e != nil {
		return nil, 0, e
	}
	return historicalPage(ctx, s, id, r, p, func(row models.Collection) int { return row.ID }, collectionDeviceIDs)
}

// GetCollection loads the scoped collection and reports missing records.
func (s *Fabric) GetCollection(ctx context.Context, id int, p *actions.DataPermission) (models.Collection, error) {
	var row models.Collection
	if e := opsDB(ctx, s).First(&row, id).Error; e != nil {
		return row, e
	}
	_, e := s.RequireFullScope(ctx, row.FabricID, p)
	if e != nil {
		return models.Collection{}, e
	}
	if err := s.requireHistoricalDeviceScope(ctx, collectionDeviceIDs(row), p); err != nil {
		return models.Collection{}, err
	}
	return row, nil
}
func (s *Fabric) opsTargets(ctx context.Context, devices []network.Device) ([]models.CollectionTarget, error) {
	out := make([]models.CollectionTarget, 0, len(devices))
	for _, d := range devices {
		c, err := s.deviceConnection(ctx, d)
		snapshot := opsDevice(d)
		t := models.CollectionTarget{DeviceID: d.ID, Device: snapshot, Connection: c, SourceHash: sourceHash(snapshot, c), Status: "pending"}
		switch {
		case err != nil:
			return nil, err
		case d.Status != "active":
			t.Status = "failed"
			t.ErrorCode = "device_unavailable"
		case c.Profile == "":
			t.Status = "failed"
			t.ErrorCode = "profile_unknown"
		case c.Profile != "cisco_nxos":
			t.Status = "unsupported"
			t.ErrorCode = "platform_unsupported"
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DeviceID < out[j].DeviceID })
	return out, nil
}
func (s *Fabric) groupingHash(ctx context.Context, id int) (string, error) {
	var f models.Fabric
	if e := opsDB(ctx, s).First(&f, id).Error; e != nil {
		return "", e
	}
	var m []models.Membership
	if e := opsDB(ctx, s).Where("fabric_id = ?", id).Order("device_id").Find(&m).Error; e != nil {
		return "", e
	}
	var devices []network.Device
	ids := []int{}
	for _, x := range m {
		ids = append(ids, x.DeviceID)
	}
	if len(ids) > 0 {
		if e := opsDB(ctx, s).Where("id IN ?", ids).Order("id").Find(&devices).Error; e != nil {
			return "", e
		}
	}
	return opsHash(struct {
		Rules   int64
		Members []models.Membership
		Devices []network.Device
	}{f.RulesVersion, m, devices}), nil
}
func (s *Fabric) createCollection(ctx context.Context, id int, r *dto.CollectionCreateReq, actor int, devices []network.Device) (models.Collection, error) {
	hash := opsHash(struct {
		ID   int
		Mode string
	}{id, r.Mode})
	var existing models.Collection
	e := opsDB(ctx, s).Where("create_by = ? AND request_key = ?", actor, r.RequestKey).First(&existing).Error
	if e == nil {
		if existing.PayloadHash != hash {
			return existing, opsConflictErr
		}
		return existing, nil
	}
	if !errors.Is(e, gorm.ErrRecordNotFound) {
		return existing, e
	}
	targets, e := s.opsTargets(ctx, devices)
	if e != nil {
		return existing, e
	}
	if len(targets) == 0 {
		return existing, opsInvalidErr
	}
	group, e := s.groupingHash(ctx, id)
	if e != nil {
		return existing, e
	}
	job := models.Collection{FabricID: id, RequestKey: r.RequestKey, PayloadHash: hash, CreateBy: actor, Mode: r.Mode, Status: "queued", Version: 1, GroupingHash: group, Targets: targets}
	e = opsDB(ctx, s).Create(&job).Error
	if e != nil {
		if q := opsDB(ctx, s).Where("create_by = ? AND request_key = ?", actor, r.RequestKey).First(&existing).Error; q == nil && existing.PayloadHash == hash {
			return existing, nil
		}
	}
	return job, e
}
func (w *Worker) runCollection(ctx context.Context, job models.Collection) error {
	ctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()
	now := time.Now().UTC()
	until := now.Add(15 * time.Minute)
	q := opsDB(ctx, w.s).Model(&models.Collection{}).Where("id = ? AND status = ?", job.ID, "queued").Updates(map[string]any{"status": "running", "started_at": now, "lease_until": until, "version": gorm.Expr("version + 1")})
	if q.Error != nil || q.RowsAffected != 1 {
		return q.Error
	}
	job.Status = "running"
	job.StartedAt = &now
	group, e := w.s.groupingHash(ctx, job.FabricID)
	if e != nil || group != job.GroupingHash {
		for i := range job.Targets {
			job.Targets[i].Status = "failed"
			job.Targets[i].ErrorCode = "grouping_changed"
		}
		return w.finishCollection(ctx, &job)
	}
	for i := range job.Targets {
		t := &job.Targets[i]
		if t.Status != "pending" {
			if e := w.persistCollectionFailure(ctx, job, *t, t.Status, t.ErrorCode); e != nil {
				return e
			}
			continue
		}
		held, e := lease.Acquire(opsDB(ctx, w.s), t.DeviceID, lease.Holder{Kind: lease.FabricCollectionNode, ID: job.ID}, runTimeout)
		if code := leaseFailure(e, "device_busy"); code != "" {
			t.Status = "failed"
			t.ErrorCode = code
			if err := w.persistCollectionFailure(ctx, job, *t, t.Status, t.ErrorCode); err != nil {
				return err
			}
			continue
		} else if e != nil {
			return e
		}
		closed := w.collectTarget(ctx, job, t)
		if e := w.release(ctx, held, closed); e != nil {
			return e
		}
		if e := opsDB(ctx, w.s).Model(&models.Collection{}).Where("id = ?", job.ID).Update("targets", opsJSON(job.Targets)).Error; e != nil {
			return e
		}
	}
	return w.finishCollection(ctx, &job)
}
func (w *Worker) finishCollection(ctx context.Context, job *models.Collection) error {
	status := "complete"
	observed := 0
	for _, t := range job.Targets {
		if t.Status == "complete" || t.Status == "partial" {
			observed++
		}
		if t.Status != "complete" {
			status = "partial"
		}
	}
	if observed == 0 {
		status = "failed"
	}
	now := time.Now().UTC()
	return opsDB(ctx, w.s).Model(job).Updates(map[string]any{"targets": opsJSON(job.Targets), "status": status, "finished_at": now, "version": gorm.Expr("version + 1")}).Error
}
func (w *Worker) persistSnapshot(ctx context.Context, fabric, id int, t models.CollectionTarget, snapshot vxlan.Snapshot, started, finished time.Time) error {
	if opsHash(snapshot.Target) != opsHash(targetRecord(t.Device)) || fmt.Sprint(snapshot.FabricKey) != fmt.Sprint(fabric) || snapshot.ObservedAt == nil || snapshot.ObservedAt.Before(started.Add(-time.Second)) || snapshot.ObservedAt.After(finished.Add(time.Second)) {
		return opsInvalidErr
	}
	var member models.Membership
	if e := opsDB(ctx, w.s).First(&member, "device_id = ? AND fabric_id = ? AND status = ?", t.DeviceID, fabric, "assigned").Error; e != nil {
		return e
	}
	return opsDB(ctx, w.s).Transaction(func(tx *gorm.DB) error {
		for dimension, o := range snapshot.Dimensions {
			if e := persistFabricDimension(tx, fabric, id, t, member, snapshot, dimension, o, started, finished); e != nil {
				return e
			}
		}
		return nil
	})
}

// runTimeout bounds one collection or change run; its leases add lease.Margin.
const runTimeout = 10 * time.Minute

// leaseFailure maps a refused device lease to the target's error code:
// busyCode while another run holds it, device_quarantined when it is quarantined.
// It answers "" for success and for errors that are not refusals.
func leaseFailure(e error, busyCode string) string {
	switch {
	case errors.Is(e, lease.QuarantinedErr):
		return "device_quarantined"
	case errors.Is(e, lease.BusyErr):
		return busyCode
	}
	return ""
}
func (w *Worker) release(ctx context.Context, held lease.Lease, closed bool) error {
	// Shutdown cancellation must not strand a confirmed-closed transport lease.
	// Unknown closure still quarantines the device, within a bounded budget.
	ctx, cancel := worker.Cleanup(ctx, 5*time.Second)
	defer cancel()
	outcome := lease.Closed
	if !closed {
		outcome = lease.Unconfirmed
	}
	// A lost lease was quarantined meanwhile; that quarantine stays.
	if e := lease.Release(opsDB(ctx, w.s), held, outcome); e != nil && !errors.Is(e, lease.LostErr) {
		return e
	}
	return nil
}
func (w *Worker) checkSource(ctx context.Context, d delivery.DeviceSnapshot, c delivery.Connection, hash string) error {
	var current network.Device
	if e := opsDB(ctx, w.s).First(&current, d.ID).Error; e != nil {
		return e
	}
	connection, e := w.s.deviceConnection(ctx, current)
	if e != nil {
		return e
	}
	if current.Status != "active" || sourceHash(opsDevice(current), connection) != hash {
		return opsConflictErr
	}
	return nil
}

func (w *Worker) persistCollectionFailure(ctx context.Context, job models.Collection, t models.CollectionTarget, status, code string) error {
	now := time.Now().UTC()
	snapshot := vxlan.Snapshot{Target: targetRecord(t.Device), FabricKey: job.FabricID, ObservedAt: &now, Dimensions: map[string]vxlan.Observation{}}
	for _, dimension := range collectionDimensions(job.Mode) {
		o, _ := vxlan.NewObservation(dimension, status, nil, []vxlan.Record{{"code": code}}, nil)
		o.StartedAt = &now
		o.CompletedAt = &now
		snapshot.Dimensions[dimension] = o
	}
	err := w.persistSnapshot(ctx, job.FabricID, job.ID, t, snapshot, now, now)
	// This synthetic observation invalidates current evidence only while the
	// device still belongs to the original Fabric. Membership changes must not
	// prevent historical runs from becoming terminal, or affect the new Fabric.
	// persistSnapshot returns ErrRecordNotFound only for absent membership;
	// database and observation persistence failures must remain visible.
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

func collectionDeviceIDs(c models.Collection) []int {
	ids := make([]int, 0, len(c.Targets))
	for _, t := range c.Targets {
		ids = append(ids, t.DeviceID)
	}
	return ids
}

func (w *Worker) collectTarget(ctx context.Context, job models.Collection, t *models.CollectionTarget) bool {
	started := time.Now().UTC()
	var snapshot vxlan.Snapshot
	e := w.checkSource(ctx, t.Device, t.Connection, t.SourceHash)
	closed := true
	if e == nil {
		e = w.transport.Open(ctx, t.Device, t.Connection, func(session Session) error {
			var collectErr error
			snapshot, collectErr = vxlan.CollectSession(ctx, session, vxlan.Request{Target: targetRecord(t.Device), FabricKey: job.FabricID, Dimensions: collectionDimensions(job.Mode)})
			return collectErr
		})
		closed = transportClosed(e)
	}
	sourceErr := w.checkSource(ctx, t.Device, t.Connection, t.SourceHash)
	if sourceErr != nil {
		t.Status = "failed"
		t.ErrorCode = "source_changed"
	} else {
		snapshot = collectionFailureSnapshot(job, *t, snapshot, e, started)
		persistErr := w.persistSnapshot(ctx, job.FabricID, job.ID, *t, snapshot, started, time.Now().UTC())
		if e != nil || persistErr != nil {
			t.Status = "failed"
			t.ErrorCode = "collection_failed"
		} else if snapshot.Complete() {
			t.Status = "complete"
		} else {
			t.Status = "partial"
		}
	}
	return closed
}
func collectionFailureSnapshot(job models.Collection, t models.CollectionTarget, snapshot vxlan.Snapshot, e error, started time.Time) vxlan.Snapshot {
	if e != nil || snapshot.Dimensions == nil {
		if snapshot.Dimensions == nil {
			snapshot = vxlan.Snapshot{Target: targetRecord(t.Device), FabricKey: job.FabricID, Dimensions: map[string]vxlan.Observation{}}
		}
		end := time.Now().UTC()
		snapshot.ObservedAt = &end
		for _, dimension := range collectionDimensions(job.Mode) {
			if _, ok := snapshot.Dimensions[dimension]; !ok {
				o, _ := vxlan.NewObservation(dimension, "failed", nil, []vxlan.Record{{"code": "connection_failed"}}, nil)
				o.StartedAt = &started
				o.CompletedAt = &end
				snapshot.Dimensions[dimension] = o
			}
		}
		o, _ := vxlan.NewObservation("collection", "failed", nil, []vxlan.Record{{"code": "transport_failed"}}, nil)
		o.StartedAt = &started
		o.CompletedAt = &end
		snapshot.Dimensions["collection"] = o
	}
	return snapshot
}
func persistFabricDimension(tx *gorm.DB, fabric, id int, t models.CollectionTarget, member models.Membership, snapshot vxlan.Snapshot, dimension string, o vxlan.Observation, started, finished time.Time) error {
	// Upstream synthesizes unconfigured vPC directly from the complete
	// configuration dimension, so that observation borrows its clock.
	if dimension == "vpc" && o.Complete() && o.CompletedAt == nil && snapshot.Dimensions["configuration"].Complete() {
		for _, issue := range o.Issues {
			if issue["code"] == "vpc_not_configured" {
				o.StartedAt = snapshot.Dimensions["configuration"].StartedAt
				o.CompletedAt = snapshot.Dimensions["configuration"].CompletedAt
			}
		}
	}
	if !validObservation(o, dimension, started, finished) {
		return opsInvalidErr
	}
	var latest models.Observation
	e := tx.Where("device_id = ? AND dimension = ?", t.DeviceID, dimension).Order("observed_at DESC,id DESC").First(&latest).Error
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	if e == nil && !o.CompletedAt.After(latest.ObservedAt) {
		return nil
	}
	row := models.Observation{FabricID: fabric, DeviceID: t.DeviceID, CollectionID: id, Dimension: dimension, Status: o.Status, Data: o, ObservedAt: *o.CompletedAt, Version: latest.Version + 1, SourceHash: t.SourceHash, MembershipVersion: member.Version, RuleVersion: member.RuleVersion}
	if dimension == "configuration" && o.Complete() {
		row.ConfigHash = opsHash(o.Records)
	}
	if e := tx.Create(&row).Error; e != nil {
		return e
	}
	return nil
}
