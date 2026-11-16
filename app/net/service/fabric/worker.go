package fabric

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gatework/motor/common/account"

	"gorm.io/gorm"

	vxlan "github.com/gatework/go-vxlan"
	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor/app/net/models/delivery"
	models "github.com/gatework/motor/app/net/models/fabric"
	network "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/models/lease"
	ds "github.com/gatework/motor/app/net/service/delivery"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
	"github.com/gatework/motor/common/worker"
)

// Worker advances queued operations while holding the required device leases.
type Worker struct {
	loop      worker.Loop
	s         *Fabric
	transport Transport

	lastSchedule time.Time
}

// NewWorker constructs an application-owned background worker.
func NewWorker(s *Fabric, t Transport) *Worker {
	if t == nil {
		t = nativeTransport{access: deviceaccess.FromEnvironment()}
	}
	// A background worker has no caller to return an error to; its log line is the
	// only trace of a failed tick, so it must never depend on the host's wiring.
	if s.Logger == nil {
		s.Logger = logger.NewHelper(logger.Default())
	}
	return &Worker{s: s, transport: t, loop: worker.Loop{Name: "fabric worker", Logger: s.Logger}}
}

// Run owns the worker lifecycle until cancellation and waits for its pollers.
func (w *Worker) Run(ctx context.Context) error {
	return w.loop.Run(ctx, 1, w.Tick)
}

// Tick advances due work while preserving lease ownership and durable outcomes.
func (w *Worker) Tick(ctx context.Context) error {
	if e := w.Recover(ctx); e != nil {
		return e
	}
	if time.Since(w.lastSchedule) >= time.Minute {
		if e := w.schedule(ctx, time.Now().UTC()); e != nil {
			return e
		}
		w.lastSchedule = time.Now().UTC()
	}
	var jobs []models.Collection
	if e := opsDB(ctx, w.s).Where("status = ?", "queued").Order("id").Limit(20).Find(&jobs).Error; e != nil {
		return e
	}
	for _, job := range jobs {
		lock, e := lease.LockFabric(opsDB(ctx, w.s), job.FabricID, lease.Holder{Kind: lease.FabricCollection, ID: job.ID}, runTimeout)
		if errors.Is(e, lease.BusyErr) {
			continue
		} else if e != nil {
			return e
		}
		e = w.runCollection(ctx, job)
		release := w.unlock(ctx, lock)
		if e != nil {
			return e
		}
		if release != nil {
			return release
		}
		break
	}
	if !ds.FromEnvironment().Enabled {
		return nil
	}
	var changes []models.Change
	if e := opsDB(ctx, w.s).Where("status = ?", "queued").Order("id").Limit(20).Find(&changes).Error; e != nil {
		return e
	}
	for _, c := range changes {
		lock, e := lease.LockFabric(opsDB(ctx, w.s), c.FabricID, lease.Holder{Kind: lease.FabricChange, ID: c.ID}, runTimeout)
		if errors.Is(e, lease.BusyErr) {
			continue
		} else if e != nil {
			return e
		}
		e = w.runChange(ctx, c)
		release := w.unlock(ctx, lock)
		if e != nil {
			return e
		}
		if release != nil {
			return release
		}
		break
	}
	return nil
}

// Recover records interrupted runs once their execution budget has elapsed.
// It never steals expired device leases or requeues a possibly attempted write.
func (w *Worker) Recover(ctx context.Context) error {
	// The sweep quarantines expired device leases and drops expired fabric locks,
	// this module's and every other module's.
	if e := lease.Sweep(opsDB(ctx, w.s)); e != nil {
		return e
	}
	now := time.Now().UTC()
	var collects []models.Collection
	if e := opsDB(ctx, w.s).Where("status = ? AND claim_until < ?", "running", now).Find(&collects).Error; e != nil {
		return e
	}
	for _, c := range collects {
		if e := w.recoverCollection(ctx, c, now); e != nil {
			return e
		}
	}
	var changes []models.Change
	if e := opsDB(ctx, w.s).Where("status = ? AND claim_until < ?", "running", now).Find(&changes).Error; e != nil {
		return e
	}
	for _, c := range changes {
		if e := w.recoverChange(ctx, c); e != nil {
			return e
		}
	}
	return nil
}

