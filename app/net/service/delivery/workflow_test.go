package delivery

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/contract/actions"
	models "github.com/gatework/motor/app/net/models/delivery"
	"github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/delivery"
	"github.com/gatework/motor/internal/testconfig"
)

func fixture(t *testing.T) *Workflow {
	t.Helper()
	t.Setenv("NC_H3C_USERNAME", "test")
	t.Setenv("NC_H3C_PASSWORD", "test")
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, _ := db.DB()
	sql.SetMaxOpenConns(1)
	t.Cleanup(func() { sql.Close() })
	if e = db.AutoMigrate(append(Tables(), &inventory.Device{})...); e != nil {
		t.Fatal(e)
	}
	d := inventory.Device{IP: "192.0.2.1", Name: "fixture", SourceTags: []string{}, Status: "active", Tags: []string{},
		CreateBy: 7}
	if e = db.Create(&d).Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Create(&models.Connection{DeviceID: d.ID, Version: 1, Profile: "h3c", Protocol: "ssh", Port: 22}).Error; e != nil {
		t.Fatal(e)
	}
	return &Workflow{DB: db, Config: Config{Enabled: true, Shared: Credentials{Username: "test", Password: "test"}, Grace: time.Minute, Concurrency: 5}}
}
func scope() *actions.DataPermission { return &actions.DataPermission{DataScope: actions.DataScopeAll} }
func request() dto.Create {
	return dto.Create{RequestKey: "request-unique-0001", Title: "Test", Targets: []dto.InputTarget{{DeviceID: 1, Script: "system-view\nsysname fixture"}}}
}
func TestWholeOrderValidationAndIdempotency(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	r := request()
	r.Targets = append(r.Targets, dto.InputTarget{DeviceID: 1, Script: "bad"})
	if _, e := s.Create(ctx, r, 7, "tester", scope()); e == nil {
		t.Fatal("duplicate target accepted")
	}
	r = request()
	r.Targets[0].Script = strings.Repeat("x", 65537)
	if _, e := s.Create(ctx, r, 7, "tester", scope()); e == nil {
		t.Fatal("oversize accepted")
	}
	a, e := s.Create(ctx, request(), 7, "tester", scope())
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.Create(ctx, request(), 7, "tester", scope())
	if e != nil || a.ID != b.ID {
		t.Fatalf("idempotency: %v", e)
	}
	r = request()
	r.Title = "different"
	if _, e = s.Create(ctx, r, 7, "tester", scope()); !errors.Is(e, ConflictErr) {
		t.Fatalf("payload mismatch %v", e)
	}
	var count int64
	s.DB.Model(&models.Order{}).Count(&count)
	if count != 1 {
		t.Fatal(count)
	}
}
func TestEditVersionAndCancelTransitions(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	o, e := s.Create(ctx, request(), 7, "tester", scope())
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.Edit(ctx, o.ID, dto.Edit{Version: 99, Title: "bad", Targets: request().Targets}, 7, scope())
	if !errors.Is(e, ConflictErr) {
		t.Fatal(e)
	}
	o, e = s.Action(ctx, o.ID, o.Version, "cancel", 7, scope())
	if e != nil || o.Status != "cancelled" || o.Targets[0].Status != "cancelled" {
		t.Fatalf("cancel %+v %v", o, e)
	}
	if _, e = s.Action(ctx, o.ID, o.Version, "execute", 7, scope()); !errors.Is(e, ConflictErr) {
		t.Fatal(e)
	}
}

func TestFullScriptBoundaryFitsDatabase(t *testing.T) {
	s := fixture(t)
	r := request()
	r.Targets[0].Script = strings.Repeat("x", MaxScriptBytes)
	o, e := s.Create(context.Background(), r, 7, "tester", scope())
	if e != nil || len(o.Targets[0].Script) != MaxScriptBytes {
		t.Fatalf("64 KiB rejected %v", e)
	}
}

func TestExplicitTelnetConnectionMetadata(t *testing.T) {
	s := fixture(t)
	c, e := s.SetConnection(context.Background(), 1, dto.Connection{Version: 1, Profile: "h3c", Protocol: "telnet", Port: 23}, 7, scope())
	if e != nil || c.Protocol != "telnet" || c.Port != 23 {
		t.Fatalf("explicit telnet %+v %v", c, e)
	}
}

func TestDataScopeCoversOrdersDevicesAndConnection(t *testing.T) {
	s := fixture(t)

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	ctx := context.Background()
	owner := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 7}
	other := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 8}
	o, e := s.Create(ctx, request(), 7, "tester", owner)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Get(ctx, o.ID, other); !errors.Is(e, NotFoundErr) {
		t.Fatal("out-of-scope order", e)
	}
	if _, e = s.Target(ctx, o.ID, o.Targets[0].ID, other); !errors.Is(e, NotFoundErr) {
		t.Fatal("out-of-scope target", e)
	}
	if _, e = s.Connection(ctx, 1, other); !errors.Is(e, NotFoundErr) {
		t.Fatal("out-of-scope connection", e)
	}
	r := request()
	r.RequestKey = "other-scope-001"
	if _, e = s.Create(ctx, r, 8, "other", other); !errors.Is(e, NotFoundErr) {
		t.Fatal("out-of-scope device", e)
	}
	rows, count, e := s.Page(ctx, dto.Page{PageIndex: 1, PageSize: 20}, other)
	if e != nil || count != 0 || len(rows) != 0 {
		t.Fatal("out-of-scope list", e)
	}
	if _, e = s.Get(ctx, o.ID, nil); !errors.Is(e, NotFoundErr) {
		t.Fatal("nil scope not closed", e)
	}
}

func TestInactiveInventoryIsNotPresentedAsReady(t *testing.T) {
	s := fixture(t)
	ctx := context.Background()
	if e := s.DB.Exec("UPDATE sys_network_device SET status = ? WHERE id = 1", "pending").Error; e != nil {
		t.Fatal(e)
	}
	c, e := s.Connection(ctx, 1, scope())
	if e != nil || c.Ready || c.Reason != "device_unavailable" {
		t.Fatalf("inactive offered as ready %+v %v", c, e)
	}
}
