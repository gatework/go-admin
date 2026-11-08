package service

import (
	"context"
	"errors"
	wireless "github.com/gatework/go-wireless"
	delivery "go-admin/app/delivery/models"
	network "go-admin/app/network/models"
	"go-admin/app/wireless/models"
	"go-admin/app/wireless/service/dto"
	"testing"
	"time"
)

type collectFunc func(context.Context, network.Device, delivery.Connection, models.Collection) (wireless.Snapshot, bool, error)

func (f collectFunc) Collect(c context.Context, d network.Device, n delivery.Connection, j models.Collection) (wireless.Snapshot, bool, error) {
	return f(c, d, n, j)
}
func TestWorkerPublishesAndRejectsChangedSource(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "publish", true: "changed"}[changed], func(t *testing.T) {
			s, p := setup(t)
			ctx := context.Background()
			job, e := s.CreateCollection(ctx, 1, dto.CollectionReq{RequestKey: "worker-test", Dimensions: []wireless.Dimension{wireless.AccessPoints}}, 1, p)
			if e != nil {
				t.Fatal(e)
			}
			w := NewWorker(s, collectFunc(func(ctx context.Context, d network.Device, c delivery.Connection, j models.Collection) (wireless.Snapshot, bool, error) {
				if changed {
					if e := s.Orm.Model(&network.Device{}).Where("id = ?", 1).Update("version", 2).Error; e != nil {
						t.Fatal(e)
					}
				}
				return snapshot(t, 1, time.Now().UTC(), wireless.Complete, []wireless.Record{{"name": "AP-1", "operational_status": "run"}}), true, nil
			}))
			if e = w.Tick(ctx); e != nil {
				t.Fatal(e)
			}
			s.Orm.First(&job, job.ID)
			var count int64
			s.Orm.Model(&models.Resource{}).Count(&count)
			if changed {
				if job.Status != "failed" || count != 0 {
					t.Fatalf("stale publication: %+v %d", job, count)
				}
			} else if job.Status != "succeeded" || count != 1 {
				t.Fatalf("not published: %+v %d", job, count)
			}
			var locks int64
			s.Orm.Model(&delivery.DeviceLease{}).Count(&locks)
			if locks != 0 {
				t.Fatal("closed transport lease retained")
			}
		})
	}
}
func TestWorkerRetainsUnclosedLease(t *testing.T) {
	s, p := setup(t)
	ctx := context.Background()
	s.CreateCollection(ctx, 1, dto.CollectionReq{RequestKey: "worker-fail"}, 1, p)
	w := NewWorker(s, collectFunc(func(context.Context, network.Device, delivery.Connection, models.Collection) (wireless.Snapshot, bool, error) {
		return wireless.Snapshot{}, false, errors.New("secret failure")
	}))
	if e := w.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	var l delivery.DeviceLease
	s.Orm.First(&l, 1)
	if !l.Quarantined {
		t.Fatal("unknown closure reused")
	}
}
func TestAnalysisAndPartialEvidenceRemainHonest(t *testing.T) {
	s, p := setup(t)
	ctx := context.Background()
	now := time.Now().UTC()
	d, _ := s.controller(ctx, 1, p)
	c, _ := s.connection(ctx, d)
	src := source(d, c)
	if e := s.Persist(ctx, 1, src, snapshot(t, 1, now, wireless.Complete, []wireless.Record{{"name": "AP-1", "operational_status": "run"}, {"name": "AP-2", "operational_status": "idle"}}), now.Add(-time.Second), now.Add(time.Second)); e != nil {
		t.Fatal(e)
	}
	a, e := s.Analysis(ctx, 1, p)
	if e != nil {
		t.Fatal(e)
	}
	if len(a.Groups) == 0 || a.Groups[0].Affected != 1 || a.Groups[0].Healthy != 1 {
		t.Fatalf("bad analysis: %+v", a)
	}
	later := now.Add(time.Second)
	if e := s.Persist(ctx, 1, src, snapshot(t, 1, later, wireless.Partial, nil), now, later); e != nil {
		t.Fatal(e)
	}
	a, e = s.Analysis(ctx, 1, p)
	if e != nil {
		t.Fatal(e)
	}
	if a.Groups[0].Healthy != 0 || a.Groups[0].Unknown == 0 {
		t.Fatalf("partial called healthy: %+v", a)
	}
}
func TestTargetedNeighborsRetainUnselectedAP(t *testing.T) {
	s, p := setup(t)
	ctx := context.Background()
	now := time.Now().UTC()
	d, _ := s.controller(ctx, 1, p)
	c, _ := s.connection(ctx, d)
	src := source(d, c)
	s.Persist(ctx, 1, src, snapshot(t, 1, now, wireless.Complete, []wireless.Record{{"name": "A"}, {"name": "B"}}), now, now)
	save := func(ap string, rows []wireless.Record, status string) {
		t.Helper()
		now = now.Add(time.Second)
		o, _ := wireless.NewObservation(wireless.ObservationOptions{Name: wireless.LLDP, Status: wireless.Partial, Records: rows, Evidence: []wireless.Record{{"kind": "ap_lldp_result", "ap_name": ap, "status": status, "neighbor_count": len(rows)}}, CompletedAt: &now})
		sn, _ := wireless.NewSnapshot(wireless.SnapshotOptions{Target: wireless.Record{"controllerId": 1}, ObservedAt: now, Dimensions: map[wireless.Dimension]wireless.Observation{wireless.LLDP: o}})
		if e := s.Persist(ctx, 1, src, sn, now, now); e != nil {
			t.Fatal(e)
		}
	}
	save("B", []wireless.Record{{"ap_name": "B", "port_id": "Gi1", "system_name": "SW-B"}}, "complete")
	save("A", []wireless.Record{{"ap_name": "A", "port_id": "Gi2", "system_name": "SW-A"}}, "complete")
	var states []models.NeighborState
	s.Orm.Order("ap_name").Find(&states)
	if len(states) != 2 || len(states[0].Records) != 1 || len(states[1].Records) != 1 {
		t.Fatalf("lost scoped evidence: %+v", states)
	}
	save("A", nil, "failed")
	s.Orm.Where("ap_name = ?", "A").First(&states[0])
	if len(states[0].Records) != 1 || states[0].Status != "failed" {
		t.Fatal("failure cleared candidates")
	}
	save("A", nil, "complete")
	s.Orm.Where("ap_name = ?", "A").First(&states[0])
	if len(states[0].Records) != 0 {
		t.Fatal("complete empty ignored")
	}
}

func TestNeighborAssociationRejectsUnprovenEvidence(t *testing.T) {
	s, p := setup(t)
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Minute)
	d, _ := s.controller(ctx, 1, p)
	c, _ := s.connection(ctx, d)
	if e := s.Persist(ctx, 1, source(d, c), snapshot(t, 1, now, wireless.Complete, []wireless.Record{{"name": "A"}}), now, now); e != nil {
		t.Fatal(e)
	}
	var ap models.Resource
	s.Orm.Where("kind = ?", "access_points").First(&ap)
	detail, e := s.APDetail(ctx, 1, ap.ID, p)
	if e != nil || detail.Resource.Fields.Name != "A" {
		t.Fatal(e, detail)
	}
	if _, e = s.Associate(ctx, 1, ap.ID, dto.NeighborReq{Version: 1, ObservationVersion: 1, EvidenceID: "invalid"}, p); !errors.Is(e, ErrConflict) {
		t.Fatal("unproven association", e)
	}
}