func (w *Worker) schedule(ctx context.Context, now time.Time) error {
	var identities []network.Device
	if e := opsDB(ctx, w.s).Select("id", "name", "device_role").Find(&identities).Error; e != nil {
		return e
	}
	if e := w.s.reconcile(ctx, identities); e != nil {
		return e
	}
	var fabrics []models.Fabric
	if e := opsDB(ctx, w.s).Where("collection_enabled = ?", true).Find(&fabrics).Error; e != nil {
		return e
	}
	for _, f := range fabrics {
		if e := w.scheduleFabric(ctx, f, now); e != nil {
			return e
		}
	}
	return nil
}

// unlock releases a fabric lock. It guards no session, and a lost lock was
// already dropped by a sweep.
func (w *Worker) unlock(ctx context.Context, lock lease.Lease) error {
	ctx, cancel := worker.Cleanup(ctx, 5*time.Second)
	defer cancel()
	if e := lease.Release(opsDB(ctx, w.s), lock, lease.NoSession); e != nil && !errors.Is(e, lease.LostErr) {
		return e
	}
	return nil
}

// executePermission is the menu permission a change's approver must still
// hold when the change runs.
const executePermission = "network:fabric:execute"

func (w *Worker) currentAuthorization(ctx context.Context, c models.Change) error {
	actor := c.ExecuteBy
	if actor <= 0 {
		actor = c.CreateBy
	}
	// The change runs long after it was approved: re-read the approver's
	// grant, so a user disabled or a role that lost the permission stops it.
	grant, ok, e := account.CurrentGrant(ctx, opsDB(ctx, w.s), actor)
	if e != nil {
		return e
	}
	if !ok {
		return ScopeErr
	}
	if held, e := grant.Holds(ctx, opsDB(ctx, w.s), executePermission); e != nil {
		return e
	} else if !held {
		return ScopeErr
	}
	devices, e := w.s.RequireFullScope(ctx, c.FabricID, grant.Permission())
	if e != nil {
		return e
	}
	ids := []int{}
	for _, d := range devices {
		ids = append(ids, d.ID)
	}
	if opsHash(ids) != opsHash(c.Authorization) {
		return ScopeErr
	}
	return nil
}
func (w *Worker) runChange(parent context.Context, c models.Change) error {
	ctx, cancel := context.WithTimeout(parent, runTimeout)
	defer cancel()
	now := time.Now().UTC()
	c.StartedAt = &now
	until := now.Add(15 * time.Minute)
	q := opsDB(ctx, w.s).Model(&models.Change{}).Where("id = ? AND status = ?", c.ID, "queued").Updates(map[string]any{"status": "running", "started_at": now, "claim_until": until, "version": gorm.Expr("version + 1")})
	if q.Error != nil || q.RowsAffected != 1 {
		return q.Error
	}
	if e := w.currentAuthorization(ctx, c); e != nil {
		return w.stopChange(ctx, &c, "authorization_changed")
	}
	if e := w.s.validateChange(ctx, c); e != nil {
		return w.stopChange(ctx, &c, "evidence_or_grouping_changed")
	}
	plan, e := vxlan.NewChangePlan(c.Plan)
	if e != nil {
		return w.stopChange(ctx, &c, "plan_invalid")
	}
	var source models.CollectionTarget
	var sd network.Device
	if e := opsDB(ctx, w.s).First(&sd, c.SourceDeviceID).Error; e != nil {
		return e
	}
	sc, e := w.s.deviceConnection(ctx, sd, deviceaccess.Read)
	if e != nil {
		return e
	}
	source = models.CollectionTarget{DeviceID: sd.ID, Device: opsDevice(sd), Connection: sc, SourceHash: c.SourceHash}
	sourceLease, e := lease.Acquire(opsDB(ctx, w.s), sd.ID, lease.Holder{Kind: lease.FabricChangeSource, ID: c.ID}, runTimeout)
	if code := leaseFailure(e, "source_busy"); code != "" {
		return w.stopChange(ctx, &c, code)
	} else if e != nil {
		return e
	}
	sourceClosed := false
	defer func() { _ = w.release(ctx, sourceLease, sourceClosed) }()
	started := time.Now().UTC()
	var sourceSnapshot vxlan.Snapshot
	e = w.transport.Open(ctx, source.Device, source.Connection, func(session Session) error {
		var err error
		sourceSnapshot, err = vxlan.CollectSession(ctx, session, vxlan.Request{Target: targetRecord(source.Device), FabricKey: c.FabricID, Dimensions: []string{"configuration"}})
		return err
	})
	sourceClosed = transportClosed(e)
	if e != nil || !sourceSnapshot.Complete() || len(sourceSnapshot.Dimensions["configuration"].Records) != 1 || opsHash(sourceSnapshot.Dimensions["configuration"].Records[0]) != c.SourceConfigHash {
		return w.stopChange(ctx, &c, "source_configuration_drift")
	}
	if e := w.persistSnapshot(ctx, c.FabricID, 0, source, sourceSnapshot, started, time.Now().UTC()); e != nil {
		return w.stopChange(ctx, &c, "source_evidence_invalid")
	}
	return w.runChangeTargets(ctx, &c, plan)
}

