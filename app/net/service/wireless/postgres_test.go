package wireless

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	wireless "github.com/gatework/go-wireless"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/delivery"
	network "github.com/gatework/motor/app/net/models/inventory"
	models "github.com/gatework/motor/app/net/models/wireless"
	deliveryService "github.com/gatework/motor/app/net/service/delivery"
	deliveryDTO "github.com/gatework/motor/app/net/service/dto/delivery"
	dto "github.com/gatework/motor/app/net/service/dto/wireless"
)

// Explicitly opt in against a disposable database. Each test owns one isolated
// schema; no application tables or real collection jobs are touched.
func postgresFixture(t *testing.T) (*Wireless, *actions.DataPermission) {
	t.Helper()
	dsn := os.Getenv("WIRELESS_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("WIRELESS_TEST_POSTGRES is not set")
	}
	db, e := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	schema := "wireless_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if e = db.Exec("CREATE SCHEMA " + schema).Error; e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := db.Exec("DROP SCHEMA " + schema + " CASCADE").Error; e != nil {
			t.Error(e)
		}
		sql, _ := db.DB()
		sql.Close()
	})
	scoped, e := gorm.Open(postgres.Open(dsn+" search_path="+schema), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if e != nil {
		t.Fatal(e)
	}
	sql, e := scoped.DB()
	if e != nil {
		t.Fatal(e)
	}
	sql.SetMaxOpenConns(10)
	t.Cleanup(func() { sql.Close() })
	if e = scoped.AutoMigrate(append(Tables(), &network.Device{}, &delivery.Connection{}, &delivery.DeviceLease{}, &delivery.Audit{})...); e != nil {
		t.Fatal(e)
	}
	d := network.Device{IP: "192.0.2.1", Name: "AC", Vendor: "H3C", DeviceType: "wireless_controller", Status: "active", Version: 1}
	d.SetCreateBy(1)
	if e = scoped.Create(&d).Error; e != nil {
		t.Fatal(e)
	}
	return &Wireless{DB: scoped}, &actions.DataPermission{DataScope: actions.DataScopeAll}
}

func TestPostgresPublicationFencesConcurrentSourceEdits(t *testing.T) {
	for _, kind := range []string{"device", "existing_connection", "new_connection"} {
		t.Run(kind, func(t *testing.T) {
			s, p := postgresFixture(t)
			ctx := context.Background()
			version := int64(0)
			if kind == "existing_connection" {
				version = 1
				if e := s.DB.Create(&delivery.Connection{DeviceID: 1, Protocol: "ssh", Port: 22, Version: version}).Error; e != nil {
					t.Fatal(e)
				}
			}
			if _, e := s.CreateCollection(ctx, 1, dto.CollectionReq{RequestKey: "source-fence", Dimensions: []wireless.Dimension{wireless.AccessPoints}}, 1, p); e != nil {
				t.Fatal(e)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			paused := false
			if e := s.DB.Callback().Create().Before("gorm:create").Register("wireless_publish_barrier", func(tx *gorm.DB) {
				if !paused && tx.Statement.Table == (models.Dimension{}).TableName() {
					paused = true
					close(entered)
					<-release
				}
			}); e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { s.DB.Callback().Create().Remove("wireless_publish_barrier") })
			workerDone := make(chan error, 1)
			w := NewWorker(s, collectFunc(func(context.Context, network.Device, delivery.Connection, models.Collection) (wireless.Snapshot, bool, error) {
				return snapshot(t, 1, time.Now().UTC(), wireless.Complete, nil), true, nil
			}))
			go func() { workerDone <- w.Tick(ctx) }()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				close(release)
				t.Fatal("worker did not reach publication")
			}
			editDone := make(chan error, 1)
			go func() {
				if kind == "device" {
					editDone <- s.DB.Model(&network.Device{}).Where("id = ?", 1).Update("version", 2).Error
					return
				}
				workflow := deliveryService.Workflow{Service: s.Service}
				_, e := workflow.SetConnection(ctx, 1, deliveryDTO.Connection{Version: version, Protocol: "ssh", Port: 2222}, 1, p)
				editDone <- e
			}()
			early := false
			select {
			case e := <-editDone:
				early = true
				if e != nil {
					t.Error(e)
				}
				t.Error("source edit committed inside the publication window")
			case <-time.After(150 * time.Millisecond):
			}
			close(release)
			select {
			case e := <-workerDone:
				if e != nil {
					t.Error(e)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not finish")
			}
			if !early {
				select {
				case e := <-editDone:
					if e != nil {
						t.Error(e)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("source edit did not resume")
				}
			}
			var job models.Collection
			if e := s.DB.First(&job).Error; e != nil {
				t.Fatal(e)
			}
			if job.Status != "succeeded" {
				t.Fatalf("publication failed: %s %s", job.Status, job.ErrorCode)
			}
		})
	}
}
