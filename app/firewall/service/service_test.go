package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	base "github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"github.com/google/uuid"
	"go-admin/app/firewall/models"
	"go-admin/app/firewall/types"
	network "go-admin/app/network/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func testService(t *testing.T) *Firewall {
	t.Helper()
	var dialect gorm.Dialector = sqlite.Open(filepath.Join(t.TempDir(), "fw.db"))
	if dsn := os.Getenv("FIREWALL_TEST_POSTGRES_DSN"); dsn != "" {
		dialect = postgres.Open(dsn)
	}
	db, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sql.Close() })
	if db.Dialector.Name() == "postgres" {
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
	if err = db.AutoMigrate(&network.Device{}, &network.DeviceGroup{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&models.Domain{Code: "default", Name: "默认网络域", Version: 1}).Error; err != nil {
		t.Fatal(err)
	}
	return &Firewall{Service: base.Service{Orm: db}, Actor: 7, Root: t.TempDir()}
}
func all() *actions.DataPermission {
	return &actions.DataPermission{DataScope: actions.DataScopeAll, UserId: 7}
}

func TestTenantSnapshotRootSurvivesRequestBoundORM(t *testing.T) {
	s := testService(t)
	t.Setenv("FIREWALL_SNAPSHOT_DIR", t.TempDir())
	want := RegisterRoot(s.Orm, "tenant-request-clone")
	s.Root = ""
	s.Orm = s.Orm.WithContext(context.Background())
	got, err := s.root()
	if err != nil || got != want {
		t.Fatalf("request ORM lost tenant snapshot store: %q want=%q err=%v", got, want, err)
	}
}
func device(t *testing.T, s *Firewall, owner int, ip string) int {
	t.Helper()
	d := network.Device{SourceFacts: network.SourceFacts{Name: "synthetic", IP: ip, Vendor: "hillstone", SourceTags: []string{}}, DeviceType: "firewall", Status: "active", Tags: []string{}}
	d.CreateBy = owner
	if e := s.Orm.Create(&d).Error; e != nil {
		t.Fatal(e)
	}
	return d.Id
}
func TestInstanceAccessIsAnchoredToInventory(t *testing.T) {
	previous := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = previous })
	s := testService(t)
	ctx := context.Background()
	own, hidden := device(t, s, 7, "192.0.2.1"), device(t, s, 8, "192.0.2.2")
	p := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 7}
	if _, e := s.CreateInstance(ctx, InstanceInput{DeviceID: hidden, DomainID: 1, Vendor: "hillstone", Mode: "routed"}, p); !errors.Is(e, ErrNotFound) {
		t.Fatalf("hidden inventory allowed: %v", e)
	}
	i, e := s.CreateInstance(ctx, InstanceInput{DeviceID: own, DomainID: 1, Vendor: "hillstone", Mode: "routed"}, p)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Instance(ctx, i.ID, &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 8}); !errors.Is(e, ErrNotFound) {
		t.Fatalf("instance escaped inventory: %v", e)
	}
}
func TestRequestVersionAndOwnershipProtectOriginalInput(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	in := RequestInput{Title: "访问需求", DomainID: 1, Input: types.Input{DefaultProtocol: "tcp", Rows: []types.InputRow{{Key: "one", Source: "192.0.2.1", Destination: "198.51.100.2", Service: "443"}}}}
	r, e := s.SaveRequest(ctx, 0, in, all())
	if e != nil {
		t.Fatal(e)
	}
	in.Version = r.Version
	in.Title = "新标题"
	if _, e = s.SaveRequest(ctx, r.ID, in, all()); e != nil {
		t.Fatal(e)
	}
	if _, e = s.SaveRequest(ctx, r.ID, in, all()); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale edit accepted: %v", e)
	}
	s.Actor = 8
	if _, e = s.Request(ctx, r.ID, all()); !errors.Is(e, ErrNotFound) {
		t.Fatalf("other owner read raw input: %v", e)
	}
}

func TestNewProtectedInventoryInvalidatesPreviouslyPinnedAnalysis(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	r, e := s.SaveRequest(ctx, 0, RequestInput{Title: "scope", DomainID: 1, Input: types.Input{Rows: []types.InputRow{{Source: "192.0.2.1", Destination: "198.51.100.1", Service: "443"}}}}, all())
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.EnqueueAnalysis(ctx, r.ID, JobInput{RequestKey: "before", Version: r.Version}, all())
	if e != nil {
		t.Fatal(e)
	}
	d := device(t, s, 7, "203.0.113.1")
	if _, e = s.CreateInstance(ctx, InstanceInput{DeviceID: d, DomainID: 1, Vendor: "hillstone", Mode: "routed"}, all()); e != nil {
		t.Fatal(e)
	}
	a, e = s.Analysis(ctx, a.ID, all())
	if e != nil || !a.Stale {
		t.Fatalf("new device silently excluded from old analysis: %+v %v", a, e)
	}
}

func TestPermissionRevocationHidesSavedCrossDeviceInputAndLists(t *testing.T) {
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	s := testService(t)
	ctx := context.Background()
	p := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 7}
	d := device(t, s, 7, "203.0.113.7")
	if _, e := s.CreateInstance(ctx, InstanceInput{DeviceID: d, DomainID: 1, Vendor: "hillstone", Mode: "routed"}, p); e != nil {
		t.Fatal(e)
	}
	r, e := s.SaveRequest(ctx, 0, RequestInput{Title: "private query", DomainID: 1, Input: types.Input{Rows: []types.InputRow{{Source: "192.0.2.1", Destination: "198.51.100.1", Service: "443"}}}}, p)
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.EnqueueAnalysis(ctx, r.ID, JobInput{RequestKey: "private", Version: r.Version}, p)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Orm.Model(&network.Device{}).Where("id = ?", d).Update("create_by", 8).Error; e != nil {
		t.Fatal(e)
	}
	if _, e = s.Analysis(ctx, a.ID, p); !errors.Is(e, ErrNotFound) {
		t.Fatalf("revoked analysis visible: %v", e)
	}
	if _, e = s.Request(ctx, r.ID, p); !errors.Is(e, ErrNotFound) {
		t.Fatalf("raw input visible: %v", e)
	}
	_, count, e := s.Requests(ctx, &Page{}, p)
	if e != nil || count != 0 {
		t.Fatalf("list leaked hidden record: count=%d err=%v", count, e)
	}
}