func (w *Worker) runChangeTarget(ctx context.Context, c *models.Change, t *models.ChangeTarget, plan *vxlan.ChangePlan, held lease.Lease) (bool, error) {
	started := time.Now().UTC()
	e := w.checkSource(ctx, t.Device, t.Connection, t.SourceHash)
	if e != nil {
		return true, e
	}

	e = w.transport.Open(ctx, t.Device, t.Connection, func(session Session) error {
		var err error
		t.PreEvidence, err = vxlan.CollectSession(ctx, session, vxlan.Request{Target: targetRecord(t.Device), FabricKey: c.FabricID, Dimensions: []string{"configuration"}})
		if err != nil {
			return err
		}
		beforeObs := t.PreEvidence.Dimensions["configuration"]
		_, sourceConfig, sourceErr := w.s.changeConfig(ctx, c.FabricID, c.SourceDeviceID)
		if sourceErr != nil || !t.PreEvidence.Complete() || len(beforeObs.Records) != 1 || opsHash(beforeObs.Records[0]) != t.ConfigHash || !plan.Eligible(beforeObs.Records[0]) || !aclServiceContextMatches(sourceConfig, beforeObs.Records[0], plan.Change()) {
			t.ErrorCode = "configuration_drift"
			return ConflictErr
		}
		if err = w.persistSnapshot(ctx, c.FabricID, 0, models.CollectionTarget{DeviceID: t.DeviceID, Device: t.Device, Connection: t.Connection, SourceHash: t.SourceHash}, t.PreEvidence, started, time.Now().UTC()); err != nil {
			return err
		}
		if err := w.writeChangeCommands(ctx, c, t, plan, held, session); err != nil {
			return err
		}
		postStarted := time.Now().UTC()
		t.PostEvidence, err = vxlan.CollectSession(ctx, session, vxlan.Request{Target: targetRecord(t.Device), FabricKey: c.FabricID, Dimensions: vxlan.DimensionNames()})
		if err != nil {
			return err
		}
		afterObs := t.PostEvidence.Dimensions["configuration"]
		if !afterObs.Complete() || len(afterObs.Records) != 1 || !plan.Satisfied(afterObs.Records[0], beforeObs.Records[0]) {
			t.ErrorCode = "postcheck_unconfirmed"
			return ConflictErr
		}
		t.ConfigurationVerified = true
		if err := w.persistSnapshot(ctx, c.FabricID, 0, models.CollectionTarget{DeviceID: t.DeviceID, Device: t.Device, Connection: t.Connection, SourceHash: t.SourceHash}, t.PostEvidence, postStarted, time.Now().UTC()); err != nil {
			return err
		}
		t.RuntimeState = postRuntimeState(t.PostEvidence, afterObs.Records[0])
		if t.RuntimeState != "healthy" {
			t.ErrorCode = "runtime_" + t.RuntimeState
			return ConflictErr
		}
		return nil
	})
	return transportClosed(e), e
}
func (w *Worker) persistChange(ctx context.Context, c *models.Change) error {
	return opsDB(ctx, w.s).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&models.Change{}).Where("id = ? AND status = ?", c.ID, "running").Update("targets", opsJSON(c.Targets))
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return ConflictErr
		}
		for _, t := range c.Targets {
			if e := tx.Model(&delivery.Target{}).Where("id = ? AND order_id = ?", t.DeliveryTargetID, c.DeliveryOrderID).Updates(map[string]any{"status": t.Status, "attempted": t.Attempted, "completed_steps": t.CompletedSteps, "error_code": t.ErrorCode}).Error; e != nil {
				return e
			}
		}
		return nil
	})
}
func (w *Worker) stopChange(ctx context.Context, c *models.Change, code string) error {
	status := "failed"
	for i := range c.Targets {
		t := &c.Targets[i]
		if t.Status == "uncertain" {
			status = "uncertain"
		}
		if t.Status == "running" {
			if t.Attempted {
				t.Status = "uncertain"
				status = "uncertain"
			} else {
				t.Status = "failed"
			}
			t.ErrorCode = code
		}
		if t.Status == "pending" {
			t.Status = "cancelled"
			t.ErrorCode = code
		}
	}
	for _, t := range c.Targets {
		if t.Status == "uncertain" && !t.ConfigurationVerified {
			if e := w.persistCollectionFailure(ctx, models.Collection{FabricID: c.FabricID, Mode: "all"}, models.CollectionTarget{DeviceID: t.DeviceID, Device: t.Device, Connection: t.Connection, SourceHash: t.SourceHash}, "failed", code); e != nil {
				return e
			}
		}
	}
	return w.finishChange(ctx, c, status)
}
func (w *Worker) finishChange(ctx context.Context, c *models.Change, status string) error {
	now := time.Now().UTC()
	return opsDB(ctx, w.s).Transaction(func(tx *gorm.DB) error {
		if e := tx.Model(&models.Change{}).Where("id = ? AND status = ?", c.ID, "running").Updates(map[string]any{"status": status, "targets": opsJSON(c.Targets), "finished_at": now, "version": gorm.Expr("version + 1")}).Error; e != nil {
			return e
		}
		counts := map[string]int{}
		for _, t := range c.Targets {
			counts[t.Status]++
			if e := tx.Model(&delivery.Target{}).Where("id = ? AND order_id = ?", t.DeliveryTargetID, c.DeliveryOrderID).Updates(map[string]any{"status": t.Status, "attempted": t.Attempted, "completed_steps": t.CompletedSteps, "error_code": t.ErrorCode, "finished_at": now}).Error; e != nil {
				return e
			}
		}
		return tx.Model(&delivery.Order{}).Where("id = ? AND kind = ?", c.DeliveryOrderID, "fabric").Updates(map[string]any{"status": status, "started_at": c.StartedAt, "finished_at": now, "succeeded_count": counts["succeeded"], "failed_count": counts["failed"], "uncertain_count": counts["uncertain"], "cancelled_count": counts["cancelled"], "version": gorm.Expr("version + 1")}).Error
	})
}

