package inventory

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/models/txlock"
	dto "github.com/gatework/motor/app/net/service/dto/inventory"
	"github.com/gatework/motor/internal/testconfig"
)

type inventorySource struct {
	fetch      func(context.Context) ([]inventory.SourceFacts, error)
	configured bool
}

func (p inventorySource) Configured() bool { return p.configured }
func (p inventorySource) Fetch(ctx context.Context) ([]inventory.SourceFacts, error) {
	return p.fetch(ctx)
}
func sourceWithFacts(facts ...inventory.SourceFacts) inventorySource {
	return inventorySource{configured: true, fetch: func(context.Context) ([]inventory.SourceFacts, error) { return facts, nil }}
}
func fact(ip, name string) inventory.SourceFacts {
	return inventory.SourceFacts{IP: ip, Name: name, SourceTags: []string{}}
}
func testService(t *testing.T) *Device {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&inventory.Device{}, &inventory.SyncState{}, &inventory.DeviceGroup{}, &inventory.DeviceGroupLock{}, &txlock.Row{}); err != nil {
		t.Fatal(err)
	}
	return &Device{DB: db}
}
func fullScope() *actions.DataPermission {
	return &actions.DataPermission{DataScope: actions.DataScopeAll}
}

func TestSyncPreservesManualFieldsOwnershipAndAbsentRows(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	s.Source = sourceWithFacts(fact("192.0.2.1", "before"), fact("192.0.2.2", "absent later"))
	result, err := syncNow(ctx, s, 7, fullScope(), false)
	if err != nil || result.Imported != 2 || result.Updated != 0 {
		t.Fatalf("first sync: %+v %v", result, err)
	}
	var original inventory.Device
	s.DB.Where("ip = ?", "192.0.2.1").First(&original)
	group := groupFixture(t, s, "branch", nil)
	req := dto.DeviceUpdateReq{Version: original.Version, GroupID: &group.ID, GroupSource: "manual", ClassificationSource: "manual", Status: "active", DeviceType: "switch", Owner: "Alice", Tags: []string{"urgent"}, Notes: "manual note"}
	if _, err := s.Update(ctx, original.ID, &req, 8, fullScope()); err != nil {
		t.Fatal(err)
	}
	s.Source = sourceWithFacts(fact("192.0.2.1", "after"), fact("192.0.2.3", "new without serial"))
	result, err = syncNow(ctx, s, 9, fullScope(), false)
	if err != nil || result.Imported != 1 || result.Updated != 1 || result.Total != 2 {
		t.Fatalf("second sync: %+v %v", result, err)
	}
	current, err := s.Get(ctx, original.ID, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	if current.Name != "after" || current.Serial != "" || current.Group != "branch" || current.Status != "active" || current.Owner != "Alice" || current.Tags[0] != "urgent" || current.Notes != "manual note" || current.CreateBy != 7 || current.UpdateBy != 8 {
		t.Fatalf("sync changed manual data: %+v", current)
	}
	_, count, err := s.Page(ctx, &dto.DevicePageReq{}, fullScope())
	if err != nil || count != 3 {
		t.Fatalf("missing source rows were deleted: %d %v", count, err)
	}
	state, err := s.SyncStatus(ctx)
	if err != nil || !state.Configured || state.Status != "succeeded" || state.DeviceCount != 2 || state.LastSuccessAt == nil {
		t.Fatalf("wrong sync state: %+v %v", state, err)
	}
}

func TestFailedSyncRollsBackEarlierBatchAndKeepsLastSuccess(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	s.Source = sourceWithFacts(fact("192.0.2.1", "original"))
	if _, err := syncNow(ctx, s, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Exec(`CREATE TRIGGER reject_bad_device BEFORE INSERT ON sys_network_device WHEN NEW.name = 'reject' BEGIN SELECT RAISE(ABORT, 'fixture failure'); END;`).Error; err != nil {
		t.Fatal(err)
	}
	facts := []inventory.SourceFacts{fact("192.0.2.1", "changed")}
	for i := 1; i <= 220; i++ {
		facts = append(facts, fact(fmt.Sprintf("198.51.100.%d", i), "new"))
	}
	facts[len(facts)-1].Name = "reject"
	s.Source = sourceWithFacts(facts...)
	if _, err := syncNow(ctx, s, 9, fullScope(), false); err == nil {
		t.Fatal("expected persistence failure")
	}
	rows, count, err := s.Page(ctx, &dto.DevicePageReq{}, fullScope())
	if err != nil || count != 1 || rows[0].Name != "original" {
		t.Fatalf("partial write escaped rollback: %+v %d %v", rows, count, err)
	}
	state, err := s.SyncStatus(ctx)
	if err != nil || state.Status != "failed" || state.LastSuccessAt == nil || state.DeviceCount != 1 || strings.Contains(state.Message, "fixture") {
		t.Fatalf("unsafe or lost state: %+v %v", state, err)
	}
}

func TestConcurrentSyncConflictsAndExpiredLeaseRecovers(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	s.Source = inventorySource{configured: true, fetch: func(context.Context) ([]inventory.SourceFacts, error) {
		close(entered)
		<-release
		return []inventory.SourceFacts{fact("192.0.2.1", "one")}, nil
	}}
	result := make(chan error, 1)
	go func() { _, err := syncNow(ctx, s, 7, fullScope(), false); result <- err }()
	<-entered
	if _, err := syncNow(ctx, s, 7, fullScope(), false); !errors.Is(err, inventory.RunningErr) {
		t.Errorf("second sync did not conflict: %v", err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	expired := time.Now().UTC().Add(-time.Minute)
	s.DB.Model(&inventory.SyncState{}).Where("id = 1").Updates(map[string]any{"status": "running", "token": "abandoned", "lease_expires_at": expired})
	state, err := s.SyncStatus(ctx)
	if err != nil || state.Status != "failed" {
		t.Fatalf("stale state not recovered: %+v %v", state, err)
	}
	s.Source = sourceWithFacts(fact("192.0.2.2", "two"))
	if _, err := syncNow(ctx, s, 7, fullScope(), false); err != nil {
		t.Fatalf("abandoned lease blocked sync: %v", err)
	}
}

func TestExpiredWriterCannotPublishAfterLeaseIsReplaced(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	s.Source = inventorySource{configured: true, fetch: func(context.Context) ([]inventory.SourceFacts, error) {
		s.DB.Model(&inventory.SyncState{}).Where("id = 1").Updates(map[string]any{"token": "new-owner", "status": "running"})
		return []inventory.SourceFacts{fact("192.0.2.1", "stale")}, nil
	}}
	if _, err := syncNow(ctx, s, 7, fullScope(), false); err == nil {
		t.Fatal("stale writer committed")
	}
	var count int64
	s.DB.Model(&inventory.Device{}).Count(&count)
	if count != 0 {
		t.Fatal("stale writer persisted facts")
	}
	var state inventory.SyncState
	s.DB.First(&state, 1)
	if state.Token != "new-owner" || state.Status != "running" {
		t.Fatalf("stale writer changed new owner: %+v", state)
	}
}

func TestDataPermissionFiltersReadsOptionsUpdatesAndExport(t *testing.T) {

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	s := testService(t)
	ctx := context.Background()
	s.Source = sourceWithFacts(fact("192.0.2.1", "allowed"))
	if _, err := syncNow(ctx, s, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	var first inventory.Device
	s.DB.First(&first)
	s.DB.Model(&first).Updates(map[string]any{"vendor": "Visible", "model": "V1", "group_name": "My group"})
	s.Source = sourceWithFacts(fact("192.0.2.2", "secret"))
	if _, err := syncNow(ctx, s, 8, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	p := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 7}
	rows, count, err := s.Page(ctx, &dto.DevicePageReq{}, p)
	if err != nil || count != 1 || rows[0].Name != "allowed" {
		t.Fatalf("list escaped scope: %+v %d %v", rows, count, err)
	}
	options, err := s.Options(ctx, p)
	if err != nil || strings.Join(options.Vendors, ",") != "Visible" || strings.Join(options.Models, ",") != "V1" {
		t.Fatalf("options escaped scope: %+v %v", options, err)
	}
	var hidden inventory.Device
	s.DB.Where("ip = ?", "192.0.2.2").First(&hidden)
	if _, err := s.Get(ctx, hidden.ID, p); !errors.Is(err, inventory.NotFoundErr) {
		t.Fatalf("detail escaped scope: %v", err)
	}
	if _, err := s.Update(ctx, hidden.ID, &dto.DeviceUpdateReq{GroupSource: "auto", ClassificationSource: "auto", Version: 1, Status: "active"}, 7, p); !errors.Is(err, inventory.NotFoundErr) {
		t.Fatalf("update escaped scope: %v", err)
	}
	data, err := s.Export(ctx, &dto.DevicePageReq{}, p)
	if err != nil || strings.Contains(string(data), "secret") || !strings.Contains(string(data), "allowed") {
		t.Fatalf("export escaped scope: %v", err)
	}
	if _, err := syncNow(ctx, s, 7, p, false); !errors.Is(err, inventory.SyncScopeErr) {
		t.Fatalf("restricted caller synced globally: %v", err)
	}
	if _, err := syncNow(ctx, s, 7, p, true); err != nil {
		t.Fatalf("admin bypass rejected: %v", err)
	}
	_, count, err = s.Page(ctx, &dto.DevicePageReq{}, &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 999})
	if err != nil || count != 0 {
		t.Fatalf("empty scope returned inventory: %d %v", count, err)
	}
}

func TestVersionConflictAndValidationLeaveValuesUnchanged(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	s.Source = sourceWithFacts(fact("192.0.2.1", "device"))
	if _, err := syncNow(ctx, s, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	var row inventory.Device
	s.DB.First(&row)
	req := dto.DeviceUpdateReq{GroupSource: "auto", ClassificationSource: "auto", Version: row.Version, Status: "active", Tags: []string{" a ", "a", ""}, Owner: "First"}
	updated, err := s.Update(ctx, row.ID, &req, 7, fullScope())
	if err != nil || updated.Version != row.Version+1 || len(updated.Tags) != 1 || updated.Tags[0] != "a" {
		t.Fatalf("invalid update: %+v %v", updated, err)
	}
	req.Owner = "Overwrite"
	if _, err := s.Update(ctx, row.ID, &req, 7, fullScope()); !errors.Is(err, inventory.ConflictErr) {
		t.Fatalf("stale update accepted: %v", err)
	}
	req.Version = updated.Version
	req.Status = "online"
	if _, err := s.Update(ctx, row.ID, &req, 7, fullScope()); !errors.Is(err, inventory.InvalidErr) {
		t.Fatalf("unsupported status accepted: %v", err)
	}
	req.Status = "active"
	req.ContactEmail = "not an email"
	if _, err := s.Update(ctx, row.ID, &req, 7, fullScope()); !errors.Is(err, inventory.InvalidErr) {
		t.Fatalf("invalid email accepted: %v", err)
	}
	actual, _ := s.Get(ctx, row.ID, fullScope())
	if actual.Owner != "First" {
		t.Fatal("conflict changed existing data")
	}
}

func TestExportIncludesAllFilteredRowsAndNeutralizesFormulas(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	facts := []inventory.SourceFacts{fact("192.0.2.1", "=HYPERLINK(\"https://invalid\")"), fact("192.0.2.2", " +SUM(1,1)"), fact("192.0.2.3", "safe")}
	for i := range facts {
		facts[i].Vendor = "Vendor"
	}
	s.Source = sourceWithFacts(facts...)
	if _, err := syncNow(ctx, s, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	req := &dto.DevicePageReq{Vendor: "Vendor"}
	req.PageSize = 1
	data, err := s.Export(ctx, req, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff"))).ReadAll()
	if err != nil || len(rows) != 4 {
		t.Fatalf("export was paginated: %d %v", len(rows), err)
	}
	for _, row := range rows[1:] {
		for _, cell := range row {
			trimmed := strings.TrimSpace(cell)
			if strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "+") {
				t.Fatalf("formula not escaped: %q", cell)
			}
		}
	}
	req.Keyword = "missing"
	data, err = s.Export(ctx, req, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	rows, err = csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\ufeff"))).ReadAll()
	if err != nil || len(rows) != 1 {
		t.Fatal("filter ignored")
	}
}

func TestUnconfiguredAndCancelledSyncExposeSafeState(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	s.Source = inventorySource{}
	state, err := s.SyncStatus(ctx)
	if err != nil || state.Configured || state.Status != "idle" {
		t.Fatalf("wrong unconfigured state: %+v %v", state, err)
	}
	if _, err := syncNow(ctx, s, 7, fullScope(), false); !errors.Is(err, inventory.NotConfiguredErr) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(ctx)
	s.Source = inventorySource{configured: true, fetch: func(context.Context) ([]inventory.SourceFacts, error) { cancel(); return nil, inventory.TimeoutErr }}
	if _, err := syncNow(ctx, s, 7, fullScope(), false); !errors.Is(err, inventory.TimeoutErr) {
		t.Fatal(err)
	}
	state, err = s.SyncStatus(context.Background())
	if err != nil || state.Status != "failed" {
		t.Fatalf("cancel left running lease: %+v %v", state, err)
	}
}

func TestFilterUsesLiteralKeywordsAndExactTags(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	s.Source = sourceWithFacts(fact("192.0.2.1", "branch!%_\\1"), fact("192.0.2.2", "branchXX1"))
	if _, err := syncNow(ctx, s, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	var row inventory.Device
	s.DB.Where("ip = ?", "192.0.2.1").First(&row)
	if _, err := s.Update(ctx, row.ID, &dto.DeviceUpdateReq{GroupSource: "auto", ClassificationSource: "auto", Version: row.Version, Status: "active", Tags: []string{"a%_", "rack"}}, 7, fullScope()); err != nil {
		t.Fatal(err)
	}
	for _, query := range []dto.DevicePageReq{{Keyword: "!%_\\"}, {Tag: "a%_"}, {Tag: "rack"}} {
		rows, count, err := s.Page(ctx, &query, fullScope())
		if err != nil || count != 1 || rows[0].IP != "192.0.2.1" {
			t.Fatalf("literal filter mismatch: %+v %d %v", query, count, err)
		}
	}
	for _, query := range []dto.DevicePageReq{{Tag: "rac"}, {Keyword: "' OR 1=1 --"}} {
		_, count, err := s.Page(ctx, &query, fullScope())
		if err != nil || count != 0 {
			t.Fatalf("partial tag or SQL text matched: %+v %d %v", query, count, err)
		}
	}
}

// MySQL interprets backslash inside quoted SQL strings, so ESCAPE followed by
// a single quoted backslash is invalid even though SQLite/PostgreSQL accept it.
func TestMySQLFilterDoesNotEmitInvalidEscapeLiteral(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{DSN: "unused:unused@tcp(127.0.0.1:1)/unused", SkipInitializeWithVersion: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	query := db.Model(&inventory.Device{}).Scopes(filters(&dto.DevicePageReq{Keyword: "!%_\\", Tag: "a%"})).Find(&[]inventory.Device{})
	if query.Error != nil {
		t.Fatal(query.Error)
	}
	if strings.Contains(query.Statement.SQL.String(), "ESCAPE '\\'") {
		t.Fatalf("query contains a MySQL-invalid string literal: %s", query.Statement.SQL.String())
	}
}

// syncNow queues a sync and runs it as the worker would.
func syncNow(ctx context.Context, s *Device, actor int, p *actions.DataPermission, admin bool) (SyncResult, error) {
	if _, err := s.RequestSync(ctx, actor, p, admin); err != nil {
		return SyncResult{}, err
	}
	result, _, err := s.RunQueuedSync(ctx)
	return result, err
}

// A sync request only queues: nothing is fetched until the worker ticks, a
// second request while one is queued is refused, and the rows the worker
// writes belong to the user who asked.
func TestRequestedSyncRunsOnTheWorkerAsTheRequester(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	fetched := 0
	s.Source = inventorySource{configured: true, fetch: func(context.Context) ([]inventory.SourceFacts, error) {
		fetched++
		return []inventory.SourceFacts{fact("192.0.2.1", "core")}, nil
	}}
	w := NewSyncWorker(s)

	if err := w.Tick(ctx); err != nil || fetched != 0 {
		t.Fatalf("idle tick fetched %d times, err %v", fetched, err)
	}
	state, err := s.RequestSync(ctx, 42, fullScope(), false)
	if err != nil || state.Status != "queued" || state.RequestedBy != 42 || fetched != 0 {
		t.Fatalf("request answered %+v %v after %d fetches", state.SyncState, err, fetched)
	}
	if _, err := s.RequestSync(ctx, 43, fullScope(), false); !errors.Is(err, inventory.RunningErr) {
		t.Fatalf("second request while queued: %v", err)
	}
	if err := w.Tick(ctx); err != nil || fetched != 1 {
		t.Fatalf("tick fetched %d times, err %v", fetched, err)
	}
	state, err = s.SyncStatus(ctx)
	if err != nil || state.Status != "succeeded" || state.Imported != 1 || state.Updated != 0 || state.DeviceCount != 1 {
		t.Fatalf("state after the run: %+v %v", state.SyncState, err)
	}
	var row inventory.Device
	if err := s.DB.First(&row, "ip = ?", "192.0.2.1").Error; err != nil || row.CreateBy != 42 || row.UpdateBy != 42 {
		t.Fatalf("row %+v %v; want created and updated by the requester", row, err)
	}
	if err := w.Tick(ctx); err != nil || fetched != 1 {
		t.Fatalf("a finished sync ran again: %d fetches, err %v", fetched, err)
	}
}

// An export matching more than MaxExportRows devices is refused, never
// truncated into a file that looks complete.
func TestDeviceExportRefusesMoreThanTheCap(t *testing.T) {
	s := testService(t)
	defer func(limit int) { maxExportRows = limit }(maxExportRows)
	maxExportRows = 3
	rows := make([]inventory.Device, maxExportRows+1)
	for i := range rows {
		rows[i] = inventory.Device{IP: fmt.Sprintf("10.0.0.%d", i), SourceTags: []string{}, Status: "active", Tags: []string{}, Version: 1}
	}
	if err := s.DB.CreateInBatches(rows, 500).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Export(context.Background(), &dto.DevicePageReq{}, fullScope()); !errors.Is(err, inventory.ExportLimitErr) {
		t.Fatalf("export of %d devices: %v, want ExportLimitErr", len(rows), err)
	}
	if _, err := s.Export(context.Background(), &dto.DevicePageReq{Keyword: "10.0.0.1"}, fullScope()); err != nil {
		t.Fatalf("a filtered export under the cap failed: %v", err)
	}
}

// A device the latest successful sync did not see is kept (sync never
// deletes) and is what the absent filter lists.
func TestAbsentFilterListsDevicesTheLastSyncDidNotSee(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	s.Source = sourceWithFacts(fact("192.0.2.1", "stays"), fact("192.0.2.2", "goes"))
	if _, err := syncNow(ctx, s, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	absent := &dto.DevicePageReq{Absent: true}
	if rows, n, err := s.Page(ctx, absent, fullScope()); err != nil || n != 0 || len(rows) != 0 {
		t.Fatalf("after the first sync: %d absent, err %v", n, err)
	}
	time.Sleep(10 * time.Millisecond)
	s.Source = sourceWithFacts(fact("192.0.2.1", "stays"))
	if _, err := syncNow(ctx, s, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	rows, n, err := s.Page(ctx, &dto.DevicePageReq{Absent: true}, fullScope())
	if err != nil || n != 1 || len(rows) != 1 || rows[0].IP != "192.0.2.2" {
		t.Fatalf("absent devices %+v (%d), err %v; want only 192.0.2.2", rows, n, err)
	}
	if _, n, err := s.Page(ctx, &dto.DevicePageReq{}, fullScope()); err != nil || n != 2 {
		t.Fatalf("all devices: %d, err %v; the absent one must be kept", n, err)
	}
}
