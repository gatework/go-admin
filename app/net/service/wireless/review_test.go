package wireless

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"gorm.io/gorm"

	wireless "github.com/gatework/go-wireless"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/delivery"
	network "github.com/gatework/motor/app/net/models/inventory"
	models "github.com/gatework/motor/app/net/models/wireless"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	dto "github.com/gatework/motor/app/net/service/dto/wireless"
	"github.com/gatework/motor/internal/testconfig"
)

func currentSource(t *testing.T, s *Wireless) string {
	t.Helper()
	d, e := s.controller(context.Background(), 1, &actions.DataPermission{DataScope: actions.DataScopeAll})
	if e != nil {
		t.Fatal(e)
	}
	c, e := s.connection(context.Background(), d)
	if e != nil {
		t.Fatal(e)
	}
	return source(d, c)
}
func saveDimensions(t *testing.T, s *Wireless, src string, at time.Time, observations map[wireless.Dimension]wireless.Observation) {
	t.Helper()
	snap, e := wireless.NewSnapshot(wireless.SnapshotOptions{Target: wireless.Record{"controllerId": 1}, ObservedAt: at, Dimensions: observations})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Persist(context.Background(), 1, src, snap, at, at); e != nil {
		t.Fatal(e)
	}
}
func observation(t *testing.T, name wireless.Dimension, status wireless.Status, at time.Time, records, evidence []wireless.Record) wireless.Observation {
	t.Helper()
	o, e := wireless.NewObservation(wireless.ObservationOptions{Name: name, Status: status, Records: records, Evidence: evidence, CompletedAt: &at})
	if e != nil {
		t.Fatal(e)
	}
	return o
}
func neighborFixture(t *testing.T, s *Wireless, p *actions.DataPermission) (dto.APDetail, time.Time) {
	t.Helper()
	at := time.Now().UTC().Add(-time.Minute)
	src := currentSource(t, s)
	saveDimensions(t, s, src, at, map[wireless.Dimension]wireless.Observation{wireless.AccessPoints: observation(t, wireless.AccessPoints, wireless.Complete, at, []wireless.Record{{"name": "A", "serial_id": "old-A", "operational_status": "run"}, {"name": "B", "serial_id": "old-B", "operational_status": "run"}}, nil)})
	at = at.Add(time.Second)
	rows := []wireless.Record{{"ap_name": "A", "remote_interface": "Gi1", "remote_name": "SW-A"}, {"ap_name": "B", "remote_interface": "Gi2", "remote_name": "SW-B"}}
	evidence := []wireless.Record{{"kind": "ap_lldp_result", "ap_name": "A", "status": "complete", "neighbor_count": 1}, {"kind": "ap_lldp_result", "ap_name": "B", "status": "complete", "neighbor_count": 1}}
	saveDimensions(t, s, src, at, map[wireless.Dimension]wireless.Observation{wireless.LLDP: observation(t, wireless.LLDP, wireless.Complete, at, rows, evidence)})
	var ap models.Resource
	if e := s.DB.Where("kind = ?", "access_points").Order("id").First(&ap).Error; e != nil {
		t.Fatal(e)
	}
	detail, e := s.APDetail(context.Background(), 1, ap.ID, p)
	if e != nil || detail.NeighborCoverage.State != "complete" {
		t.Fatal(e, detail)
	}
	return detail, at
}
func TestSelectedAPFailureWithoutPerAPEvidenceInvalidatesOnlyItsNeighbors(t *testing.T) {
	s, p := setup(t)
	before, _ := neighborFixture(t, s, p)
	ctx := context.Background()
	if _, e := s.CreateCollection(ctx, 1, dto.CollectionReq{RequestKey: "lldp-no-evidence", Dimensions: []wireless.Dimension{wireless.LLDP}, APNames: []string{"A"}}, p); e != nil {
		t.Fatal(e)
	}
	w := NewWorker(s, collectFunc(func(context.Context, network.Device, delivery.Connection, models.Collection) (wireless.Snapshot, bool, error) {
		at := time.Now().UTC()
		snap, e := wireless.NewSnapshot(wireless.SnapshotOptions{Target: wireless.Record{"controllerId": 1}, ObservedAt: at, Dimensions: map[wireless.Dimension]wireless.Observation{wireless.AccessPoints: observation(t, wireless.AccessPoints, wireless.Partial, at, nil, nil), wireless.LLDP: observation(t, wireless.LLDP, wireless.Failed, at, nil, nil)}})
		return snap, true, e
	}))
	if e := w.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	after, e := s.APDetail(ctx, 1, before.Resource.ID, p)
	if e != nil {
		t.Fatal(e)
	}
	if after.NeighborCoverage.State == "complete" || len(after.Neighbors) != 1 {
		t.Fatalf("failed attempt still authoritative or facts lost: %+v", after.NeighborCoverage)
	}
	if _, e = s.Associate(ctx, 1, before.Resource.ID, dto.NeighborReq{Version: before.Resource.Version, ObservationVersion: before.NeighborCoverage.Version, EvidenceID: before.Neighbors[0].EvidenceID}, p); !errors.Is(e, ConflictErr) {
		t.Fatalf("old association accepted: %v", e)
	}
	var b models.NeighborState
	if e = s.DB.Where("ap_name = ?", "B").First(&b).Error; e != nil || b.Status != "complete" {
		t.Fatal(e, b)
	}
}
func TestReplacementAPCannotInheritNeighborEvidence(t *testing.T) {
	s, p := setup(t)
	old, at := neighborFixture(t, s, p)
	at = at.Add(time.Second)
	if e := s.Persist(context.Background(), 1, currentSource(t, s), snapshot(t, 1, at, wireless.Complete, []wireless.Record{{"name": "A", "serial_id": "new-A"}}), at, at); e != nil {
		t.Fatal(e)
	}
	var ap models.Resource
	s.DB.Where("kind = ? AND missing = ?", "access_points", false).First(&ap)
	if ap.ID == old.Resource.ID {
		t.Fatal("fixture did not replace AP")
	}
	detail, e := s.APDetail(context.Background(), 1, ap.ID, p)
	if e != nil {
		t.Fatal(e)
	}
	if detail.NeighborCoverage.State == "complete" || len(detail.Neighbors) != 0 {
		t.Fatal("replacement inherited prior hardware evidence")
	}
}
func TestNeighborReferencesFollowTargetReadScope(t *testing.T) {
	s, p := setup(t)
	detail, _ := neighborFixture(t, s, p)
	ctx := context.Background()
	sw := network.Device{DeviceType: "switch", Status: "active", Version: 1}
	sw.SetCreateBy(2)
	if e := s.DB.Create(&sw).Error; e != nil {
		t.Fatal(e)
	}
	id := sw.ID
	if _, e := s.Associate(ctx, 1, detail.Resource.ID, dto.NeighborReq{Version: 1, ObservationVersion: detail.NeighborCoverage.Version, EvidenceID: detail.Neighbors[0].EvidenceID, DeviceID: &id, Interface: "Gi1"}, p); e != nil {
		t.Fatal(e)
	}

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	restricted := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 1}
	rows, _, e := s.ResourcePage(ctx, 1, "access_points", &dto.PageReq{}, restricted)
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range rows {
		if r.NeighborDeviceID != nil || r.NeighborInterface != "" {
			t.Fatal("restricted reference leaked through list")
		}
	}
	if _, e = s.Management(ctx, 1, detail.Resource.ID, dto.ManagementReq{Version: 2, LocalAlias: "mine"}, restricted); e != nil {
		t.Fatal(e)
	}
	after, e := s.APDetail(ctx, 1, detail.Resource.ID, restricted)
	if e != nil || after.Resource.NeighborDeviceID != nil || after.Resource.NeighborInterface != "" {
		t.Fatal("restricted detail leaked", e)
	}
	var stored models.Resource
	s.DB.First(&stored, detail.Resource.ID)
	if stored.NeighborDeviceID == nil || *stored.NeighborDeviceID != id {
		t.Fatal("scope filtering mutated stored association")
	}
}
func TestLLDPProjectionPreservesAnInterleavedManagementEdit(t *testing.T) {
	s, p := setup(t)
	detail, at := neighborFixture(t, s, p)
	at = at.Add(time.Second)
	applied := false
	e := s.DB.Callback().Update().Before("gorm:update").Register("wireless_interleaved_edit", func(tx *gorm.DB) {
		if applied || tx.Statement.Table != (models.Resource{}).TableName() {
			return
		}
		if row, ok := tx.Statement.Dest.(*models.Resource); ok && row.ID == detail.Resource.ID {
			applied = true
			if e := tx.Exec("UPDATE sys_wireless_resource SET local_alias = ?, version = ?, neighbor_interface = ? WHERE id = ?", "concurrent", 2, "Gi99", row.ID).Error; e != nil {
				tx.AddError(e)
			}
		}
	})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.DB.Callback().Update().Remove("wireless_interleaved_edit") })
	saveDimensions(t, s, currentSource(t, s), at, map[wireless.Dimension]wireless.Observation{wireless.LLDP: observation(t, wireless.LLDP, wireless.Partial, at, nil, []wireless.Record{{"kind": "ap_lldp_result", "ap_name": "A", "status": "failed"}})})
	var ap models.Resource
	s.DB.First(&ap, detail.Resource.ID)
	if !applied || ap.LocalAlias != "concurrent" || ap.Version != 2 || ap.NeighborInterface != "Gi99" {
		t.Fatalf("manual edit overwritten: applied=%v row=%+v", applied, ap)
	}
}
func TestChildHealthRequiresCurrentStableAPParent(t *testing.T) {
	for _, mode := range []string{"retired", "replacement", "source_changed"} {
		t.Run(mode, func(t *testing.T) {
			s, p := setup(t)
			_, at := neighborFixture(t, s, p)
			at = at.Add(time.Second)
			src := currentSource(t, s)
			children := []wireless.Record{{"ap_name": "A", "radio_id": "1", "client_mac": "00:11:22:33:44:55", "ssid": "CORP"}}
			saveDimensions(t, s, src, at, map[wireless.Dimension]wireless.Observation{wireless.Clients: observation(t, wireless.Clients, wireless.Complete, at, children, nil)})
			at = at.Add(time.Second)
			if mode == "source_changed" {
				s.DB.Model(&network.Device{}).Where("id = ?", 1).Update("version", 2)
				saveDimensions(t, s, currentSource(t, s), at, map[wireless.Dimension]wireless.Observation{wireless.Clients: observation(t, wireless.Clients, wireless.Complete, at, children, nil)})
			} else {
				rows := []wireless.Record{}
				if mode == "replacement" {
					rows = append(rows, wireless.Record{"name": "A", "serial_id": "replacement"})
				}
				if e := s.Persist(context.Background(), 1, src, snapshot(t, 1, at, wireless.Complete, rows), at, at); e != nil {
					t.Fatal(e)
				}
			}
			rows, _, e := s.ResourcePage(context.Background(), 1, "clients", &dto.PageReq{}, p)
			if e != nil || len(rows) != 1 {
				t.Fatal(e, rows)
			}
			if rows[0].Health == "healthy" {
				t.Fatal("unproven parent still healthy")
			}
		})
	}
}
func TestFailedTransportInvalidatesCoverageWithoutLosingFacts(t *testing.T) {
	for _, mode := range []string{"error", "uncertain", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			s, p := setup(t)
			detail, _ := neighborFixture(t, s, p)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, e := s.CreateCollection(ctx, 1, dto.CollectionReq{RequestKey: "fail-" + mode, Dimensions: []wireless.Dimension{wireless.AccessPoints}}, p)
			if e != nil {
				t.Fatal(e)
			}
			w := NewWorker(s, collectFunc(func(context.Context, network.Device, delivery.Connection, models.Collection) (wireless.Snapshot, bool, error) {
				if mode == "cancel" {
					cancel()
				}
				return wireless.Snapshot{}, mode != "uncertain", errors.New("sensitive transport body")
			}))
			if e = w.Tick(ctx); e != nil {
				t.Fatal(e)
			}
			after, e := s.APDetail(context.Background(), 1, detail.Resource.ID, p)
			if e != nil {
				t.Fatal(e)
			}
			if after.Resource.Health == "healthy" || after.Resource.Fields.Name != "A" || after.Resource.Coverage.Status != "failed" {
				t.Fatal("old facts still authoritative after failure", after.Resource.Coverage)
			}
		})
	}
}
func TestBlockedControllerCannotStarveLaterQueuedWork(t *testing.T) {
	s, p := setup(t)
	ctx := context.Background()
	for i := range 20 {
		if _, e := s.CreateCollection(ctx, 1, dto.CollectionReq{RequestKey: time.Unix(int64(i), 0).String(), Dimensions: []wireless.Dimension{wireless.AccessPoints}}, p); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.DB.Create(&delivery.DeviceLease{DeviceID: 1, Token: "quarantined", Quarantined: true, ExpiresAt: time.Now().Add(time.Hour)}).Error; e != nil {
		t.Fatal(e)
	}
	job, e := s.CreateCollection(ctx, 2, dto.CollectionReq{RequestKey: "eligible", Dimensions: []wireless.Dimension{wireless.AccessPoints}}, p)
	if e != nil {
		t.Fatal(e)
	}
	w := NewWorker(s, collectFunc(func(_ context.Context, d network.Device, _ delivery.Connection, _ models.Collection) (wireless.Snapshot, bool, error) {
		return snapshot(t, d.ID, time.Now().UTC(), wireless.Complete, nil), true, nil
	}))
	if e = w.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	s.DB.First(&job, job.ID)
	if job.Status != "succeeded" {
		t.Fatal("eligible job starved", job.Status)
	}
}

// Collection only reads, so a controller still pending review may be
// collected; one in maintenance, disabled or retired may not.
func TestCollectionFollowsTheReadUseOfDeviceStatus(t *testing.T) {
	s, p := setup(t)
	ctx := context.Background()
	for i, tc := range []struct {
		status string
		want   error
	}{{"pending", nil}, {"maintenance", deviceaccess.UnavailableErr}, {"retired", deviceaccess.UnavailableErr}} {
		if e := s.DB.Model(&network.Device{}).Where("id = ?", 1).Update("status", tc.status).Error; e != nil {
			t.Fatal(e)
		}
		_, e := s.CreateCollection(ctx, 1, dto.CollectionReq{RequestKey: "status-" + tc.status + strconv.Itoa(i), Dimensions: []wireless.Dimension{wireless.AccessPoints}}, p)
		if !errors.Is(e, tc.want) {
			t.Errorf("%s controller: %v, want %v", tc.status, e, tc.want)
		}
	}
}