func postRuntimeState(snapshot vxlan.Snapshot, configuration vxlan.Record) string {
	state := "healthy"
	for _, dimension := range []string{"nve_peers", "nve_vnis", "bgp_neighbors", "ospf_neighbors", "vpc", "interfaces", "port_channels"} {
		if !applicable(dimension, configuration) {
			continue
		}
		o, ok := snapshot.Dimensions[dimension]
		if !ok || !o.Complete() {
			state = "unknown"
			continue
		}
		rows := o.Records
		if dimension == "interfaces" || dimension == "port_channels" {
			rows = fabricInterfaceRows(configuration, rows)
		}
		if missingExpectedRuntime(dimension, configuration, rows) {
			return "fault"
		}
		if len(rows) == 0 && (dimension == "nve_peers" || dimension == "nve_vnis" || dimension == "bgp_neighbors" || dimension == "ospf_neighbors" || dimension == "vpc") {
			state = "unknown"
			continue
		}
		if bad, _ := healthRows(dimension, rows); bad {
			return "fault"
		}
	}
	return state
}

func (w *Worker) checkLease(ctx context.Context, held lease.Lease) error {
	if e := lease.Check(opsDB(ctx, w.s), held); errors.Is(e, lease.LostErr) {
		return ConflictErr
	} else if e != nil {
		return e
	}
	return nil
}

