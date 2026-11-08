//go:build integration

package devicelease_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	dm "go-admin/app/delivery/models"
	"go-admin/app/network/lease"
	"gorm.io/gorm"
)

func TestConcurrentAcquisitionAndTransactionRollback(t *testing.T) {
	a, b := database(t)
	start := make(chan struct{})
	result := make(chan bool, 2)
	failures := make(chan error, 2)
	var wg sync.WaitGroup
	for i, db := range []*gorm.DB{a, b} {
		wg.Add(1)
		go func(i int, db *gorm.DB) {
			defer wg.Done()
			<-start
			ok, err := lease.TryAcquire(db, lease.Claim{Owner: lease.Owner{DeviceID: 10, Token: []string{"first", "second"}[i]}, ExpiresAt: time.Now().Add(time.Minute)})
			result <- ok
			failures <- err
		}(i, db)
	}
	close(start)
	wg.Wait()
	close(result)
	close(failures)
	winners := 0
	for ok := range result {
		if ok {
			winners++
		}
	}
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("winners=%d", winners)
	}
	rollback := errors.New("business write failed")
	err := a.Transaction(func(tx *gorm.DB) error {
		ok, err := lease.TryAcquire(tx, lease.Claim{Owner: lease.Owner{DeviceID: 20, Token: "rollback"}, ExpiresAt: time.Now().Add(time.Minute)})
		if err != nil {
			return err
		}
		if !ok {
			t.Fatal("fresh acquisition refused")
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if ok, err := lease.TryAcquire(b, lease.Claim{Owner: lease.Owner{DeviceID: 20, Token: "next"}, ExpiresAt: time.Now().Add(time.Minute)}); err != nil || !ok {
		t.Fatal("rolled-back claim survived", err)
	}
	if err := a.Transaction(func(tx *gorm.DB) error {
		if err := lease.Complete(tx, lease.Owner{DeviceID: 20, Token: "next"}, true); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if ok, err := lease.TryAcquire(b, lease.Claim{Owner: lease.Owner{DeviceID: 20, Token: "unexpected"}, ExpiresAt: time.Now().Add(time.Minute)}); err != nil || ok {
		t.Fatal("rolled-back release escaped", err)
	}
}

func TestDeliveryClaimRollsBackWithBusinessWrite(t *testing.T) {
	a, b := database(t)
	gate := newBarrier(t, true)
	worker, _ := fixture(t, "wireless", a, b, gate)
	injected := errors.New("target update rejected")
	name := "devicelease_reject_target"
	if err := a.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "sys_delivery_target" {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Callback().Update().Remove(name) })
	if target, err := worker.Claim(context.Background()); !errors.Is(err, injected) || target != nil {
		t.Fatalf("claim=%v error=%v", target, err)
	}
	var count int64
	if err := b.Model(&dm.DeviceLease{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("lease escaped claim transaction", count, err)
	}
}

func TestOldOwnerCannotCleanReplacement(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(map[bool]string{false: "quarantine", true: "release"}[closed], func(t *testing.T) {
			a, b := database(t)
			gate := newBarrier(t, closed)
			worker, _ := fixture(t, "wireless", a, b, gate)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			target, err := worker.Claim(ctx)
			if err != nil || target == nil {
				t.Fatal(err)
			}
			ended := make(chan struct{})
			go func() { defer close(ended); worker.RunTarget(ctx, *target) }()
			t.Cleanup(func() {
				gate.finish()
				select {
				case <-ended:
				case <-time.After(10 * time.Second):
					t.Error("worker cleanup timeout")
				}
			})
			entered(t, gate)
			if err := b.Model(&dm.DeviceLease{}).Where("device_id = ?", target.DeviceID).Update("token", "replacement").Error; err != nil {
				t.Fatal(err)
			}
			gate.finish()
			select {
			case <-ended:
			case <-time.After(10 * time.Second):
				t.Fatal("worker did not finish")
			}
			var current dm.DeviceLease
			if err := b.First(&current, target.DeviceID).Error; err != nil || current.Token != "replacement" || current.Quarantined {
				t.Fatal("old cleanup changed replacement", current, err)
			}
		})
	}
}

func TestPublicationOwnerLockAndTenantIsolation(t *testing.T) {
	a, b := database(t)
	other, _ := database(t)
	claim := lease.Claim{Owner: lease.Owner{DeviceID: 1, Token: "owner"}, ExpiresAt: time.Now().Add(time.Minute)}
	for _, db := range []*gorm.DB{a, other} {
		if ok, err := lease.TryAcquire(db, claim); err != nil || !ok {
			t.Fatal("tenant claim", ok, err)
		}
	}
	tx := a.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if err := lease.LockOwner(tx, claim.Owner, time.Now()); err != nil {
		t.Fatal(err)
	}
	contender := b.Begin()
	if contender.Error != nil {
		t.Fatal(contender.Error)
	}
	if err := contender.Exec("SET LOCAL lock_timeout = '100ms'").Error; err != nil {
		t.Fatal(err)
	}
	err := contender.Model(&dm.DeviceLease{}).Where("device_id = ?", 1).Update("token", "replacement").Error
	contender.Rollback()
	type sqlState interface{ SQLState() string }
	var state sqlState
	if !errors.As(err, &state) || state.SQLState() != "55P03" {
		t.Fatalf("expected PostgreSQL lock timeout, got %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := b.Model(&dm.DeviceLease{}).Where("device_id = ?", 1).Update("token", "replacement").Error; err != nil {
		t.Fatal(err)
	}
	if ok, err := lease.CheckOwner(other, claim.Owner, time.Now); err != nil || !ok {
		t.Fatal("other tenant changed", err)
	}
}

func TestFirewallClaimRollsBackWithBusinessWrite(t *testing.T) {
	a, b := database(t)
	gate := newBarrier(t, true)
	_, peer := fixture(t, "firewall", a, b, gate)
	injected := errors.New("target update rejected")
	name := "devicelease_reject_firewall_target"
	if err := b.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "sys_firewall_collection_target" {
			if _, ok := tx.Statement.Dest.(map[string]any); !ok {
				tx.AddError(injected)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Callback().Update().Remove(name) })
	if err := peer(context.Background()); !errors.Is(err, injected) {
		t.Fatalf("claim error=%v", err)
	}
	var count int64
	if err := a.Model(&dm.DeviceLease{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("lease escaped claim transaction", count, err)
	}
	if gate.calls.Load() != 0 {
		t.Fatal("failed claim opened transport")
	}
}

func TestWirelessPublicationHoldsOwnerLock(t *testing.T) {
	a, b := database(t)
	gate := newBarrier(t, true)
	_, peer := fixture(t, "wireless", a, b, gate)
	gate.finish()
	locked, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	name := "devicelease_pause_publication"
	if err := b.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		values, ok := tx.Statement.Dest.(map[string]any)
		if tx.Statement.Table == "sys_wireless_collection" && ok && values["results"] != nil && values["finished_at"] != nil {
			close(locked)
			select {
			case <-release:
			case <-tx.Statement.Context.Done():
				tx.AddError(tx.Statement.Context.Err())
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Callback().Update().Remove(name) })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	ended := make(chan struct{})
	go func() { defer close(ended); finished <- peer(ctx) }()
	t.Cleanup(func() {
		unblock()
		select {
		case <-ended:
		case <-time.After(10 * time.Second):
			t.Error("publication did not stop")
		}
	})
	select {
	case <-locked:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not reach publication")
	}
	contender := a.Begin()
	if contender.Error != nil {
		t.Fatal(contender.Error)
	}
	if err := contender.Exec("SET LOCAL lock_timeout = '100ms'").Error; err != nil {
		contender.Rollback()
		t.Fatal(err)
	}
	err := contender.Model(&dm.DeviceLease{}).Where("device_id = ?", 1).Update("token", "unexpected").Error
	contender.Rollback()
	type sqlState interface{ SQLState() string }
	var state sqlState
	if !errors.As(err, &state) || state.SQLState() != "55P03" {
		t.Fatalf("publication did not lock owner: %v", err)
	}
	unblock()
	done(t, finished)
}

func TestDeliveryCancellationStillCleansConfirmedClose(t *testing.T) {
	a, b := database(t)
	gate := newBarrier(t, true)
	worker, _ := fixture(t, "wireless", a, b, gate)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	target, err := worker.Claim(ctx)
	if err != nil || target == nil {
		t.Fatal(err)
	}
	ended := make(chan struct{})
	go func() { defer close(ended); worker.RunTarget(ctx, *target) }()
	t.Cleanup(func() {
		gate.finish()
		select {
		case <-ended:
		case <-time.After(10 * time.Second):
			t.Error("worker did not stop")
		}
	})
	entered(t, gate)
	cancel()
	select {
	case <-ended:
	case <-time.After(10 * time.Second):
		t.Fatal("cancel did not stop worker")
	}
	if ok, err := lease.TryAcquire(b, lease.Claim{Owner: lease.Owner{DeviceID: target.DeviceID, Token: "after-cancel"}, ExpiresAt: time.Now().Add(time.Minute)}); err != nil || !ok {
		t.Fatal("cancel stranded confirmed-closed owner", err)
	}
}

func TestWorkersRacePastCandidateSelection(t *testing.T) {
	for _, kind := range []string{"wireless", "fabric", "firewall"} {
		t.Run(kind, func(t *testing.T) {
			a, b := database(t)
			gate := newBarrier(t, true)
			worker, peer := fixture(t, kind, a, b, gate)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			arrivals := make(chan struct{}, 2)
			insert := make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(insert) }) }
			defer release()
			name := "devicelease_simultaneous_insert"
			callback := func(tx *gorm.DB) {
				if row, ok := tx.Statement.Dest.(*lease.Record); ok && row.DeviceID > 0 {
					arrivals <- struct{}{}
					select {
					case <-insert:
					case <-ctx.Done():
						tx.AddError(ctx.Err())
					}
				}
			}
			for _, db := range []*gorm.DB{a, b} {
				if err := db.Callback().Create().Before("gorm:create").Register(name, callback); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() {
				for _, db := range []*gorm.DB{a, b} {
					db.Callback().Create().Remove(name)
				}
			})
			finished := make(chan error, 2)
			var workers sync.WaitGroup
			workers.Add(2)
			go func() {
				defer workers.Done()
				target, err := worker.Claim(ctx)
				if err == nil && target != nil {
					worker.RunTarget(ctx, *target)
				}
				finished <- err
			}()
			go func() { defer workers.Done(); finished <- peer(ctx) }()
			stopped := make(chan struct{})
			go func() { workers.Wait(); close(stopped) }()
			t.Cleanup(func() {
				release()
				gate.finish()
				cancel()
				select {
				case <-stopped:
				case <-time.After(10 * time.Second):
					t.Error("competing workers did not stop")
				}
			})
			for range 2 {
				select {
				case <-arrivals:
				case <-ctx.Done():
					t.Fatal("workers did not reach concurrent acquisition")
				}
			}
			release()
			entered(t, gate)
			var count int64
			if err := a.Model(&dm.DeviceLease{}).Where("device_id > 0").Count(&count).Error; err != nil || count != 1 {
				t.Fatal("multiple owners", count, err)
			}
			done(t, finished) // The loser must finish while the winner still owns the device.
			gate.finish()
			done(t, finished)
			if gate.calls.Load() != 1 {
				t.Fatalf("transport opened %d times", gate.calls.Load())
			}
		})
	}
}
