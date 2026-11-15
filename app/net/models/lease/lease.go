// Package lease owns device leases and fabric locks for every network module
// (see GLOSSARY.md): which piece of work may open sessions to a device, and
// which device is quarantined because a session ended without a confirmed
// close.
//
// The rules live here and nowhere else:
//
//   - A device has at most one lease. Taking one never waits: a held lease
//     answers ErrBusy at once, so holding several (a fabric lock, then device
//     leases, always in that order) cannot deadlock.
//   - An expired device lease is never reused. Whoever finds it first, a
//     caller in Acquire or any module's recovery through Sweep, turns it into
//     a quarantine.
//   - A quarantine is cleared only by an operator, by hand (see README.md).
//     No release can clear one, even a late release reporting a clean close.
//   - A fabric lock guards no session, so an expired one is dropped instead
//     of quarantined.
//
// Every call takes the caller's handle, so a lease can join the caller's
// transaction. When Acquire answers ErrQuarantined after quarantining an
// expired lease, that write is part of the caller's transaction; if the caller
// rolls back, the next Sweep quarantines the lease again.
package lease

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Margin is added to every run timeout, so a lease outlives the work it guards.
const Margin = 5 * time.Minute

//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var (
	// BusyErr another piece of work holds the lease. Try again later.
	// BusyErr identifies busy failures.
	BusyErr = errors.New("lease.busy")
	// QuarantinedErr the device may hold an unconfirmed change and takes no new work.
	// QuarantinedErr identifies quarantined failures.
	QuarantinedErr = errors.New("lease.quarantined")
	// LostErr the caller no longer holds this lease: it expired, was quarantined, or was never taken.
	// LostErr identifies lost failures.
	LostErr = errors.New("lease.lost")
	// InvalidErr a device or fabric id that cannot be leased.
	// InvalidErr identifies invalid failures.
	InvalidErr = errors.New("lease.invalid")
)

//revive:enable:error-naming

// Outcome is how the caller's session to the device ended.
type Outcome int

const (
	// Closed the transport closed and the close was confirmed.
	// Closed defines the closed used by network operations.
	Closed Outcome = iota
	// Unconfirmed the close was not confirmed; the device may still hold an unconfirmed change.
	// Unconfirmed defines the unconfirmed used by network operations.
	Unconfirmed
	// NoSession no session was opened under this lease.
	// NoSession defines the no session used by network operations.
	NoSession
)

// Lease is a lease the caller holds. Callers store Token on their own job rows
// so recovery can find it again; they never build one themselves.
type Lease struct {
	DeviceID  int
	Token     string
	ExpiresAt time.Time
}

// Record is the stored lease row. The table keeps its original name; only the
// migration and this package refer to it.
type Record struct {
	DeviceID    int `gorm:"primaryKey;autoIncrement:false"`
	TargetID    int
	Token       string `gorm:"size:40"`
	ExpiresAt   time.Time
	Quarantined bool
}

// TableName returns the stable database table for Record.
func (Record) TableName() string { return table }

const table = "sys_delivery_device_lease"

// now is the module's one clock: UTC from the Go process, replaceable in tests.
var now = func() time.Time { return time.Now().UTC() }

// Acquire takes the lease on a device for work that runs at most run. ref
// identifies the caller's job row for an operator reading the table.
func Acquire(db *gorm.DB, deviceID, ref int, run time.Duration) (Lease, error) {
	if deviceID <= 0 {
		return Lease{}, InvalidErr
	}
	return take(db, deviceID, ref, run, false)
}

// LockFabric takes the fabric lock that serialises coordination of one Fabric.
// Release it with NoSession.
func LockFabric(db *gorm.DB, fabricID, ref int, run time.Duration) (Lease, error) {
	if fabricID <= 0 {
		return Lease{}, InvalidErr
	}
	// Fabric locks share the device table under negative keys, which no
	// inventory device can have.
	return take(db, -fabricID, ref, run, true)
}