func (w *Worker) recoverCollection(ctx context.Context, c models.Collection, now time.Time) error {
	for i := range c.Targets {
		if c.Targets[i].Status == "pending" || c.Targets[i].Status == "running" {
			c.Targets[i].Status = "failed"
			c.Targets[i].ErrorCode = "worker_lost"
			if e := w.persistCollectionFailure(ctx, c, c.Targets[i], "failed", "worker_lost"); e != nil {
				return e
			}
		}
	}
	if e := opsDB(ctx, w.s).Model(&models.Collection{}).Where("id = ? AND status = ?", c.ID, "running").Updates(map[string]any{"status": "interrupted", "targets": opsJSON(c.Targets), "finished_at": now, "version": gorm.Expr("version + 1")}).Error; e != nil {
		return e
	}
	return nil
}
func (w *Worker) recoverChange(ctx context.Context, c models.Change) error {
	for i := range c.Targets {
		t := &c.Targets[i]
		if t.Status == "running" {
			t.Status = "uncertain"
			t.ErrorCode = "worker_lost"
			if t.Attempted && !t.ConfigurationVerified {
				if e := w.persistCollectionFailure(ctx, models.Collection{FabricID: c.FabricID, Mode: "all"}, models.CollectionTarget{DeviceID: t.DeviceID, Device: t.Device, Connection: t.Connection, SourceHash: t.SourceHash}, "failed", "worker_lost"); e != nil {
					return e
				}
			}
		}
		if t.Status == "pending" {
			t.Status = "cancelled"
			t.ErrorCode = "prior_target_interrupted"
		}
	}
	if e := w.finishChange(ctx, &c, "uncertain"); e != nil {
		return e
	}
	return nil
}
func (w *Worker) scheduleFabric(ctx context.Context, f models.Fabric, now time.Time) error {
	var pending int64
	if e := opsDB(ctx, w.s).Model(&models.Collection{}).Where("fabric_id = ? AND status IN ?", f.ID, []string{"queued", "running"}).Count(&pending).Error; e != nil {
		return e
	}
	if pending > 0 {
		return nil
	}
	var devices []network.Device
	ids := opsDB(ctx, w.s).Model(&models.Membership{}).Select("device_id").Where("fabric_id = ? AND status = ?", f.ID, "assigned")
	if e := opsDB(ctx, w.s).Where("id IN (?) AND status IN ?", ids, []string{"pending", "active"}).Order("id").Find(&devices).Error; e != nil {
		return e
	}
	if len(devices) == 0 {
		return nil
	}
	for _, mode := range []string{"configuration", "runtime"} {
		if e := w.scheduleMode(ctx, f, devices, mode, now); e != nil {
			return e
		}
	}

	return nil
}
func (w *Worker) scheduleMode(ctx context.Context, f models.Fabric, devices []network.Device, mode string, now time.Time) error {
	minutes := f.RuntimeMinutes
	if mode == "configuration" {
		minutes = f.ConfigMinutes
	}
	if minutes <= 0 {
		if mode == "configuration" {
			minutes = 30
		} else {
			minutes = 5
		}
	}
	var recent models.Collection
	e := opsDB(ctx, w.s).Where("fabric_id = ? AND mode IN ?", f.ID, []string{mode, "all"}).Order("created_at DESC").First(&recent).Error
	if e == nil && now.Sub(recent.CreatedAt) < time.Duration(minutes)*time.Minute {
		return nil
	}
	if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
		return e
	}
	slot := now.Unix() / int64(minutes*60)
	key := fmt.Sprintf("schedule:%d:%s:%d", f.ID, mode, slot)
	if _, e := w.s.createCollection(ctx, f.ID, &dto.CollectionCreateReq{RequestKey: key, Mode: mode}, 0, devices); e != nil {
		return e
	}
	return nil
}
func (w *Worker) runChangeTargets(ctx context.Context, c *models.Change, plan *vxlan.ChangePlan) error {
	for i := range c.Targets {
		t := &c.Targets[i]
		if e := w.currentAuthorization(ctx, *c); e != nil {
			return w.stopChange(ctx, c, "authorization_changed")
		}
		if e := w.s.validateChange(ctx, *c); e != nil {
			return w.stopChange(ctx, c, "evidence_or_grouping_changed")
		}
		held, e := lease.Acquire(opsDB(ctx, w.s), t.DeviceID, lease.Holder{Kind: lease.DeliveryTarget, ID: t.DeliveryTargetID}, runTimeout)
		if code := leaseFailure(e, "device_busy"); code != "" {
			return w.stopChange(ctx, c, code)
		} else if e != nil {
			return e
		}
		t.Status = "running"
		if e := w.persistChange(ctx, c); e != nil {
			return e
		}
		closed, runErr := w.runChangeTarget(ctx, c, t, plan, held)
		releaseErr := w.release(ctx, held, closed)
		if releaseErr != nil {
			return releaseErr
		}
		if runErr != nil {
			if t.Attempted && t.RuntimeState != "fault" {
				t.Status = "uncertain"
			} else {
				t.Status = "failed"
			}
			if t.ErrorCode == "" {
				t.ErrorCode = "execution_failed"
			}
			stop, cancel := worker.Cleanup(ctx, 10*time.Second)
			defer cancel()
			return w.stopChange(stop, c, t.ErrorCode)
		}
		t.Status = "succeeded"
		if e := w.persistChange(ctx, c); e != nil {
			return e
		}
	}
	return w.finishChange(ctx, c, "succeeded")
}
func (w *Worker) writeChangeCommands(ctx context.Context, c *models.Change, t *models.ChangeTarget, plan *vxlan.ChangePlan, held lease.Lease, session Session) error {
	for i, command := range plan.Commands() {
		if e := w.authorizeChangeCommand(ctx, c, t, held); e != nil {
			return e
		}
		t.Attempted = true
		t.Receipts = append(t.Receipts, models.ChangeReceipt{Step: i + 1, Command: command, Status: "attempting", At: time.Now().UTC()})
		if e := w.persistChange(ctx, c); e != nil {
			return e
		}
		if e := session.Write(ctx, command); e != nil {
			t.ErrorCode = "write_outcome_unknown"
			return e
		}
		t.CompletedSteps++
		t.Receipts[len(t.Receipts)-1].Status = "acknowledged"
		if e := w.persistChange(ctx, c); e != nil {
			return e
		}
	}
	return nil
}
func (w *Worker) authorizeChangeCommand(ctx context.Context, c *models.Change, t *models.ChangeTarget, held lease.Lease) error {
	if e := w.checkLease(ctx, held); e != nil {
		return e
	}
	if e := w.s.validateChange(ctx, *c); e != nil {
		return e
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := w.checkSource(ctx, t.Device, t.Connection, t.SourceHash); e != nil {
		return e
	}
	if e := w.currentAuthorization(ctx, *c); e != nil {
		return e
	}
	group, e := w.s.groupingHash(ctx, c.FabricID)
	if e != nil || group != c.GroupingHash {
		return ConflictErr
	}
	return nil
}
