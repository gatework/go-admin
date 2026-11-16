package firewall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/contract/actions"
	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/firewall/types"
	network "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/models/txlock"
	"github.com/gatework/motor/app/net/service/firewall/dto"
	"github.com/gatework/motor/internal/testconfig"
)

func testService(t *testing.T) *Firewall {
	t.Helper()
	var dialect = sqlite.Open(filepath.Join(t.TempDir(), "fw.db"))
	if dsn := os.Getenv("FIREWALL_TEST_POSTGRES_DSN"); dsn != "" {
		dialect = postgres.Open(dsn)
	}
	db, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), TranslateError: true})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sql.Close() })
	if db.Name() == "postgres" {
		// Each test owns a random schema; never migrate or truncate caller tables.
		schema := "firewall_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if err = db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if e := db.Exec("DROP SCHEMA " + schema + " CASCADE").Error; e != nil {
				t.Error(e)
			}
		})
		if err = db.Exec("SET search_path TO " + schema).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = models.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&network.Device{}, &network.DeviceGroup{}, &txlock.Row{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&models.Domain{Code: "default", Name: "默认网络域", Version: 1}).Error; err != nil {
		t.Fatal(err)
	}
	return &Firewall{DB: db, Actor: 7, Root: t.TempDir()}
}
func all() *actions.DataPermission {
	return &actions.DataPermission{DataScope: actions.DataScopeAll, UserID: 7}
}

func TestTenantSnapshotRootSurvivesRequestBoundORM(t *testing.T) {
	s := testService(t)
	t.Setenv("FIREWALL_SNAPSHOT_DIR", t.TempDir())
	want := RegisterRoot(s.DB, "tenant-request-clone")
	s.Root = ""
	s.DB = s.DB.WithContext(context.Background())
	got, err := s.root()
	if err != nil || got != want {
		t.Fatalf("request ORM lost tenant snapshot store: %q want=%q err=%v", got, want, err)
	}
}
func device(t *testing.T, s *Firewall, owner int, ip string) int {
	t.Helper()
	d := network.Device{CreateBy: owner, Name: "synthetic", IP: ip, Vendor: "hillstone", SourceTags: []string{}, DeviceType: "firewall", Status: "active", Tags: []string{}}
	if e := s.DB.Create(&d).Error; e != nil {
		t.Fatal(e)
	}
	return d.ID
}
func TestInstanceAccessIsAnchoredToInventory(t *testing.T) {

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	s := testService(t)
	ctx := context.Background()
	own, hidden := device(t, s, 7, "192.0.2.1"), device(t, s, 8, "192.0.2.2")
	p := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 7}
	if _, e := s.CreateInstance(ctx, dto.InstanceInput{DeviceID: hidden, DomainID: 1, Vendor: "hillstone", Mode: "routed"}, p); !errors.Is(e, NotFoundErr) {
		t.Fatalf("hidden inventory allowed: %v", e)
	}
	i, e := s.CreateInstance(ctx, dto.InstanceInput{DeviceID: own, DomainID: 1, Vendor: "hillstone", Mode: "routed"}, p)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Instance(ctx, i.ID, &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 8}); !errors.Is(e, NotFoundErr) {
		t.Fatalf("instance escaped inventory: %v", e)
	}
}
func TestRequestVersionAndOwnershipProtectOriginalInput(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	in := dto.RequestInput{Title: "访问需求", DomainID: 1, Input: types.Input{DefaultProtocol: "tcp", Rows: []types.InputRow{{Key: "one", Source: "192.0.2.1", Destination: "198.51.100.2", Service: "443"}}}}
	r, e := s.SaveRequest(ctx, 0, in, all())
	if e != nil {
		t.Fatal(e)
	}
	in.Version = r.Version
	in.Title = "新标题"
	if _, e = s.SaveRequest(ctx, r.ID, in, all()); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveRequest(ctx, r.ID, in, all()); !errors.Is(e, ConflictErr) {
		t.Fatalf("stale edit accepted: %v", e)
	}
	s.Actor = 8
	if _, e = s.Request(ctx, r.ID, all()); !errors.Is(e, NotFoundErr) {
		t.Fatalf("other owner read raw input: %v", e)
	}
}

func TestNewProtectedInventoryInvalidatesPreviouslyPinnedAnalysis(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	r, e := s.SaveRequest(ctx, 0, dto.RequestInput{Title: "scope", DomainID: 1, Input: types.Input{Rows: []types.InputRow{{Source: "192.0.2.1", Destination: "198.51.100.1", Service: "443"}}}}, all())
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.EnqueueAnalysis(ctx, r.ID, dto.JobInput{RequestKey: "before", Version: r.Version}, all())
	if e != nil {
		t.Fatal(e)
	}
	d := device(t, s, 7, "203.0.113.1")
	if _, e = s.CreateInstance(ctx, dto.InstanceInput{DeviceID: d, DomainID: 1, Vendor: "hillstone", Mode: "routed"}, all()); e != nil {
		t.Fatal(e)
	}
	a, e = s.Analysis(ctx, a.ID, all())
	if e != nil || !a.Stale {
		t.Fatalf("new device silently excluded from old analysis: %+v %v", a, e)
	}
}

func TestPermissionRevocationHidesSavedCrossDeviceInputAndLists(t *testing.T) {

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	s := testService(t)
	ctx := context.Background()
	p := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 7}
	d := device(t, s, 7, "203.0.113.7")
	if _, e := s.CreateInstance(ctx, dto.InstanceInput{DeviceID: d, DomainID: 1, Vendor: "hillstone", Mode: "routed"}, p); e != nil {
		t.Fatal(e)
	}
	r, e := s.SaveRequest(ctx, 0, dto.RequestInput{Title: "private query", DomainID: 1, Input: types.Input{Rows: []types.InputRow{{Source: "192.0.2.1", Destination: "198.51.100.1", Service: "443"}}}}, p)
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.EnqueueAnalysis(ctx, r.ID, dto.JobInput{RequestKey: "private", Version: r.Version}, p)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.DB.Model(&network.Device{}).Where("id = ?", d).Update("create_by", 8).Error; e != nil {
		t.Fatal(e)
	}
	if _, e = s.Analysis(ctx, a.ID, p); !errors.Is(e, NotFoundErr) {
		t.Fatalf("revoked analysis visible: %v", e)
	}
	if _, e = s.Request(ctx, r.ID, p); !errors.Is(e, NotFoundErr) {
		t.Fatalf("raw input visible: %v", e)
	}
	_, count, e := s.Requests(ctx, &dto.Page{}, p)
	if e != nil || count != 0 {
		t.Fatalf("list leaked hidden record: count=%d err=%v", count, e)
	}
}

// Only a broken unique key is a conflict; any other failure to save is
// reported as itself, not as "someone changed this first".
func TestSaveDomainCallsOnlyADuplicateAConflict(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if _, err := s.SaveDomain(ctx, 0, dto.DomainInput{Code: "dc-1", Name: "DC 1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveDomain(ctx, 0, dto.DomainInput{Code: "dc-1", Name: "again"}); !errors.Is(err, ConflictErr) {
		t.Fatalf("duplicate code: %v, want ConflictErr", err)
	}
	if err := s.DB.Exec("DROP TABLE " + (models.Domain{}).TableName()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveDomain(ctx, 0, dto.DomainInput{Code: "dc-2", Name: "DC 2"}); err == nil || errors.Is(err, ConflictErr) {
		t.Fatalf("missing table: %v, want the database error", err)
	}
}
