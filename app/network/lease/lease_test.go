package lease

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// job stands in for any module's job rows, to exercise the query scopes.
type job struct {
	ID       int `gorm:"primaryKey"`
	DeviceID int
}

func (job) TableName() string { return "lease_test_job" }

var start = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

// each runs a scenario against SQLite and, when GO_ADMIN_TEST_POSTGRES_DSN is
// set (as in CI), against Postgres too, with the module clock at start.
func each(t *testing.T, scenario func(t *testing.T, db *gorm.DB, at func(time.Duration))) {
	t.Run("sqlite", func(t *testing.T) {
		db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		run(t, db, scenario)
	})
	t.Run("postgres", func(t *testing.T) {
		dsn := os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("GO_ADMIN_TEST_POSTGRES_DSN is required")
		}
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		pool, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		pool.SetMaxOpenConns(1)
		t.Cleanup(func() { pool.Close() })
		for _, sql := range []string{"DROP SCHEMA IF EXISTS lease_test CASCADE", "CREATE SCHEMA lease_test", "SET search_path TO lease_test"} {
			if err := db.Exec(sql).Error; err != nil {
				t.Fatal(err)
			}
		}
		t.Cleanup(func() { db.Exec("DROP SCHEMA IF EXISTS lease_test CASCADE") })
		run(t, db, scenario)
	})
}

func run(t *testing.T, db *gorm.DB, scenario func(t *testing.T, db *gorm.DB, at func(time.Duration))) {
	if err := db.AutoMigrate(&Record{}, &job{}); err != nil {
		t.Fatal(err)
	}
	original := now
	t.Cleanup(func() { now = original })
	at := func(d time.Duration) { now = func() time.Time { return start.Add(d) } }
	at(0)
	scenario(t, db, at)
}

func row(t *testing.T, db *gorm.DB, key int) (Record, bool) {
	t.Helper()
	var r Record
	err := db.Where("device_id = ?", key).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r, false
	}
	if err != nil {
		t.Fatal(err)
	}
	return r, true
}

