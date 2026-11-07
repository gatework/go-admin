package jobs

import (
	"context"
	"gorm.io/gorm"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg/cronjob"

	models2 "go-admin/app/jobs/models"
)

// An instance that starts while another one holds the lease must not
// register the jobs. This is the whole point: every instance registering the
// whole enabled list into its own scheduler is what made one job fire once
// per instance (#915).
func TestASecondInstanceDoesNotScheduleWhileTheFirstHoldsTheLease(t *testing.T) {
	const tenant = "second-instance"
	db := leaseDB(t)
	sdk.Runtime.SetCrontabByTenant(tenant, cronjob.NewWithSeconds())

	first := newSupervisor(tenant, db, "instance-a")
	first.tick()
	if !first.holdsLease() {
		t.Fatal("the first instance did not take a free lease")
	}

	second := newSupervisor(tenant, db, "instance-b")
	second.tick()
	if second.holdsLease() {
		t.Error("a second instance scheduled while the first holds the lease: the job would fire twice per tick")
	}
}

// Losing the lease has to stop the scheduler, not merely stop it from being
// taken again. A holder that keeps scheduling after its lease has gone to
// somebody else is two schedulers at once - the defect the lease exists to
// prevent, reached from the other direction.
func TestTheSupervisorStopsSchedulingWhenItLosesTheLease(t *testing.T) {
	const tenant = "loses-lease"
	db := leaseDB(t)
	sdk.Runtime.SetCrontabByTenant(tenant, cronjob.NewWithSeconds())

	holder := newSupervisor(tenant, db, "instance-a")
	holder.tick()
	if !holder.holdsLease() {
		t.Fatal("the supervisor did not take a free lease, so losing it cannot be observed")
	}

	// What a partition looks like from the database's side: the lease
	// lapsed and somebody else took it while this instance was away.
	expire(t, db)
	successor := &lease{db: db, owner: "instance-b", ttl: time.Minute}
	if held, err := successor.acquire(); err != nil || !held {
		t.Fatalf("the successor could not take the expired lease: held=%v err=%v", held, err)
	}

	holder.tick()

	if holder.holdsLease() {
		t.Error("the supervisor kept scheduling after the lease went to another instance")
	}
	if got := readLease(t, db).Owner; got != "instance-b" {
		t.Errorf("owner is %q; the instance that lost the lease wrote over its successor", got)
	}
}

// A database that cannot be reached is not the same as a lease that has been
// lost. Stopping on the first failed renewal would stop the jobs every time
// the database blinked - and hand them to nobody, because no other instance
// can reach it to take the lease either.
func TestABrieflyUnreachableDatabaseDoesNotStopTheScheduler(t *testing.T) {
	const tenant = "db-blip"
	db := leaseDB(t)
	sdk.Runtime.SetCrontabByTenant(tenant, cronjob.NewWithSeconds())

	s := newSupervisor(tenant, db, "instance-a")
	s.tick()
	if !s.holdsLease() {
		t.Fatal("the supervisor did not take a free lease")
	}

	// The table going missing is how an unreachable database presents to
	// acquire: every statement against it returns an error.
	if err := db.Migrator().DropTable(&models2.SysJobLease{}); err != nil {
		t.Fatalf("dropping the lease table: %v", err)
	}

	s.tick()

	if !s.holdsLease() {
		t.Error("one failed renewal stopped the scheduler; the lease had not expired yet and nobody else could have taken it")
	}
}

func TestBlockedRenewalExpiresIndependently(t *testing.T) {
	db := leaseDB(t)
	const tenant = "blocked-renewal-regression"
	c := cronjob.NewWithSeconds()
	sdk.Runtime.SetCrontabByTenant(tenant, c)
	s := newSupervisor(tenant, db, "first")
	s.lease.ttl = 120 * time.Millisecond
	s.tick()
	t.Cleanup(func() { s.stopScheduling() })
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once atomic.Bool
	if err := db.Callback().Row().Before("gorm:row").Register("block-renewal", func(tx *gorm.DB) {
		if once.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
	}); err != nil {
		t.Fatal(err)
	}
	go func() { defer close(done); s.tick() }()
	<-entered
	time.Sleep(200 * time.Millisecond)
	if s.holdsLease() {
		t.Error("blocked renewal kept scheduler active past lease deadline")
	}
	close(release)
	<-done
	if s.holdsLease() {
		t.Error("late renewal restarted scheduler with stale proof")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s.control.waitStopped(ctx)
}

// A successful UPDATE delivered late is no newer proof than its start time.
func TestSlowSuccessfulAcquisitionCannotExtendLeaseFromReceipt(t *testing.T) {
	db := leaseDB(t).Session(&gorm.Session{SkipDefaultTransaction: true})
	const key = "slow-successful-acquisition"
	sdk.Runtime.SetCrontabByTenant(key, cronjob.NewWithSeconds())
	s := newSupervisor(key, db, "slow-owner")
	s.lease.ttl = 80 * time.Millisecond
	var delayed atomic.Bool
	if err := db.Callback().Update().After("gorm:update").Register("delay-successful-reply", func(tx *gorm.DB) {
		if tx.Error == nil && tx.RowsAffected > 0 && delayed.CompareAndSwap(false, true) {
			time.Sleep(120 * time.Millisecond)
		}
	}); err != nil {
		t.Fatal(err)
	}
	s.tick()
	t.Cleanup(s.stopScheduling)
	if !delayed.Load() {
		t.Fatal("fixture did not delay successful acquisition")
	}
	if s.holdsLease() {
		t.Fatal("reply latency extended the local lease")
	}
}
