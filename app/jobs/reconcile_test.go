package jobs

import (
	"context"
	"github.com/robfig/cron/v3"
	"runtime"
	"testing"
	"time"
	"weak"

	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg/cronjob"
	"go-admin/app/jobs/models"
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
	sdk.Runtime.SetCrontabByTenant(leaderKey, leaderCron)
	sdk.Runtime.SetCrontabByTenant(standbyKey, standbyCron)
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
	updated := entries[0].Job.(*ExecJob)
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
