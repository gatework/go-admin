//go:build integration

package inventory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	base "github.com/gatework/motor-core/sdk/service"
	"github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/models/txlock"
	dto "github.com/gatework/motor/app/net/service/dto/inventory"
)

// An explicit DSN is required. Every run owns a disposable schema and never
// mutates the application's tables, even when sharing a local test database.
func postgresService(t *testing.T) *Device {
	t.Helper()
	dsn := os.Getenv("NETWORK_TEST_DSN")
	if dsn == "" {
		t.Skip("set NETWORK_TEST_DSN to a PostgreSQL test database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	adminConn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { adminConn.Close() })
	schema := fmt.Sprintf("inventory_test_%d", time.Now().UnixNano())
	if err := db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("drop disposable schema: %v", err)
		}
	})
	scoped, err := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := scoped.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := scoped.AutoMigrate(&inventory.Device{}, &inventory.SyncState{}, &inventory.DeviceGroup{}, &inventory.DeviceGroupLock{}, &txlock.Row{}); err != nil {
		t.Fatal(err)
	}
	return &Device{Service: base.Service{DB: scoped}}
}

func TestPostgresLeaseVersionAndTransactionalSnapshot(t *testing.T) {
	s := postgresService(t)
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	s.Source = inventorySource{configured: true, fetch: func(context.Context) ([]inventory.SourceFacts, error) {
		close(entered)
		<-release
		return []inventory.SourceFacts{fact("192.0.2.1", "original")}, nil
	}}
	first := make(chan error, 1)
	go func() { _, err := syncNow(ctx, s, 7, fullScope(), false); first <- err }()
	<-entered
	if _, err := syncNow(ctx, s, 8, fullScope(), false); !errors.Is(err, inventory.RunningErr) {
		t.Errorf("parallel PostgreSQL sync did not conflict: %v", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	var row inventory.Device
	if err := s.DB.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, owner := range []string{"Alice", "Bob"} {
		wg.Add(1)
		go func(owner string) {
			defer wg.Done()
			<-start
			_, err := s.Update(ctx, row.ID, &dto.DeviceUpdateReq{GroupSource: "auto", ClassificationSource: "auto", Version: row.Version, Status: "active", Owner: owner}, 7, fullScope())
			results <- err
		}(owner)
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, inventory.ConflictErr) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("optimistic version winners=%d conflicts=%d", success, conflict)
	}
	manual, err := s.Get(ctx, row.ID, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	s.Source = sourceWithFacts(fact("192.0.2.1", "refreshed"), fact("192.0.2.2", "without serial"))
	result, err := syncNow(ctx, s, 9, fullScope(), false)
	if err != nil || result.Imported != 1 || result.Updated != 1 {
		t.Fatalf("PostgreSQL upsert: %+v %v", result, err)
	}
	after, err := s.Get(ctx, row.ID, fullScope())
	if err != nil || after.Name != "refreshed" || after.Owner != manual.Owner || after.Status != "active" || after.CreateBy != 7 || after.Version != manual.Version+1 {
		t.Fatalf("PostgreSQL merge corrupted data: %+v %v", after, err)
	}
	if err := s.DB.Exec(`ALTER TABLE sys_network_device ADD CONSTRAINT reject_fixture_name CHECK (name <> 'reject')`).Error; err != nil {
		t.Fatal(err)
	}
	facts := []inventory.SourceFacts{fact("192.0.2.1", "must roll back")}
	for i := 1; i <= 220; i++ {
		facts = append(facts, fact(fmt.Sprintf("198.51.100.%d", i), "new"))
	}
	facts[len(facts)-1].Name = "reject"
	s.Source = sourceWithFacts(facts...)
	if _, err := syncNow(ctx, s, 10, fullScope(), false); !errors.Is(err, inventory.PersistenceErr) {
		t.Fatalf("bad batch did not fail: %v", err)
	}
	_, count, err := s.Page(ctx, &dto.DevicePageReq{}, fullScope())
	if err != nil || count != 2 {
		t.Fatalf("PostgreSQL partial commit: %d %v", count, err)
	}
	unchanged, err := s.Get(ctx, row.ID, fullScope())
	if err != nil || unchanged.Name != "refreshed" || unchanged.Version != after.Version {
		t.Fatalf("earlier batch was committed: %+v %v", unchanged, err)
	}
	state, err := s.SyncStatus(ctx)
	if err != nil || state.Status != "failed" || state.DeviceCount != 2 || state.LastSuccessAt == nil {
		t.Fatalf("PostgreSQL failure state: %+v %v", state, err)
	}
}
