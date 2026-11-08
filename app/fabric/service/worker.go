package service

import (
	"context"
	"fmt"
	vxlan "github.com/gatework/go-vxlan"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"github.com/google/uuid"
	delivery "go-admin/app/delivery/models"
	ds "go-admin/app/delivery/service"
	"go-admin/app/fabric/models"
	"go-admin/app/fabric/service/dto"
	network "go-admin/app/network/models"
	"gorm.io/gorm"
	"sync"
	"time"
)

type Worker struct {
	s            *Fabric
	transport    Transport
	cancel       context.CancelFunc
	done         chan struct{}
	start        sync.Once
	stop         sync.Once
	lastSchedule time.Time
}

func NewWorker(s *Fabric, t Transport) *Worker {
	if t == nil {
		t = nativeTransport{config: ds.FromEnvironment()}
	}
	return &Worker{s: s, transport: t, done: make(chan struct{})}
}
func (w *Worker) Start() {
	w.start.Do(func() {
		ctx, cancel := context.WithCancel(context.Background())
		w.cancel = cancel
		go func() {
			defer close(w.done)
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if e := w.Tick(ctx); e != nil && w.s.Log != nil {
						w.s.Log.Errorf("Fabric worker: %v", e)
					}
				}
			}
		}()
	})
}
func (w *Worker) Close(ctx context.Context) {
	w.stop.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
	})
	if w.cancel == nil {
		return
	}
	select {
	case <-w.done:
	case <-ctx.Done():
	}
}
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
	if e := opsDB(w.s, ctx).Where("status = ?", "queued").Order("id").Limit(20).Find(&jobs).Error; e != nil {
		return e
	}
	for _, job := range jobs {
		token := uuid.NewString()
		if e := w.acquire(ctx, -job.FabricID, token, -job.ID); e != nil {
			continue
		}
		e := w.runCollection(ctx, job)
		release := w.release(context.WithoutCancel(ctx), -job.FabricID, token, true)
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
	if e := opsDB(w.s, ctx).Where("status = ?", "queued").Order("id").Limit(20).Find(&changes).Error; e != nil {
		return e
	}
	for _, c := range changes {
		token := uuid.NewString()
		if e := w.acquire(ctx, -c.FabricID, token, -c.ID); e != nil {
			continue
		}
		e := w.runChange(ctx, c)
		release := w.release(context.WithoutCancel(ctx), -c.FabricID, token, true)
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
func (w *Worker) schedule(ctx context.Context, now time.Time) error {
	var identities []network.Device
	if e := opsDB(w.s, ctx).Select("id", "name", "device_role").Find(&identities).Error; e != nil {
		return e
	}
	if e := w.s.reconcile(ctx, identities); e != nil {
		return e
	}
	var fabrics []models.Fabric
	if e := opsDB(w.s, ctx).Where("collection_enabled = ?", true).Find(&fabrics).Error; e != nil {
		return e
	}
	for _, f := range fabrics {
		var pending int64
		if e := opsDB(w.s, ctx).Model(&models.Collection{}).Where("fabric_id = ? AND status IN ?", f.ID, []string{"queued", "running"}).Count(&pending).Error; e != nil {
			return e
		}
		if pending > 0 {
			continue
		}
		var devices []network.Device
		ids := opsDB(w.s, ctx).Model(&models.Membership{}).Select("device_id").Where("fabric_id = ? AND status = ?", f.ID, "assigned")
		if e := opsDB(w.s, ctx).Where("id IN (?) AND status = ?", ids, "active").Order("id").Find(&devices).Error; e != nil {
			return e
		}
		if len(devices) == 0 {
			continue
		}
		for _, mode := range []string{"configuration", "runtime"} {
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
			e := opsDB(w.s, ctx).Where("fabric_id = ? AND mode IN ?", f.ID, []string{mode, "all"}).Order("created_at DESC").First(&recent).Error
			if e == nil && now.Sub(recent.CreatedAt) < time.Duration(minutes)*time.Minute {
				continue
			}
			if e != nil && e != gorm.ErrRecordNotFound {
				return e
			}
			slot := now.Unix() / int64(minutes*60)
			key := fmt.Sprintf("schedule:%d:%s:%d", f.ID, mode, slot)
			if _, e := w.s.createCollection(ctx, f.ID, &dto.CollectionCreateReq{RequestKey: key, Mode: mode}, 0, devices); e != nil {
				return e
			}
		}
	}
	return nil
}

// Recover records interrupted runs once their execution budget has elapsed.
// It never steals expired device leases or requeues a possibly attempted write.
func (w *Worker) Recover(ctx context.Context) error {
	now := time.Now().UTC()
	var collects []models.Collection
	if e := opsDB(w.s, ctx).Where("status = ? AND lease_until < ?", "running", now).Find(&collects).Error; e != nil {
		return e
	}
	for _, c := range collects {
		for i := range c.Targets {
			if c.Targets[i].Status == "pending" || c.Targets[i].Status == "running" {
				c.Targets[i].Status = "failed"
				c.Targets[i].ErrorCode = "worker_lost"
				if e := w.persistCollectionFailure(ctx, c, c.Targets[i], "failed", "worker_lost"); e != nil {
					return e
				}
			}
		}
		if e := opsDB(w.s, ctx).Model(&models.Collection{}).Where("id = ? AND status = ?", c.ID, "running").Updates(map[string]any{"status": "interrupted", "targets": opsJSON(c.Targets), "finished_at": now, "version": gorm.Expr("version + 1")}).Error; e != nil {
			return e
		}
	}
	var changes []models.Change
	if e := opsDB(w.s, ctx).Where("status = ? AND lease_until < ?", "running", now).Find(&changes).Error; e != nil {
		return e
	}
	for _, c := range changes {
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
	}
	// Fabric coordinator locks own no transport. Their bounded run has expired;
	// physical device leases remain quarantined.
	if e := opsDB(w.s, ctx).Where("device_id < 0 AND expires_at < ?", now).Delete(&delivery.DeviceLease{}).Error; e != nil {
		return e
	}
	// Expired device leases remain quarantined until an operator establishes transport closure.
	return opsDB(w.s, ctx).Model(&delivery.DeviceLease{}).Where("expires_at < ?", now).Update("quarantined", true).Error
}
func (w *Worker) currentAuthorization(ctx context.Context, c models.Change) error {
	actor := c.ExecuteBy
	if actor <= 0 {
		actor = c.CreateBy
	}
	var p actions.DataPermission
	q := opsDB(w.s, ctx).Table("sys_user").Select("sys_user.user_id,sys_user.dept_id,sys_role.role_id,sys_role.data_scope").Joins("JOIN sys_role ON sys_role.role_id = sys_user.role_id").Where("sys_user.user_id = ? AND sys_user.status = ? AND sys_role.status = ? AND sys_user.deleted_at = 0 AND sys_role.deleted_at = 0", actor, "2", "2").Scan(&p)
	if q.Error != nil {
		return q.Error
	}
	if p.UserId == 0 {
		return errOpsScope
	}
	var role struct{ RoleKey string }
	if e := opsDB(w.s, ctx).Table("sys_role").Select("role_key").Where("role_id = ?", p.RoleId).Scan(&role).Error; e != nil {
		return e
	}
	if role.RoleKey != "admin" {
		var grants int64
		if e := opsDB(w.s, ctx).Table("sys_role_menu rm").Joins("JOIN sys_menu m ON m.menu_id = rm.menu_id").Where("rm.role_id = ? AND m.permission = ? AND m.deleted_at = 0", p.RoleId, "network:fabric:execute").Count(&grants).Error; e != nil {
			return e
		}
		if grants == 0 {
			return errOpsScope
		}
	}
	devices, e := w.s.RequireFullScope(ctx, c.FabricID, &p)
	if e != nil {
		return e
	}
	ids := []int{}
	for _, d := range devices {
		ids = append(ids, d.Id)
	}
	if opsHash(ids) != opsHash(c.Authorization) {
		return errOpsScope
	}
	return nil
}
func (w *Worker) runChange(parent context.Context, c models.Change) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	now := time.Now().UTC()
	c.StartedAt = &now
	until := now.Add(15 * time.Minute)
	q := opsDB(w.s, ctx).Model(&models.Change{}).Where("id = ? AND status = ?", c.ID, "queued").Updates(map[string]any{"status": "running", "started_at": now, "lease_until": until, "version": gorm.Expr("version + 1")})
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
	if e := opsDB(w.s, ctx).First(&sd, c.SourceDeviceID).Error; e != nil {
		return e
	}
	sc, e := w.s.deviceConnection(ctx, sd)
	if e != nil {
		return e
	}
	source = models.CollectionTarget{DeviceID: sd.Id, Device: opsDevice(sd), Connection: sc, SourceHash: c.SourceHash}
	sourceToken := uuid.NewString()
	if e := w.acquire(ctx, sd.Id, sourceToken, 0); e != nil {
		return w.stopChange(ctx, &c, "source_busy")
	}
	sourceClosed := false
	defer func() { _ = w.release(context.WithoutCancel(ctx), sd.Id, sourceToken, sourceClosed) }()
	started := time.Now().UTC()
	var sourceSnapshot vxlan.Snapshot
	e = w.transport.Open(ctx, source.Device, source.Connection, func(session FabricSession) error {
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
	for i := range c.Targets {
		t := &c.Targets[i]
		if e := w.currentAuthorization(ctx, c); e != nil {
			return w.stopChange(ctx, &c, "authorization_changed")
		}
		if e := w.s.validateChange(ctx, c); e != nil {
			return w.stopChange(ctx, &c, "evidence_or_grouping_changed")
		}
		token := uuid.NewString()
		if e := w.acquire(ctx, t.DeviceID, token, t.DeliveryTargetID); e != nil {
			return w.stopChange(ctx, &c, "device_busy")
		}
		t.Status = "running"
		if e := w.persistChange(ctx, &c); e != nil {
			return e
		}
		closed, runErr := w.runChangeTarget(ctx, &c, t, plan, token)
		releaseErr := w.release(context.WithoutCancel(ctx), t.DeviceID, token, closed)
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
			return w.stopChange(context.WithoutCancel(ctx), &c, t.ErrorCode)
		}
		t.Status = "succeeded"
		if e := w.persistChange(ctx, &c); e != nil {
			return e
		}
	}
	return w.finishChange(ctx, &c, "succeeded")
}
func (w *Worker) runChangeTarget(ctx context.Context, c *models.Change, t *models.ChangeTarget, plan *vxlan.ChangePlan, token string) (bool, error) {
	started := time.Now().UTC()
	e := w.checkSource(ctx, t.Device, t.Connection, t.SourceHash)
	if e != nil {
		return true, e
	}

	e = w.transport.Open(ctx, t.Device, t.Connection, func(session FabricSession) error {
		var err error
		t.PreEvidence, err = vxlan.CollectSession(ctx, session, vxlan.Request{Target: targetRecord(t.Device), FabricKey: c.FabricID, Dimensions: []string{"configuration"}})
		if err != nil {
			return err
		}
		beforeObs := t.PreEvidence.Dimensions["configuration"]
		_, sourceConfig, sourceErr := w.s.changeConfig(ctx, c.FabricID, c.SourceDeviceID)
		if sourceErr != nil || !t.PreEvidence.Complete() || len(beforeObs.Records) != 1 || opsHash(beforeObs.Records[0]) != t.ConfigHash || !plan.Eligible(beforeObs.Records[0]) || !aclServiceContextMatches(sourceConfig, beforeObs.Records[0], plan.Change()) {
			t.ErrorCode = "configuration_drift"
			return errOpsConflict
		}
		if err = w.persistSnapshot(ctx, c.FabricID, 0, models.CollectionTarget{DeviceID: t.DeviceID, Device: t.Device, Connection: t.Connection, SourceHash: t.SourceHash}, t.PreEvidence, started, time.Now().UTC()); err != nil {
			return err
		}
		for i, command := range plan.Commands() {
			if e := w.checkLease(ctx, t.DeviceID, token); e != nil {
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
				return errOpsConflict
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
		postStarted := time.Now().UTC()
		t.PostEvidence, err = vxlan.CollectSession(ctx, session, vxlan.Request{Target: targetRecord(t.Device), FabricKey: c.FabricID, Dimensions: vxlan.DimensionNames()})
		if err != nil {
			return err
		}
		afterObs := t.PostEvidence.Dimensions["configuration"]
		if !afterObs.Complete() || len(afterObs.Records) != 1 || !plan.Satisfied(afterObs.Records[0], beforeObs.Records[0]) {
			t.ErrorCode = "postcheck_unconfirmed"
			return errOpsConflict
		}
		t.ConfigurationVerified = true
		if err := w.persistSnapshot(ctx, c.FabricID, 0, models.CollectionTarget{DeviceID: t.DeviceID, Device: t.Device, Connection: t.Connection, SourceHash: t.SourceHash}, t.PostEvidence, postStarted, time.Now().UTC()); err != nil {
			return err
		}
		t.RuntimeState = postRuntimeState(t.PostEvidence, afterObs.Records[0])
		if t.RuntimeState != "healthy" {
			t.ErrorCode = "runtime_" + t.RuntimeState
			return errOpsConflict
		}
		return nil
	})
	return transportClosed(e), e
}
func (w *Worker) persistChange(ctx context.Context, c *models.Change) error {
	return opsDB(w.s, ctx).Transaction(func(tx *gorm.DB) error {
		q := tx.Model(&models.Change{}).Where("id = ? AND status = ?", c.ID, "running").Update("targets", opsJSON(c.Targets))
		if q.Error != nil {
			return q.Error
		}
		if q.RowsAffected != 1 {
			return errOpsConflict
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
	return opsDB(w.s, ctx).Transaction(func(tx *gorm.DB) error {
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

func (w *Worker) checkLease(ctx context.Context, id int, token string) error {
	var lease delivery.DeviceLease
	if e := opsDB(w.s, ctx).First(&lease, "device_id = ? AND token = ?", id, token).Error; e != nil {
		return e
	}
	if lease.Quarantined || time.Now().UTC().After(lease.ExpiresAt) {
		return errOpsConflict
	}
	return nil
}
