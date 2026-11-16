package wireless

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	wireless "github.com/gatework/go-wireless"
	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/delivery"
	network "github.com/gatework/motor/app/net/models/inventory"
	models "github.com/gatework/motor/app/net/models/wireless"
	dto "github.com/gatework/motor/app/net/service/dto/wireless"
	"github.com/gatework/motor/internal/testconfig"
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
		d := network.Device{IP: []string{"192.0.2.1", "192.0.2.2"}[i-1], Name: "AC", Vendor: "H3C", SourceTags: []string{}, DeviceType: "wireless_controller", Status: "active", Tags: []string{}, Version: 1}
		d.SetCreateBy(i)
		if e = db.Create(&d).Error; e != nil {
			t.Fatal(e)
		}
	}
	return &Wireless{DB: db}, &actions.DataPermission{DataScope: actions.DataScopeAll}
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
	s.DB.First(&row, id)
	if row.Missing || row.LocalAlias != "Human" {
		t.Fatal(row)
	}
	var state models.Dimension
	s.DB.Where("controller_id = ? AND name = ?", 1, "access_points").First(&state)
	if state.Latest.Status() != wireless.Partial || len(state.LastComplete.Records()) != 1 {
		t.Fatal("lost complete evidence")
	}
	if e = persist(t, s, 1, now.Add(2*time.Second), wireless.Complete, nil); e != nil {
		t.Fatal(e)
	}
	s.DB.First(&row, id)
	if !row.Missing || row.LocalAlias != "Human" {
		t.Fatal("human metadata removed")
	}
	if e = persist(t, s, 1, now.Add(3*time.Second), wireless.Complete, rows); e != nil {
		t.Fatal(e)
	}
	s.DB.First(&row, id)
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
	if e := persist(t, s, 1, now, wireless.Complete, []wireless.Record{r, r}); !errors.Is(e, InvalidErr) {
		t.Fatalf("duplicates: %v", e)
	}
	if e := persist(t, s, 1, now, wireless.Complete, []wireless.Record{r}); e != nil {
		t.Fatal(e)
	}
	if e := persist(t, s, 1, now.Add(-time.Second), wireless.Complete, nil); !errors.Is(e, ConflictErr) {
		t.Fatal("late overwrite", e)
	}
	if e := s.Persist(context.Background(), 1, "source", snapshot(t, 2, now, wireless.Complete, nil), now, now); !errors.Is(e, InvalidErr) {
		t.Fatal("wrong source", e)
	}
}
func TestPermissionManagementImportAndTransactionalConflict(t *testing.T) {
	s, p := setup(t)

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	now := time.Now().UTC()
	for _, id := range []int{1, 2} {
		if e := persist(t, s, id, now, wireless.Complete, []wireless.Record{{"name": "AP", "ap_id": "1"}}); e != nil {
			t.Fatal(e)
		}
	}
	self := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 1}
	if _, _, e := s.Resources(context.Background(), 2, "access_points", &dto.PageReq{}, self); e == nil {
		t.Fatal("scope bypass")
	}
	var aps []models.Resource
	s.DB.Order("id").Find(&aps)
	csv := "id,version,localAlias,installationLocation,owner,notes\n1,1,first,,,\n2,1,second,,,\n"
	preview, e := s.PreviewImport(context.Background(), csv, p)
	if e != nil || len(preview.Errors) != 0 {
		t.Fatalf("%+v %v", preview, e)
	}
	if _, e = s.Management(context.Background(), 2, aps[1].ID, dto.ManagementReq{Version: 1, LocalAlias: "changed"}, p); e != nil {
		t.Fatal(e)
	}
	if e = s.CommitImport(context.Background(), dto.ImportReq{Content: csv, Digest: preview.Digest}, p); !errors.Is(e, ConflictErr) {
		t.Fatal("nontransactional import", e)
	}
	s.DB.First(&aps[0], aps[0].ID)
	if aps[0].LocalAlias != "" {
		t.Fatal("partial mutation")
	}
	denied, e := s.PreviewImport(context.Background(), csv, self)
	if e != nil || len(denied.Errors) == 0 {
		t.Fatal("import scope bypass", e)
	}
	if _, e = s.Management(context.Background(), 1, aps[0].ID, dto.ManagementReq{Version: 2}, p); !errors.Is(e, ConflictErr) {
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
	if _, e = s.CreateCollection(ctx, 1, r, 9, p); !errors.Is(e, ConflictErr) {
		t.Fatal("request key rebound", e)
	}
	if _, e = s.CreateCollection(ctx, 1, dto.CollectionReq{RequestKey: "test-key-002", Dimensions: []wireless.Dimension{wireless.LLDP}}, 9, p); !errors.Is(e, InvalidErr) {
		t.Fatal("untargeted LLDP", e)
	}
	if _, e = s.CreateCollection(ctx, 1, dto.CollectionReq{RequestKey: "test-key-003"}, 0, p); !errors.Is(e, InvalidErr) {
		t.Fatal("spoofed actor", e)
	}
	s.DB.Model(&network.Device{}).Where("id = ?", 2).Updates(map[string]any{"vendor": "Huawei"})
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
	if err := s.DB.Where("kind = ?", "radios").First(&row).Error; err != nil {
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
	s.DB.Model(&models.Resource{}).Where("kind = ?", "clients").Count(&count)
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
	if err := s.DB.Where("controller_id = ? AND name = ?", 1, "access_points").First(&state).Error; err != nil {
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
	job := models.Collection{ControllerID: 1, CreateBy: 1, RequestKey: "recover-job", PayloadHash: "hash", SourceHash: "source", Status: "running", Version: 1, ClaimToken: "owner", ClaimUntil: &expired}
	if err := s.DB.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	lease := delivery.DeviceLease{DeviceID: 1, Token: "owner", ExpiresAt: expired}
	if err := s.DB.Create(&lease).Error; err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(s, nil)
	if err := worker.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != "interrupted" {
		t.Fatal(job.Status)
	}
	if err := s.DB.First(&lease, "device_id = ?", 1).Error; err != nil {
		t.Fatal(err)
	}
	if !lease.Quarantined || lease.Token != "owner" {
		t.Fatal(lease)
	}
}

func TestQueuedCollectionOnQuarantinedControllerFailsVisibly(t *testing.T) {
	s, _ := setup(t)
	job := models.Collection{ControllerID: 1, CreateBy: 1, RequestKey: "quarantined-job", PayloadHash: "hash", SourceHash: "source", Status: "queued", Version: 1}
	if err := s.DB.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	// Another module left this controller quarantined.
	if err := s.DB.Create(&delivery.DeviceLease{DeviceID: 1, Token: "other-module", ExpiresAt: time.Now().UTC().Add(time.Hour), Quarantined: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := NewWorker(s, nil).Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.First(&job, job.ID).Error; err != nil {
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
	if err := s.DB.Where("controller_id = ?", 1).First(&row).Error; err != nil {
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

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	self := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 1}
	if _, err = s.ExportManagement(context.Background(), 2, self); !errors.Is(err, ScopeErr) {
		t.Fatalf("export scope: %v", err)
	}
}

func TestControllersRespectGroupAndDataScope(t *testing.T) {
	s, _ := setup(t)
	ctx := context.Background()
	if err := s.DB.AutoMigrate(&network.DeviceGroup{}); err != nil {
		t.Fatal(err)
	}
	group := network.DeviceGroup{Code: "region", Name: "Region", Enabled: true}
	if err := s.DB.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Model(&network.Device{}).Where("id = ?", 1).Update("group_id", group.ID).Error; err != nil {
		t.Fatal(err)
	}

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	self := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 1}
	rows, count, err := s.Controllers(ctx, &dto.PageReq{GroupID: &group.ID}, self)
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
	if err := s.DB.Where("kind = ?", "lldp").Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || !rows[0].Missing || rows[1].Missing {
		t.Fatalf("A zero must preserve B: %+v", rows)
	}
	if err := observe(now.Add(3*time.Second), nil, []wireless.Record{ev("B", "failed", 0)}); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.First(&rows[1], rows[1].ID).Error; err != nil || rows[1].Missing {
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

// The resource filters, sort and paging. Written against the in-memory
// implementation and kept unchanged when filtering moved into SQL, so both
// answer every query alike.
func TestResourceFiltersSortAndPage(t *testing.T) {
	s, p := setup(t)
	ctx := context.Background()
	now := time.Now().UTC()
	rows := []wireless.Record{
		{"ap_id": "01", "name": "Lobby-AP", "mac_address": "0011.2233.4455", "ip_address": "10.1.0.11", "operational_status": "run"},
		{"ap_id": "02", "name": "Lab-AP", "mac_address": "0011.2233.6677", "ip_address": "10.2.0.12", "operational_status": "down"},
		{"ap_id": "03", "name": "Lab-AP-2", "mac_address": "aabb.ccdd.eeff", "ip_address": "10.2.0.13", "operational_status": "run"},
		{"ap_id": "04", "name": "100%_AP", "mac_address": "aabb.ccdd.0000", "ip_address": "10.3.0.14", "operational_status": "run"},
	}
	if e := persist(t, s, 1, now, wireless.Complete, rows); e != nil {
		t.Fatal(e)
	}
	names := func(r dto.PageReq) ([]string, int64) {
		t.Helper()
		got, n, e := s.Resources(ctx, 1, "access_points", &r, p)
		if e != nil {
			t.Fatal(e)
		}
		out := make([]string, len(got))
		for i, row := range got {
			out[i] = text(row.Facts["name"])
		}
		return out, n
	}
	for _, tc := range []struct {
		name  string
		req   dto.PageReq
		want  []string
		count int64
	}{
		{"everything, by id", dto.PageReq{}, []string{"Lobby-AP", "Lab-AP", "Lab-AP-2", "100%_AP"}, 4},
		{"newest first", dto.PageReq{Direction: "desc"}, []string{"100%_AP", "Lab-AP-2", "Lab-AP", "Lobby-AP"}, 4},
		{"second page", dto.PageReq{PageIndex: 2, PageSize: 3}, []string{"100%_AP"}, 4},
		{"ip contains", dto.PageReq{IP: "10.2."}, []string{"Lab-AP", "Lab-AP-2"}, 2},
		{"mac ignores case", dto.PageReq{MAC: "AABB"}, []string{"Lab-AP-2", "100%_AP"}, 2},
		{"ap contains", dto.PageReq{AP: "lab"}, []string{"Lab-AP", "Lab-AP-2"}, 2},
		{"ap exact", dto.PageReq{AP: "Lab-AP", ExactAP: true}, []string{"Lab-AP"}, 1},
		{"status", dto.PageReq{Status: "down"}, []string{"Lab-AP"}, 1},
		{"keyword over facts", dto.PageReq{Keyword: "6677"}, []string{"Lab-AP"}, 1},
		{"wildcards are literal", dto.PageReq{AP: "%_"}, []string{"100%_AP"}, 1},
		{"filters combine", dto.PageReq{IP: "10.2.", Status: "run"}, []string{"Lab-AP-2"}, 1},
		{"no match", dto.PageReq{SSID: "guest"}, []string{}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, n := names(tc.req)
			if n != tc.count || len(got) != len(tc.want) {
				t.Fatalf("got %v (count %d), want %v (count %d)", got, n, tc.want, tc.count)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
	if _, _, e := s.Resources(ctx, 1, "access_points", &dto.PageReq{Sort: "name"}, p); e == nil {
		t.Fatal("an unsupported sort was accepted")
	}
}
