package diagnostics

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/gatework/motor/app/net/models/diagnosticrun"
	"github.com/gatework/motor/common/account"
	"github.com/gatework/motor/common/errors"
)

func runsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&diagnosticrun.Record{}, &account.Limit{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// Each user may start RunsPerWindow diagnostics per RunWindow; other users
// and the next window are unaffected.
func TestAdmitLimitsRunsPerUser(t *testing.T) {
	now := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	r := Runs{DB: runsDB(t), Now: func() time.Time { return now }}
	ctx := context.Background()
	alice, bob := Owner{Tenant: "t", UserID: 7}, Owner{Tenant: "t", UserID: 8}
	for i := range RunsPerWindow {
		if err := r.Admit(ctx, alice); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	if err := r.Admit(ctx, alice); !errors.Is(err, RateLimitedErr) {
		t.Fatalf("run %d: %v, want RateLimitedErr", RunsPerWindow+1, err)
	}
	if err := r.Admit(ctx, bob); err != nil {
		t.Fatalf("another user was limited: %v", err)
	}
	now = now.Add(RunWindow)
	if err := r.Admit(ctx, alice); err != nil {
		t.Fatalf("next window: %v", err)
	}
}

// Every started run is recorded: who ran which tool against which target.
func TestRecordKeepsWhoRanWhatAgainstWhich(t *testing.T) {
	db := runsDB(t)
	r := Runs{DB: db}
	job := Job{ID: "abc", Tool: "ping", Target: "10.0.0.1", Family: "4", Mode: "client", Command: "ping -4 -c 4 10.0.0.1", StartedAt: time.Now().UTC()}
	if err := r.Record(context.Background(), Owner{Tenant: "t", UserID: 7}, job); err != nil {
		t.Fatal(err)
	}
	var got diagnosticrun.Record
	if err := db.First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.UserID != 7 || got.JobID != "abc" || got.Tool != "ping" || got.Target != "10.0.0.1" || got.Command != job.Command {
		t.Fatalf("recorded %+v", got)
	}
}
