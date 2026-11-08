package service

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	base "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"go-admin/app/network"
	"go-admin/app/network/models"
	"go-admin/app/network/service/dto"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type inventorySource struct {
	fetch      func(context.Context) ([]models.SourceFacts, error)
	configured bool
}

func (p inventorySource) Configured() bool { return p.configured }
func (p inventorySource) Fetch(ctx context.Context) ([]models.SourceFacts, error) {
	return p.fetch(ctx)
}
func inventory(facts ...models.SourceFacts) inventorySource {
	return inventorySource{configured: true, fetch: func(context.Context) ([]models.SourceFacts, error) { return facts, nil }}
}
func fact(ip, name string) models.SourceFacts {
	return models.SourceFacts{IP: ip, Name: name, SourceTags: []string{}}
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
	if err := db.AutoMigrate(&models.Device{}, &models.SyncState{}, &models.DeviceGroup{}, &models.DeviceGroupLock{}); err != nil {
		t.Fatal(err)
	}
	return &Device{Service: base.Service{Orm: db}}
}
func fullScope() *actions.DataPermission {
	return &actions.DataPermission{DataScope: actions.DataScopeAll}
}

func TestSyncPreservesManualFieldsOwnershipAndAbsentRows(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	s.Source = inventory(fact("192.0.2.1", "before"), fact("192.0.2.2", "absent later"))
	result, err := s.Sync(ctx, 7, fullScope(), false)
	if err != nil || result.Imported != 2 || result.Updated != 0 {
		t.Fatalf("first sync: %+v %v", result, err)
	}
	var original models.Device
	s.Orm.Where("ip = ?", "192.0.2.1").First(&original)
	group := groupFixture(t, s, "branch", nil)
	req := dto.DeviceUpdateReq{Version: original.Version, GroupID: &group.Id, GroupSource: "manual", ClassificationSource: "manual", Status: "active", DeviceType: "switch", Owner: "Alice", Tags: []string{"urgent"}, Notes: "manual note"}
	if _, err := s.Update(ctx, original.Id, &req, 8, fullScope()); err != nil {
		t.Fatal(err)
	}
	s.Source = inventory(fact("192.0.2.1", "after"), fact("192.0.2.3", "new without serial"))
	result, err = s.Sync(ctx, 9, fullScope(), false)
	if err != nil || result.Imported != 1 || result.Updated != 1 || result.Total != 2 {
		t.Fatalf("second sync: %+v %v", result, err)
	}
	current, err := s.Get(ctx, original.Id, fullScope())
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
	s.Source = inventory(fact("192.0.2.1", "original"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	if err := s.Orm.Exec(`CREATE TRIGGER reject_bad_device BEFORE INSERT ON sys_network_device WHEN NEW.name = 'reject' BEGIN SELECT RAISE(ABORT, 'fixture failure'); END;`).Error; err != nil {
		t.Fatal(err)
	}
	facts := []models.SourceFacts{fact("192.0.2.1", "changed")}
	for i := 1; i <= 220; i++ {
		facts = append(facts, fact(fmt.Sprintf("198.51.100.%d", i), "new"))
	}
	facts[len(facts)-1].Name = "reject"
	s.Source = inventory(facts...)
	if _, err := s.Sync(ctx, 9, fullScope(), false); err == nil {
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
	s.Source = inventorySource{configured: true, fetch: func(context.Context) ([]models.SourceFacts, error) {
		close(entered)
		<-release
		return []models.SourceFacts{fact("192.0.2.1", "one")}, nil
	}}
	result := make(chan error, 1)
	go func() { _, err := s.Sync(ctx, 7, fullScope(), false); result <- err }()
	<-entered
	if _, err := s.Sync(ctx, 7, fullScope(), false); !errors.Is(err, network.ErrRunning) {
		t.Errorf("second sync did not conflict: %v", err)
	}
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	expired := time.Now().UTC().Add(-time.Minute)
	s.Orm.Model(&models.SyncState{}).Where("id = 1").Updates(map[string]any{"status": "running", "token": "abandoned", "lease_expires_at": expired})
	state, err := s.SyncStatus(ctx)
	if err != nil || state.Status != "failed" {
		t.Fatalf("stale state not recovered: %+v %v", state, err)
	}
	s.Source = inventory(fact("192.0.2.2", "two"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatalf("abandoned lease blocked sync: %v", err)
	}
}

func TestExpiredWriterCannotPublishAfterLeaseIsReplaced(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	s.Source = inventorySource{configured: true, fetch: func(context.Context) ([]models.SourceFacts, error) {
		s.Orm.Model(&models.SyncState{}).Where("id = 1").Updates(map[string]any{"token": "new-owner", "status": "running"})
		return []models.SourceFacts{fact("192.0.2.1", "stale")}, nil
	}}
	if _, err := s.Sync(ctx, 7, fullScope(), false); err == nil {
		t.Fatal("stale writer committed")
	}
	var count int64
	s.Orm.Model(&models.Device{}).Count(&count)
	if count != 0 {
		t.Fatal("stale writer persisted facts")
	}
	var state models.SyncState
	s.Orm.First(&state, 1)
	if state.Token != "new-owner" || state.Status != "running" {
		t.Fatalf("stale writer changed new owner: %+v", state)
	}
}

func TestDataPermissionFiltersReadsOptionsUpdatesAndExport(t *testing.T) {
	previous := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = previous })
	s := testService(t)
	ctx := context.Background()
	s.Source = inventory(fact("192.0.2.1", "allowed"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	var first models.Device
	s.Orm.First(&first)
	s.Orm.Model(&first).Updates(map[string]any{"vendor": "Visible", "model": "V1", "group_name": "My group"})
	s.Source = inventory(fact("192.0.2.2", "secret"))
	if _, err := s.Sync(ctx, 8, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	p := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 7}
	rows, count, err := s.Page(ctx, &dto.DevicePageReq{}, p)
	if err != nil || count != 1 || rows[0].Name != "allowed" {
		t.Fatalf("list escaped scope: %+v %d %v", rows, count, err)
	}
	options, err := s.Options(ctx, p)
	if err != nil || strings.Join(options.Vendors, ",") != "Visible" || strings.Join(options.Models, ",") != "V1" {
		t.Fatalf("options escaped scope: %+v %v", options, err)
	}
	var hidden models.Device
	s.Orm.Where("ip = ?", "192.0.2.2").First(&hidden)
	if _, err := s.Get(ctx, hidden.Id, p); !errors.Is(err, network.ErrNotFound) {
		t.Fatalf("detail escaped scope: %v", err)
	}
	if _, err := s.Update(ctx, hidden.Id, &dto.DeviceUpdateReq{GroupSource: "auto", ClassificationSource: "auto", Version: 1, Status: "active"}, 7, p); !errors.Is(err, network.ErrNotFound) {
		t.Fatalf("update escaped scope: %v", err)
	}
	data, err := s.Export(ctx, &dto.DevicePageReq{}, p)
	if err != nil || strings.Contains(string(data), "secret") || !strings.Contains(string(data), "allowed") {
		t.Fatalf("export escaped scope: %v", err)
	}
	if _, err := s.Sync(ctx, 7, p, false); !errors.Is(err, network.ErrSyncScope) {
		t.Fatalf("restricted caller synced globally: %v", err)
	}
	if _, err := s.Sync(ctx, 7, p, true); err != nil {
		t.Fatalf("admin bypass rejected: %v", err)
	}
	_, count, err = s.Page(ctx, &dto.DevicePageReq{}, &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 999})
	if err != nil || count != 0 {
		t.Fatalf("empty scope returned inventory: %d %v", count, err)
	}
}

func TestVersionConflictAndValidationLeaveValuesUnchanged(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	s.Source = inventory(fact("192.0.2.1", "device"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	var row models.Device
	s.Orm.First(&row)
	req := dto.DeviceUpdateReq{GroupSource: "auto", ClassificationSource: "auto", Version: row.Version, Status: "active", Tags: []string{" a ", "a", ""}, Owner: "First"}
	updated, err := s.Update(ctx, row.Id, &req, 7, fullScope())
	if err != nil || updated.Version != row.Version+1 || len(updated.Tags) != 1 || updated.Tags[0] != "a" {
		t.Fatalf("invalid update: %+v %v", updated, err)
	}
	req.Owner = "Overwrite"
	if _, err := s.Update(ctx, row.Id, &req, 7, fullScope()); !errors.Is(err, network.ErrConflict) {
		t.Fatalf("stale update accepted: %v", err)
	}
	req.Version = updated.Version
	req.Status = "online"
	if _, err := s.Update(ctx, row.Id, &req, 7, fullScope()); !errors.Is(err, network.ErrInvalidInput) {
		t.Fatalf("unsupported status accepted: %v", err)
	}
	req.Status = "active"
	req.ContactEmail = "not an email"
	if _, err := s.Update(ctx, row.Id, &req, 7, fullScope()); !errors.Is(err, network.ErrInvalidInput) {
		t.Fatalf("invalid email accepted: %v", err)
	}
	actual, _ := s.Get(ctx, row.Id, fullScope())
	if actual.Owner != "First" {
		t.Fatal("conflict changed existing data")
	}
}

func TestExportIncludesAllFilteredRowsAndNeutralizesFormulas(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	facts := []models.SourceFacts{fact("192.0.2.1", "=HYPERLINK(\"https://invalid\")"), fact("192.0.2.2", " +SUM(1,1)"), fact("192.0.2.3", "safe")}
	for i := range facts {
		facts[i].Vendor = "Vendor"
	}
	s.Source = inventory(facts...)
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
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
	if _, err := s.Sync(ctx, 7, fullScope(), false); !errors.Is(err, network.ErrNotConfigured) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(ctx)
	s.Source = inventorySource{configured: true, fetch: func(context.Context) ([]models.SourceFacts, error) { cancel(); return nil, network.ErrTimeout }}
	if _, err := s.Sync(ctx, 7, fullScope(), false); !errors.Is(err, network.ErrTimeout) {
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
	s.Source = inventory(fact("192.0.2.1", "branch!%_\\1"), fact("192.0.2.2", "branchXX1"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	var row models.Device
	s.Orm.Where("ip = ?", "192.0.2.1").First(&row)
	if _, err := s.Update(ctx, row.Id, &dto.DeviceUpdateReq{GroupSource: "auto", ClassificationSource: "auto", Version: row.Version, Status: "active", Tags: []string{"a%_", "rack"}}, 7, fullScope()); err != nil {
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
	query := db.Model(&models.Device{}).Scopes(filters(&dto.DevicePageReq{Keyword: "!%_\\", Tag: "a%"})).Find(&[]models.Device{})
	if query.Error != nil {
		t.Fatal(query.Error)
	}
	if strings.Contains(query.Statement.SQL.String(), "ESCAPE '\\'") {
		t.Fatalf("query contains a MySQL-invalid string literal: %s", query.Statement.SQL.String())
	}
}
