// Package lease owns device leases and fabric locks for every network module
// (see GLOSSARY.md): which piece of work may open sessions to a device, and
// which device is quarantined because a session ended without a confirmed
// close.
//
// The rules live here and nowhere else:
//
//   - A device has at most one lease. Taking one never waits: a held lease
//     answers BusyErr at once, so holding several (a fabric lock, then device
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
// transaction. When Acquire answers QuarantinedErr after quarantining an
// expired lease, that write is part of the caller's transaction; if the caller
// rolls back, the next Sweep quarantines the lease again.
package lease

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	errs "github.com/gatework/motor/common/errors"
)

// Margin is added to every run timeout, so a lease outlives the work it guards.
const Margin = 5 * time.Minute

//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname // Project sentinel errors use Err suffix; see docs/standards.md.
var (
	// BusyErr is a device or fabric another piece of work holds. Try again later.
	BusyErr = errs.New(errs.KindUnavailable, "lease.busy", "")
	// QuarantinedErr is a device that may hold an unconfirmed change and takes
	// no new work until an operator releases it.
	QuarantinedErr = errs.New(errs.KindConflict, "lease.quarantined", "")
	// LostErr is a lease the caller no longer holds: it expired, was
	// quarantined, or was never taken.
	LostErr = errs.New(errs.KindConflict, "lease.lost", "")
	// InvalidErr is a device or fabric ID that cannot be leased.
	InvalidErr = errs.New(errs.KindInvalid, "lease.invalid", "")
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

// Holder names the job holding a lease, so that an operator reading the lease
// table (for instance before clearing a quarantine) can find it. Kind names
// the job's type, ID its row.
type Holder struct {
	Kind string
	ID   int
}

// Holder kinds, one per kind of job that takes leases.
const (
	DeliveryTarget       = "delivery.target"
	FabricCollection     = "fabric.collection"
	FabricCollectionNode = "fabric.collection.target"
	FabricChange         = "fabric.change"
	FabricChangeSource   = "fabric.change.source"
	FirewallCollection   = "firewall.collection"
	WirelessCollection   = "wireless.collection"
)

// String is the holder as recorded, e.g. "fabric.change:12".
func (h Holder) String() string { return fmt.Sprintf("%s:%d", h.Kind, h.ID) }

// Record is the stored lease row. The table keeps its original name; only the
// migration and this package refer to it.
type Record struct {
	DeviceID int `gorm:"primaryKey;autoIncrement:false"`
	// HolderID is the holder's row, in the column first named for delivery
	// targets; Holder says whose row it is.
	HolderID    int    `gorm:"column:target_id"`
	Holder      string `gorm:"size:64"`
	Token       string `gorm:"size:40"`
	ExpiresAt   time.Time
	Quarantined bool
}

// TableName returns the stable database table for Record.
func (Record) TableName() string { return table }

const table = "sys_delivery_device_lease"

// now is the module's one clock: UTC from the Go process, replaceable in tests.
var now = func() time.Time { return time.Now().UTC() }

// Acquire takes the lease on a device for work that runs at most run,
// recording holder for an operator reading the table.
func Acquire(db *gorm.DB, deviceID int, holder Holder, run time.Duration) (Lease, error) {
	if deviceID <= 0 {
		return Lease{}, InvalidErr
	}
	return take(db, deviceID, holder, run, false)
}

// LockFabric takes the fabric lock that serialises coordination of one Fabric.
// Release it with NoSession.
func LockFabric(db *gorm.DB, fabricID int, holder Holder, run time.Duration) (Lease, error) {
	if fabricID <= 0 {
		return Lease{}, InvalidErr
	}
	// Fabric locks share the device table under negative keys, which no
	// inventory device can have.
	return take(db, -fabricID, holder, run, true)
}

func take(db *gorm.DB, key int, holder Holder, run time.Duration, fabric bool) (Lease, error) {
	l := Lease{DeviceID: key, Token: uuid.NewString(), ExpiresAt: now().Add(run + Margin)}
	q := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&Record{DeviceID: key, HolderID: holder.ID, Holder: holder.String(), Token: l.Token, ExpiresAt: l.ExpiresAt})
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
		q = db.Clauses(clause.OnConflict{DoNothing: true}).Create(&Record{DeviceID: key, HolderID: holder.ID, Holder: holder.String(), Token: l.Token, ExpiresAt: l.ExpiresAt})
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
// answers LostErr when the caller no longer holds the lease, and then changes
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
