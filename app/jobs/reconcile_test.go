package jobs

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
	"weak"

	"github.com/robfig/cron/v3"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/pkg/cronjob"
	"github.com/gatework/motor/app/jobs/models"
)

func TestReconcileCommittedChangesAndStandby(t *testing.T) {
	db := leaseDB(t)
	job := models.SysJob{JobName: "original", JobType: 2, Status: 2, CronExpression: "0 * * * * *", Args: "old"}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	const leaderKey = "reconcile-leader"
	const standbyKey = "reconcile-standby"
	leaderCron, standbyCron := cronjob.NewWithSeconds(), cronjob.NewWithSeconds()
	sdk.Runtime.SetCrontab(leaderKey, leaderCron)
	sdk.Runtime.SetCrontab(standbyKey, standbyCron)
	leader := newSupervisor(leaderKey, db, "first")
	standby := newSupervisor(standbyKey, db, "second")
	t.Cleanup(leader.stopScheduling)
	t.Cleanup(standby.stopScheduling)
	leader.tick()
	standby.tick()
	if len(leaderCron.Entries()) != 1 || len(standbyCron.Entries()) != 0 {
		t.Fatal("only leader should schedule")
	}
	if err := db.Model(&job).Updates(map[string]any{"args": "", "cron_expression": "0 */2 * * * *"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := standby.reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(standbyCron.Entries()) != 0 {
		t.Fatal("standby applied local schedule")
	}
	leader.tick()
	entries := leaderCron.Entries()
	if len(entries) != 1 {
		t.Fatalf("update left %d entries", len(entries))
	}
	updated := entries[0].Job.(*TaskJob)
	if updated.Args != "" || updated.CronExpression != "0 */2 * * * *" {
		t.Fatalf("stale job: %+v", updated)
	}
	if err := db.Model(&job).Update("status", 1).Error; err != nil {
		t.Fatal(err)
	}
	leader.tick()
	if len(leaderCron.Entries()) != 0 {
		t.Fatal("disabled job still scheduled")
	}
	if err := db.Model(&job).Update("status", 2).Error; err != nil {
		t.Fatal(err)
	}
	leader.tick()
	if len(leaderCron.Entries()) != 1 {
		t.Fatal("enabled job not scheduled")
	}
	if err := db.Delete(&job).Error; err != nil {
		t.Fatal(err)
	}
	leader.tick()
	if len(leaderCron.Entries()) != 0 {
		t.Fatal("soft-deleted job still scheduled")
	}
	leader.stopScheduling()
	if err := leader.lease.release(); err != nil {
		t.Fatal(err)
	}
	standby.tick()
	if !standby.holdsLease() || len(standbyCron.Entries()) != 0 {
		t.Fatal("takeover resurrected deleted job")
	}
}

// A completed standalone request must not leave the scheduler and its job
// payloads rooted in a process-wide coordination registry.
func TestCompletedStandaloneCronsCanBeCollected(t *testing.T) {
	db := leaseDB(t)
	create := func() weak.Pointer[cron.Cron] {
		c := cronjob.NewWithSeconds()
		if err := ReconcileCron(t.Context(), db, c); err != nil {
			t.Fatal(err)
		}
		return weak.Make(c)
	}
	crons := make([]weak.Pointer[cron.Cron], 8)
	for i := range crons {
		crons[i] = create()
	}
	deadline := time.Now().Add(time.Second)
	for {
		runtime.GC()
		retained := 0
		for _, c := range crons {
			if c.Value() != nil {
				retained++
			}
		}
		if retained == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("completed requests retain %d standalone schedulers", retained)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The leader reconciles every heartbeat; when no entry ID moved it must not
// rewrite every enabled job.
func TestSaveEntryIDsWritesOnlyChanges(t *testing.T) {
	db := leaseDB(t)
	if err := db.AutoMigrate(&models.SysJob{}); err != nil {
		t.Fatal(err)
	}
	jobs := []models.SysJob{{JobID: 1, Status: models.StatusEnabled, EntryID: 3}, {JobID: 2, Status: models.StatusEnabled, EntryID: 4}}
	for _, job := range jobs {
		if err := db.Create(&job).Error; err != nil {
			t.Fatal(err)
		}
	}
	var updates int
	if err := db.Callback().Update().After("gorm:update").Register("count_updates", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "job_id =") {
			updates++
		}
	}); err != nil {
		t.Fatal(err)
	}
	stored := storedEntryIDs(jobs)
	if err := saveEntryIDs(t.Context(), db, jobs, stored); err != nil {
		t.Fatal(err)
	}
	unchanged := updates
	jobs[1].EntryID = 9
	if err := saveEntryIDs(t.Context(), db, jobs, stored); err != nil {
		t.Fatal(err)
	}
	if updates-unchanged != 1 {
		t.Fatalf("%d per-job writes for one changed entry", updates-unchanged)
	}
	var second models.SysJob
	if err := db.First(&second, 2).Error; err != nil || second.EntryID != 9 {
		t.Fatalf("changed entry not saved: %+v %v", second, err)
	}
}