func take(db *gorm.DB, key, ref int, run time.Duration, fabric bool) (Lease, error) {
	l := Lease{DeviceID: key, Token: uuid.NewString(), ExpiresAt: now().Add(run + Margin)}
	q := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&Record{DeviceID: key, TargetID: ref, Token: l.Token, ExpiresAt: l.ExpiresAt})
	if q.Error != nil {
		return Lease{}, q.Error
	}
	if q.RowsAffected == 1 {
		return l, nil
	}
	var held Record
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("device_id = ?", key).First(&held).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Released between the insert and the read; the next attempt can take it.
			return Lease{}, BusyErr
		}
		return Lease{}, err
	}
	expired := !held.ExpiresAt.After(now())
	if fabric {
		if !expired {
			return Lease{}, BusyErr
		}
		if err := db.Where("device_id = ? AND token = ?", key, held.Token).Delete(&Record{}).Error; err != nil {
			return Lease{}, err
		}
		q = db.Clauses(clause.OnConflict{DoNothing: true}).Create(&Record{DeviceID: key, TargetID: ref, Token: l.Token, ExpiresAt: l.ExpiresAt})
		if q.Error != nil {
			return Lease{}, q.Error
		}
		if q.RowsAffected != 1 {
			return Lease{}, BusyErr
		}
		return l, nil
	}
	if held.Quarantined {
		return Lease{}, QuarantinedErr
	}
	if expired {
		if err := db.Model(&Record{}).Where("device_id = ? AND token = ?", key, held.Token).Update("quarantined", true).Error; err != nil {
			return Lease{}, err
		}
		return Lease{}, QuarantinedErr
	}
	return Lease{}, BusyErr
}

// Release ends a lease according to how its session ended: a confirmed close
// or no session frees the device, an unconfirmed close quarantines it. It
// answers ErrLost when the caller no longer holds the lease, and then changes
// nothing: a quarantine set meanwhile stays.
func Release(db *gorm.DB, l Lease, outcome Outcome) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// The owner lock can wait past expiry; validate after acquiring it.
		if err := Check(tx, l); err != nil {
			return err
		}
		q := tx.Model(&Record{}).Where("device_id = ? AND token = ? AND quarantined = ? AND expires_at > ?", l.DeviceID, l.Token, false, now())
		var res *gorm.DB
		if outcome == Unconfirmed {
			res = q.Update("quarantined", true)
		} else {
			res = q.Delete(&Record{})
		}
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return LostErr
		}
		return nil
	})
}

// Check confirms, under a row lock in the caller's transaction, that the
// caller still holds a live lease. Call it before every write to the device
// and before publishing a result.
func Check(db *gorm.DB, l Lease) error {
	var held Record
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("device_id = ? AND token = ? AND quarantined = ? AND expires_at > ?", l.DeviceID, l.Token, false, now()).First(&held).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return LostErr
	}
	if err != nil {
		return err
	}
	// A row lock or a slow query may have outlived the sampled SQL cutoff.
	if !held.ExpiresAt.After(now()) {
		return LostErr
	}
	return nil
}

// Sweep quarantines every expired device lease and drops every expired fabric
// lock, whichever module took them. It is idempotent: every module's recovery
// runs it first, on every replica.
func Sweep(db *gorm.DB) error {
	t := now()
	if err := db.Where("device_id < 0 AND expires_at < ?", t).Delete(&Record{}).Error; err != nil {
		return err
	}
	return db.Model(&Record{}).Where("device_id > 0 AND expires_at < ? AND quarantined = ?", t, false).Update("quarantined", true).Error
}

// Free limits a query to rows whose device has no lease at all. column is a
// trusted column reference holding the device id, e.g. "sys_wireless_collection.controller_id".
func Free(column string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("NOT EXISTS (SELECT 1 FROM " + table + " lease WHERE lease.device_id = " + column + ")")
	}
}

// Held limits a query to rows whose device another piece of work holds live.
func Held(column string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("EXISTS (SELECT 1 FROM "+table+" lease WHERE lease.device_id = "+column+" AND lease.quarantined = ? AND lease.expires_at > ?)", false, now())
	}
}

// Quarantined limits a query to rows whose device is quarantined, counting an
// expired lease as quarantined since it can never be reused.
func Quarantined(column string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("EXISTS (SELECT 1 FROM "+table+" lease WHERE lease.device_id = "+column+" AND (lease.quarantined = ? OR lease.expires_at <= ?))", true, now())
	}
}
