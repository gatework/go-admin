package service

import (
	"context"
	"errors"
	wireless "github.com/gatework/go-wireless"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	base "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	delivery "go-admin/app/delivery/models"
	network "go-admin/app/network/models"
	"go-admin/app/wireless/models"
	"go-admin/app/wireless/service/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"strings"
	"testing"
	"time"
)

func setup(t *testing.T) (*Wireless, *actions.DataPermission) {
	t.Helper()
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	if e = db.AutoMigrate(append(Tables(), &network.Device{}, &delivery.Connection{}, &delivery.DeviceLease{})...); e != nil {
		t.Fatal(e)
	}
	for i := 1; i <= 2; i++ {
		d := network.Device{SourceFacts: network.SourceFacts{IP: []string{"192.0.2.1", "192.0.2.2"}[i-1], Name: "AC", Vendor: "H3C", SourceTags: []string{}}, DeviceType: "wireless_controller", Status: "active", Tags: []string{}, Version: 1}
		d.SetCreateBy(i)
		if e = db.Create(&d).Error; e != nil {
			t.Fatal(e)
		}
	}
	return &Wireless{Service: base.Service{Orm: db}}, &actions.DataPermission{DataScope: actions.DataScopeAll}
}
func snapshot(t *testing.T, id int, at time.Time, status wireless.Status, rows []wireless.Record) wireless.Snapshot {
	t.Helper()
	o, e := wireless.NewObservation(wireless.ObservationOptions{Name: wireless.AccessPoints, Status: status, Records: rows, CompletedAt: &at})
	if e != nil {
		t.Fatal(e)
	}
	s, e := wireless.NewSnapshot(wireless.SnapshotOptions{Target: wireless.Record{"controllerId": id}, ObservedAt: at, Dimensions: map[wireless.Dimension]wireless.Observation{wireless.AccessPoints: o}})
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func persist(t *testing.T, s *Wireless, id int, at time.Time, status wireless.Status, rows []wireless.Record) error {
	t.Helper()
	return s.Persist(context.Background(), id, "source", snapshot(t, id, at, status, rows), at.Add(-time.Second), at.Add(time.Second))
}
func TestScopedIdentityRetentionAndCompleteZero(t *testing.T) {
	s, p := setup(t)
	now := time.Now().UTC()
	rows := []wireless.Record{{"ap_id": "01", "name": "AP", "mac_address": "0011.2233.4455", "operational_status": "run"}}
	for _, id := range []int{1, 2} {
		if e := persist(t, s, id, now, wireless.Complete, rows); e != nil {
			t.Fatal(e)
		}
	}
	aps, _, e := s.Resources(context.Background(), 1, "access_points", &dto.PageReq{}, p)
	if e != nil || len(aps) != 1 {
		t.Fatalf("%v %v", aps, e)
	}
	id := aps[0].ID
	if _, e = s.Management(context.Background(), 1, id, dto.ManagementReq{Version: 1, LocalAlias: "Human"}, p); e != nil {
		t.Fatal(e)
	}
	if e = persist(t, s, 1, now.Add(time.Second), wireless.Partial, nil); e != nil {
		t.Fatal(e)
	}
	var row models.Resource
	s.Orm.First(&row, id)
	if row.Missing || row.LocalAlias != "Human" {
		t.Fatal(row)
	}
	var state models.Dimension
	s.Orm.Where("controller_id = ? AND name = ?", 1, "access_points").First(&state)
	if state.Latest.Status() != wireless.Partial || len(state.LastComplete.Records()) != 1 {
		t.Fatal("lost complete evidence")
	}
	if e = persist(t, s, 1, now.Add(2*time.Second), wireless.Complete, nil); e != nil {
		t.Fatal(e)
	}
	s.Orm.First(&row, id)
	if !row.Missing || row.LocalAlias != "Human" {
		t.Fatal("human metadata removed")
	}
	if e = persist(t, s, 1, now.Add(3*time.Second), wireless.Complete, rows); e != nil {
		t.Fatal(e)
	}
	s.Orm.First(&row, id)
	if row.Missing || row.LocalAlias != "Human" {
		t.Fatal("storage identity changed")
	}
	other, _, _ := s.Resources(context.Background(), 2, "access_points", &dto.PageReq{}, p)
	if len(other) != 1 || other[0].ID == id {
		t.Fatal("cross-controller collision")
	}
}
func TestDuplicateEvidenceAndLateWritesRejected(t *testing.T) {
	s, _ := setup(t)
	now := time.Now().UTC()
	r := wireless.Record{"ap_id": "1", "name": "AP"}
	if e := persist(t, s, 1, now, wireless.Complete, []wireless.Record{r, r}); !errors.Is(e, ErrInvalid) {
		t.Fatalf("duplicates: %v", e)
	}
	if e := persist(t, s, 1, now, wireless.Complete, []wireless.Record{r}); e != nil {
		t.Fatal(e)
	}
	if e := persist(t, s, 1, now.Add(-time.Second), wireless.Complete, nil); !errors.Is(e, ErrConflict) {
		t.Fatal("late overwrite", e)
	}
	if e := s.Persist(context.Background(), 1, "source", snapshot(t, 2, now, wireless.Complete, nil), now, now); !errors.Is(e, ErrInvalid) {
		t.Fatal("wrong source", e)
	}
}
func TestPermissionManagementImportAndTransactionalConflict(t *testing.T) {
	s, p := setup(t)
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	now := time.Now().UTC()
	for _, id := range []int{1, 2} {
		if e := persist(t, s, id, now, wireless.Complete, []wireless.Record{{"name": "AP", "ap_id": "1"}}); e != nil {
			t.Fatal(e)
		}
	}
	self := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 1}
	if _, _, e := s.Resources(context.Background(), 2, "access_points", &dto.PageReq{}, self); e == nil {
		t.Fatal("scope bypass")
	}
	var aps []models.Resource
	s.Orm.Order("id").Find(&aps)
	csv := "id,version,localAlias,installationLocation,owner,notes\n1,1,first,,,\n2,1,second,,,\n"
	preview, e := s.PreviewImport(context.Background(), csv, p)
	if e != nil || len(preview.Errors) != 0 {
		t.Fatalf("%+v %v", preview, e)
	}
	if _, e = s.Management(context.Background(), 2, aps[1].ID, dto.ManagementReq{Version: 1, LocalAlias: "changed"}, p); e != nil {
		t.Fatal(e)
	}
	if e = s.CommitImport(context.Background(), dto.ImportReq{Content: csv, Digest: preview.Digest}, p); !errors.Is(e, ErrConflict) {
		t.Fatal("nontransactional import", e)
	}
	s.Orm.First(&aps[0], aps[0].ID)
	if aps[0].LocalAlias != "" {
		t.Fatal("partial mutation")
	}
	denied, e := s.PreviewImport(context.Background(), csv, self)
	if e != nil || len(denied.Errors) == 0 {
		t.Fatal("import scope bypass", e)
	}
	if _, e = s.Management(context.Background(), 1, aps[0].ID, dto.ManagementReq{Version: 2}, p); !errors.Is(e, ErrConflict) {
		t.Fatal("optimistic version bypass", e)
	}
	for _, content := range []string{"id,id\n1,1\n", "id,version,configuration\n1,1,x\n", csv + "1,1,duplicate,,,\n"} {
		r, e := s.PreviewImport(context.Background(), content, p)
		if e == nil && len(r.Errors) == 0 {
			t.Fatal("invalid CSV accepted")
		}
	}
}
func TestCollectionValidationDedupAndTargetedLLDP(t *testing.T) {
	s, p := setup(t)
	ctx := context.Background()
	r := dto.CollectionReq{RequestKey: "test-key-001"}
	job, e := s.CreateCollection(ctx, 1, r, 9, p)
	if e != nil {
		t.Fatal(e)
	}
	for _, d := range job.Dimensions {
		if d == wireless.LLDP {
			t.Fatal("implicit LLDP")
		}
	}
	same, e := s.CreateCollection(ctx, 1, r, 9, p)
	if e != nil || same.ID != job.ID {
		t.Fatal("dedup", e)
	}
	r.Dimensions = []wireless.Dimension{wireless.Clients}
	if _, e = s.CreateCollection(ctx, 1, r, 9, p); !errors.Is(e, ErrConflict) {
		t.Fatal("request key rebound", e)
	}
	if _, e = s.CreateCollection(ctx, 1, dto.CollectionReq{RequestKey: "test-key-002", Dimensions: []wireless.Dimension{wireless.LLDP}}, 9, p); !errors.Is(e, ErrInvalid) {
		t.Fatal("untargeted LLDP", e)
	}
	if _, e = s.CreateCollection(ctx, 1, dto.CollectionReq{RequestKey: "test-key-003"}, 0, p); !errors.Is(e, ErrInvalid) {
		t.Fatal("spoofed actor", e)
	}
	s.Orm.Model(&network.Device{}).Where("id = ?", 2).Updates(map[string]any{"vendor": "Huawei"})
	d, e := s.dimensions("huawei", []wireless.Dimension{wireless.Radios})
	if e != nil || len(d) != 1 {
		t.Fatal("known unsupported dimension must be persisted by collector", d, e)
	}
}

