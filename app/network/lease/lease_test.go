package lease_test

import (
	"context"
	"errors"
	"github.com/glebarez/sqlite"
	"go-admin/app/network/lease"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"path/filepath"
	"testing"
	"time"
)

func database(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "lease.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sql, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sql.Close() })
	if err := db.AutoMigrate(&lease.Record{}); err != nil {
		t.Fatal(err)
	}
	return db
}
func acquire(t *testing.T, db *gorm.DB, owner lease.Owner, until time.Time) {
	t.Helper()
	ok, err := lease.TryAcquire(db, lease.Claim{Owner: owner, ExpiresAt: until})
	if err != nil || !ok {
		t.Fatalf("acquire: %v %v", ok, err)
	}
}
func TestOwnershipAndClosure(t *testing.T) {
	db := database(t)
	owner := lease.Owner{DeviceID: 1, Token: "original"}
	until := time.Now().Add(time.Hour)
	acquire(t, db, owner, until)
	if ok, err := lease.TryAcquire(db, lease.Claim{Owner: lease.Owner{DeviceID: 1, Token: "other"}, ExpiresAt: until}); err != nil || ok {
		t.Fatalf("occupied acquisition: %v %v", ok, err)
	}
	for _, closed := range []bool{true, false} {
		if err := lease.Complete(db, lease.Owner{DeviceID: 1, Token: "stale"}, closed); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := lease.CheckOwner(db, owner, time.Now); err != nil || !ok {
		t.Fatalf("stale cleanup changed owner: %v %v", ok, err)
	}
	if err := lease.Complete(db, owner, false); err != nil {
		t.Fatal(err)
	}
	if ok, err := lease.CheckOwner(db, owner, time.Now); err != nil || ok {
		t.Fatalf("quarantine: %v %v", ok, err)
	}
	if err := lease.Complete(db, owner, true); err != nil {
		t.Fatal(err)
	}
	if err := lease.Complete(db, owner, true); err != nil {
		t.Fatal(err)
	}
	acquire(t, db, lease.Owner{DeviceID: 1, Token: "next"}, until)
}
func TestExpiryAndPublication(t *testing.T) {
	db := database(t)
	now := time.Now().UTC().Truncate(time.Second)
	owner := lease.Owner{DeviceID: 1, Token: "owner"}
	acquire(t, db, owner, now)
	if ok, err := lease.CheckOwner(db, owner, func() time.Time { return now }); err != nil || !ok {
		t.Fatalf("inclusive check: %v %v", ok, err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return lease.LockOwner(tx, owner, now) }); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("strict publication: %v", err)
	}
	if err := lease.QuarantineExpired(db, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if ok, err := lease.TryAcquire(db, lease.Claim{Owner: owner, ExpiresAt: now.Add(time.Hour)}); err != nil || ok {
		t.Fatalf("expired row stolen: %v %v", ok, err)
	}
	var occupied, active []int
	if err := lease.OccupiedDevices(db).Scan(&occupied).Error; err != nil {
		t.Fatal(err)
	}
	if err := lease.ActiveDevices(db, now).Scan(&active).Error; err != nil {
		t.Fatal(err)
	}
	if len(occupied) != 1 || len(active) != 0 {
		t.Fatalf("selection: %v %v", occupied, active)
	}
}
func TestCallerTransactionAndCancellation(t *testing.T) {
	db := database(t)
	owner := lease.Owner{DeviceID: 1, Token: "owner"}
	stop := errors.New("rollback")
	err := db.Transaction(func(tx *gorm.DB) error { acquire(t, tx, owner, time.Now().Add(time.Hour)); return stop })
	if !errors.Is(err, stop) {
		t.Fatal(err)
	}
	acquire(t, db, owner, time.Now().Add(time.Hour))
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := lease.Complete(tx, owner, true); err != nil {
			return err
		}
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatal(err)
	}
	if ok, err := lease.CheckOwner(db, owner, time.Now); err != nil || !ok {
		t.Fatalf("completion escaped tx: %v %v", ok, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := lease.Complete(db.WithContext(ctx), owner, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
func TestRecoveryRetainsCoordinatorCompatibility(t *testing.T) {
	db := database(t)
	past := time.Now().Add(-time.Hour)
	for _, id := range []int{-1, 1} {
		acquire(t, db, lease.Owner{DeviceID: id, Token: "old"}, past)
	}
	if err := lease.QuarantineExpired(db, time.Now()); err != nil {
		t.Fatal(err)
	}
	var rows []lease.Record
	if err := db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || !rows[0].Quarantined || !rows[1].Quarantined {
		t.Fatalf("legacy sweep: %+v", rows)
	}
}

func TestOwnerClockIsSampledAfterRead(t *testing.T) {
	db := database(t)
	expires := time.Now().UTC().Truncate(time.Second)
	owner := lease.Owner{DeviceID: 1, Token: "owner"}
	acquire(t, db, owner, expires)
	observed := expires.Add(-time.Second)
	if err := db.Callback().Query().After("gorm:query").Register("advance_clock", func(*gorm.DB) { observed = expires.Add(time.Second) }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Callback().Query().Remove("advance_clock") })
	valid, err := lease.CheckOwner(db, owner, func() time.Time { return observed })
	if err != nil || valid {
		t.Fatalf("expired during read: %v %v", valid, err)
	}
}