func must(t *testing.T, err error, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func TestOneLeasePerDeviceAndExpiryIsRunPlusMargin(t *testing.T) {
	each(t, func(t *testing.T, db *gorm.DB, _ func(time.Duration)) {
		held, err := Acquire(db, 7, 42, 10*time.Minute)
		must(t, err, nil)
		if !held.ExpiresAt.Equal(start.Add(10*time.Minute + Margin)) {
			t.Fatalf("expiry %v", held.ExpiresAt)
		}
		_, err = Acquire(db, 7, 43, time.Minute)
		must(t, err, ErrBusy)
		_, err = Acquire(db, 8, 44, time.Minute)
		must(t, err, nil)
		if r, _ := row(t, db, 7); r.Token != held.Token || r.TargetID != 42 {
			t.Fatalf("lease row %+v", r)
		}
	})
}

func TestReleaseFreesOrQuarantinesByOutcome(t *testing.T) {
	each(t, func(t *testing.T, db *gorm.DB, _ func(time.Duration)) {
		closed, _ := Acquire(db, 1, 0, time.Minute)
		must(t, Release(db, closed, Closed), nil)
		if _, ok := row(t, db, 1); ok {
			t.Fatal("confirmed close must free the device")
		}
		none, _ := Acquire(db, 2, 0, time.Minute)
		must(t, Release(db, none, NoSession), nil)
		if _, ok := row(t, db, 2); ok {
			t.Fatal("no session must free the device")
		}
		unknown, _ := Acquire(db, 3, 0, time.Minute)
		must(t, Release(db, unknown, Unconfirmed), nil)
		if r, _ := row(t, db, 3); !r.Quarantined {
			t.Fatal("unconfirmed close must quarantine")
		}
		_, err := Acquire(db, 3, 0, time.Minute)
		must(t, err, ErrQuarantined)
	})
}

func TestReleaseNeverTouchesAnotherOwnersLease(t *testing.T) {
	each(t, func(t *testing.T, db *gorm.DB, _ func(time.Duration)) {
		owner, _ := Acquire(db, 1, 0, time.Minute)
		stale := Lease{DeviceID: 1, Token: "former-owner"}
		must(t, Release(db, stale, Closed), ErrLost)
		must(t, Release(db, stale, Unconfirmed), ErrLost)
		if r, _ := row(t, db, 1); r.Token != owner.Token || r.Quarantined {
			t.Fatalf("owner's lease changed: %+v", r)
		}
	})
}

func TestLateReleaseCannotClearAQuarantine(t *testing.T) {
	each(t, func(t *testing.T, db *gorm.DB, at func(time.Duration)) {
		late, _ := Acquire(db, 1, 0, time.Minute)
		at(time.Minute + Margin + time.Second)
		must(t, Sweep(db), nil)
		// The worker outlived its lease and now reports a clean close.
		must(t, Release(db, late, Closed), ErrLost)
		if r, ok := row(t, db, 1); !ok || !r.Quarantined {
			t.Fatal("a late release cleared the quarantine")
		}
		_, err := Acquire(db, 1, 0, time.Minute)
		must(t, err, ErrQuarantined)
	})
}

func TestExpiredLeaseIsNeverReusedEvenBeforeASweep(t *testing.T) {
	each(t, func(t *testing.T, db *gorm.DB, at func(time.Duration)) {
		if _, err := Acquire(db, 1, 0, time.Minute); err != nil {
			t.Fatal(err)
		}
		at(time.Minute + Margin)
		_, err := Acquire(db, 1, 0, time.Minute)
		must(t, err, ErrQuarantined)
		if r, _ := row(t, db, 1); !r.Quarantined {
			t.Fatal("expired lease was not quarantined")
		}
	})
}

func TestCheckRequiresALiveLeaseThisCallerHolds(t *testing.T) {
	each(t, func(t *testing.T, db *gorm.DB, at func(time.Duration)) {
		held, _ := Acquire(db, 1, 0, time.Minute)
		must(t, Check(db, held), nil)
		must(t, Check(db, Lease{DeviceID: 1, Token: "someone-else"}), ErrLost)
		at(time.Minute + Margin)
		must(t, Check(db, held), ErrLost)

		at(0)
		quarantined, _ := Acquire(db, 2, 0, time.Minute)
		must(t, db.Model(&Record{}).Where("device_id = ?", 2).Update("quarantined", true).Error, nil)
		must(t, Check(db, quarantined), ErrLost)
	})
}

func TestSweepCoversEveryModulesLeasesAndIsIdempotent(t *testing.T) {
	each(t, func(t *testing.T, db *gorm.DB, at func(time.Duration)) {
		// Leases taken by different modules, told apart only by their refs.
		_, _ = Acquire(db, 1, 100, time.Minute)
		_, _ = Acquire(db, 2, 200, time.Minute)
		_, _ = LockFabric(db, 1, 300, time.Minute)
		live, _ := Acquire(db, 3, 400, time.Hour)
		at(time.Minute + Margin + time.Second)
		for i := 0; i < 2; i++ {
			must(t, Sweep(db), nil)
		}
		for _, id := range []int{1, 2} {
			if r, ok := row(t, db, id); !ok || !r.Quarantined {
				t.Fatalf("expired lease on device %d not quarantined", id)
			}
		}
		if _, ok := row(t, db, -1); ok {
			t.Fatal("expired fabric lock must be dropped, not quarantined")
		}
		if r, _ := row(t, db, 3); r.Quarantined || r.Token != live.Token {
			t.Fatal("sweep touched a live lease")
		}
	})
}

func TestFabricLockIsSeparateFromDevicesAndReplacedOnceExpired(t *testing.T) {
	each(t, func(t *testing.T, db *gorm.DB, at func(time.Duration)) {
		lock, err := LockFabric(db, 5, 1, time.Minute)
		must(t, err, nil)
		_, err = LockFabric(db, 5, 2, time.Minute)
		must(t, err, ErrBusy)
		// Device 5 and Fabric 5 are different things.
		_, err = Acquire(db, 5, 0, time.Minute)
		must(t, err, nil)
		at(time.Minute + Margin)
		next, err := LockFabric(db, 5, 3, time.Minute)
		must(t, err, nil)
		must(t, Release(db, lock, NoSession), ErrLost)
		must(t, Release(db, next, NoSession), nil)
	})
}

func TestAcquireJoinsTheCallersTransaction(t *testing.T) {
	each(t, func(t *testing.T, db *gorm.DB, _ func(time.Duration)) {
		rollback := errors.New("rollback")
		err := db.Transaction(func(tx *gorm.DB) error {
			if _, err := Acquire(tx, 1, 0, time.Minute); err != nil {
				return err
			}
			return rollback
		})
		must(t, err, rollback)
		if _, ok := row(t, db, 1); ok {
			t.Fatal("a rolled-back acquire left a lease")
		}
	})
}

func TestScopesSplitJobsByTheirDevicesLease(t *testing.T) {
	each(t, func(t *testing.T, db *gorm.DB, at func(time.Duration)) {
		must(t, db.Create(&[]job{{ID: 1, DeviceID: 1}, {ID: 2, DeviceID: 2}, {ID: 3, DeviceID: 3}, {ID: 4, DeviceID: 4}}).Error, nil)
		_, _ = Acquire(db, 2, 0, time.Hour)
		quarantined, _ := Acquire(db, 3, 0, time.Hour)
		must(t, Release(db, quarantined, Unconfirmed), nil)
		_, _ = Acquire(db, 4, 0, time.Minute)
		at(time.Minute + Margin) // device 4's lease has expired but nobody swept yet
		ids := func(scope func(*gorm.DB) *gorm.DB) []int {
			var out []int
			must(t, db.Model(&job{}).Scopes(scope).Order("id").Pluck("id", &out).Error, nil)
			return out
		}
		const column = "lease_test_job.device_id"
		if got := ids(Free(column)); len(got) != 1 || got[0] != 1 {
			t.Fatalf("free %v", got)
		}
		if got := ids(Held(column)); len(got) != 1 || got[0] != 2 {
			t.Fatalf("held %v", got)
		}
		if got := ids(Quarantined(column)); len(got) != 2 || got[0] != 3 || got[1] != 4 {
			t.Fatalf("quarantined %v", got)
		}
	})
}

func TestInvalidIDsAreRefused(t *testing.T) {
	each(t, func(t *testing.T, db *gorm.DB, _ func(time.Duration)) {
		_, err := Acquire(db, 0, 0, time.Minute)
		must(t, err, ErrInvalid)
		_, err = Acquire(db, -3, 0, time.Minute)
		must(t, err, ErrInvalid)
		_, err = LockFabric(db, 0, 0, time.Minute)
		must(t, err, ErrInvalid)
	})
}

func TestCheckRejectsLeaseThatExpiresDuringRead(t *testing.T) {
	each(t, func(t *testing.T, db *gorm.DB, at func(time.Duration)) {
		held, err := Acquire(db, 1, 0, time.Minute)
		must(t, err, nil)
		if err := db.Callback().Query().After("gorm:query").Register("advance_lease_clock", func(*gorm.DB) { at(time.Minute + Margin + time.Second) }); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Callback().Query().Remove("advance_lease_clock") })
		must(t, Check(db, held), ErrLost)
	})
}

func TestLateReleaseBeforeSweepCannotFreeExpiredDevice(t *testing.T) {
	each(t, func(t *testing.T, db *gorm.DB, at func(time.Duration)) {
		held, err := Acquire(db, 1, 0, time.Minute)
		must(t, err, nil)
		at(time.Minute + Margin)
		must(t, Release(db, held, Closed), ErrLost)
		_, err = Acquire(db, 1, 0, time.Minute)
		must(t, err, ErrQuarantined)
	})
}
