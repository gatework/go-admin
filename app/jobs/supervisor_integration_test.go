package jobs

import (
	"os"
	"testing"
	"time"

	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg/cronjob"
	"go-admin/app/jobs/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPostgresBlockedRenewalDeadline(t *testing.T) {
	dsn := os.Getenv(postgresDSNEnv)
	if dsn == "" {
		t.Skip("GO_ADMIN_TEST_POSTGRES_DSN is required")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	seedLeaseTable(t, db)
	if err := db.AutoMigrate(&models.SysJob{}); err != nil {
		t.Fatal(err)
	}
	const key = "pg-blocked-renewal"
	sdk.Runtime.SetCrontabByTenant(key, cronjob.NewWithSeconds())
	s := newSupervisor(key, db, "pg-first")
	s.lease.ttl = 300 * time.Millisecond
	s.tick()
	t.Cleanup(s.stopScheduling)
	if !s.holdsLease() {
		t.Fatal("initial lease not acquired")
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if err := tx.Exec("SELECT 1 FROM sys_job_lease WHERE name = ? FOR UPDATE", models.SchedulerLeaseName).Error; err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); s.tick() }()
	time.Sleep(400 * time.Millisecond)
	if s.holdsLease() {
		t.Error("PostgreSQL row lock extended local scheduler lease")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("renewal SQL ignored lease deadline")
	}
	if err := tx.Rollback().Error; err != nil {
		t.Fatal(err)
	}
	successor := &lease{db: db, owner: "pg-second", ttl: time.Second}
	if held, err := successor.acquire(); err != nil || !held {
		t.Fatalf("successor held=%v err=%v", held, err)
	}
}