func TestSameSnapshotResolvesAPBeforeChild(t *testing.T) {
	s, _ := setup(t)
	now := time.Now().UTC()
	ap, _ := wireless.NewObservation(wireless.ObservationOptions{Name: wireless.AccessPoints, Status: wireless.Complete, Records: []wireless.Record{{"name": "AP-1", "ap_id": "1"}}, CompletedAt: &now})
	radio, _ := wireless.NewObservation(wireless.ObservationOptions{Name: wireless.Radios, Status: wireless.Complete, Records: []wireless.Record{{"ap_name": "AP-1", "radio_id": "1"}}, CompletedAt: &now})
	snap, _ := wireless.NewSnapshot(wireless.SnapshotOptions{Target: wireless.Record{"controllerId": 1}, ObservedAt: now, Dimensions: map[wireless.Dimension]wireless.Observation{wireless.Radios: radio, wireless.AccessPoints: ap}})
	if err := s.Persist(context.Background(), 1, "source", snap, now.Add(-time.Second), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var row models.Resource
	if err := s.Orm.Where("kind = ?", "radios").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.ParentStatus != "confirmed" {
		t.Fatal(row.ParentStatus)
	}
}

func TestDistinctClientsOnSameRadioDoNotCollide(t *testing.T) {
	s, _ := setup(t)
	now := time.Now().UTC()
	obs, _ := wireless.NewObservation(wireless.ObservationOptions{Name: wireless.Clients, Status: wireless.Complete, Records: []wireless.Record{{"client_mac": "0011.2233.4401", "client_ip": "192.0.2.11", "ap_name": "AP-1", "radio_id": "1", "ssid": "corp"}, {"client_mac": "0011.2233.4402", "client_ip": "192.0.2.12", "ap_name": "AP-1", "radio_id": "1", "ssid": "corp"}}, CompletedAt: &now})
	snap, _ := wireless.NewSnapshot(wireless.SnapshotOptions{Target: wireless.Record{"controllerId": 1}, ObservedAt: now, Dimensions: map[wireless.Dimension]wireless.Observation{wireless.Clients: obs}})
	if err := s.Persist(context.Background(), 1, "source", snap, now.Add(-time.Second), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var count int64
	s.Orm.Model(&models.Resource{}).Where("kind = ?", "clients").Count(&count)
	if count != 2 {
		t.Fatal(count)
	}
}

func TestFirstPartialObservationRetainsCandidateEvidence(t *testing.T) {
	s, _ := setup(t)
	now := time.Now().UTC()
	if err := persist(t, s, 1, now, wireless.Partial, []wireless.Record{{"ap_id": "1", "name": "AP-1"}}); err != nil {
		t.Fatal(err)
	}
	var state models.Dimension
	if err := s.Orm.Where("controller_id = ? AND name = ?", 1, "access_points").First(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state.Latest.Status() != wireless.Partial || len(state.Latest.Records()) != 1 {
		t.Fatal("partial evidence missing")
	}
}

func TestRecoveryQuarantinesExpiredLeaseAndInterruptsRun(t *testing.T) {
	s, _ := setup(t)
	now := time.Now().UTC()
	expired := now.Add(-time.Second)
	job := models.Collection{ControllerID: 1, CreateBy: 1, RequestKey: "recover-job", PayloadHash: "hash", SourceHash: "source", Status: "running", Version: 1, LeaseToken: "owner", LeaseUntil: &expired}
	if err := s.Orm.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	lease := delivery.DeviceLease{DeviceID: 1, Token: "owner", ExpiresAt: expired}
	if err := s.Orm.Create(&lease).Error; err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(s, nil)
	if err := worker.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Orm.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != "interrupted" {
		t.Fatal(job.Status)
	}
	if err := s.Orm.First(&lease, "device_id = ?", 1).Error; err != nil {
		t.Fatal(err)
	}
	if !lease.Quarantined || lease.Token != "owner" {
		t.Fatal(lease)
	}
}

func TestQueuedCollectionOnQuarantinedControllerFailsVisibly(t *testing.T) {
	s, _ := setup(t)
	job := models.Collection{ControllerID: 1, CreateBy: 1, RequestKey: "quarantined-job", PayloadHash: "hash", SourceHash: "source", Status: "queued", Version: 1}
	if err := s.Orm.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	// Another module left this controller quarantined.
	if err := s.Orm.Create(&delivery.DeviceLease{DeviceID: 1, Token: "other-module", ExpiresAt: time.Now().UTC().Add(time.Hour), Quarantined: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewWorker(s, nil).Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Orm.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != "failed" || job.ErrorCode != "device_quarantined" {
		t.Fatalf("queued collection on a quarantined controller: %s %s", job.Status, job.ErrorCode)
	}
}

func TestManagementExportScopesAndEscapesFormulas(t *testing.T) {
	s, p := setup(t)
	now := time.Now().UTC()
	for _, id := range []int{1, 2} {
		if err := persist(t, s, id, now, wireless.Complete, []wireless.Record{{"name": "AP", "ap_id": "1"}}); err != nil {
			t.Fatal(err)
		}
	}
	var row models.Resource
	if err := s.Orm.Where("controller_id = ?", 1).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Management(context.Background(), 1, row.ID, dto.ManagementReq{Version: 1, LocalAlias: "=1+1"}, p); err != nil {
		t.Fatal(err)
	}
	out, err := s.ExportManagement(context.Background(), 1, p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "'=1+1") || strings.Contains(out, "\n2,") {
		t.Fatal(out)
	}
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	self := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 1}
	if _, err = s.ExportManagement(context.Background(), 2, self); !errors.Is(err, ErrScope) {
		t.Fatalf("export scope: %v", err)
	}
}

func TestControllersRespectGroupAndDataScope(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	if err := s.Orm.AutoMigrate(&network.DeviceGroup{}); err != nil {
		t.Fatal(err)
	}
	group := network.DeviceGroup{Code: "region", Name: "Region", Enabled: true}
	if err := s.Orm.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Orm.Model(&network.Device{}).Where("id = ?", 1).Update("group_id", group.Id).Error; err != nil {
		t.Fatal(err)
	}
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	self := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 1}
	rows, count, err := s.Controllers(ctx, &dto.PageReq{GroupID: &group.Id}, self)
	if err != nil || count != 1 || len(rows) != 1 || rows[0].Group != "Region" {
		t.Fatalf("%+v %d %v", rows, count, err)
	}
	zero := 0
	rows, count, err = s.Controllers(ctx, &dto.PageReq{GroupID: &zero}, self)
	if err != nil || count != 0 || len(rows) != 0 {
		t.Fatalf("%+v %d %v", rows, count, err)
	}
}

func TestTargetedLLDPEvidenceRetainsOtherAPAndFailedResults(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := persist(t, s, 1, now, wireless.Complete, []wireless.Record{{"name": "A", "ap_id": "1"}, {"name": "B", "ap_id": "2"}}); err != nil {
		t.Fatal(err)
	}
	observe := func(at time.Time, records, evidence []wireless.Record) error {
		o, err := wireless.NewObservation(wireless.ObservationOptions{Name: wireless.LLDP, Status: wireless.Partial, Records: records, Evidence: evidence, CompletedAt: &at})
		if err != nil {
			return err
		}
		snap, err := wireless.NewSnapshot(wireless.SnapshotOptions{Target: wireless.Record{"controllerId": 1}, ObservedAt: at, Dimensions: map[wireless.Dimension]wireless.Observation{wireless.LLDP: o}})
		if err != nil {
			return err
		}
		return s.Persist(ctx, 1, "source", snap, at.Add(-time.Second), at.Add(time.Second))
	}
	ev := func(name, status string, count int) wireless.Record {
		return wireless.Record{"kind": "ap_lldp_result", "ap_name": name, "status": status, "neighbor_count": count}
	}
	rec := func(name, port string) wireless.Record {
		return wireless.Record{"ap_name": name, "local_interface": port, "chassis_id": "switch", "port_id": port}
	}
	if err := observe(now.Add(time.Second), []wireless.Record{rec("A", "a"), rec("B", "b")}, []wireless.Record{ev("A", "complete", 1), ev("B", "complete", 1)}); err != nil {
		t.Fatal(err)
	}
	if err := observe(now.Add(2*time.Second), nil, []wireless.Record{ev("A", "complete", 0)}); err != nil {
		t.Fatal(err)
	}
	var rows []models.Resource
	if err := s.Orm.Where("kind = ?", "lldp").Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || !rows[0].Missing || rows[1].Missing {
		t.Fatalf("A zero must preserve B: %+v", rows)
	}
	if err := observe(now.Add(3*time.Second), nil, []wireless.Record{ev("B", "failed", 0)}); err != nil {
		t.Fatal(err)
	}
	if err := s.Orm.First(&rows[1], rows[1].ID).Error; err != nil || rows[1].Missing {
		t.Fatal("failed B removed last complete", err)
	}
}

func TestLLDPIdentityIncludesActualRemoteFields(t *testing.T) {
	a := wireless.Record{"ap_name": "A", "local_interface": "GE1", "remote_chassis_id": "0011.2233.4401", "remote_interface": "GE1/0/1"}
	b := wireless.Record{"ap_name": "A", "local_interface": "GE1", "remote_chassis_id": "0011.2233.4402", "remote_interface": "GE1/0/2"}
	if identity("lldp", a) == identity("lldp", b) {
		t.Fatal("distinct neighbors collided")
	}
}
