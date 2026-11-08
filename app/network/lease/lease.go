// Package lease owns device exclusion. Operations retain the supplied database's
// transaction, context and logger; callers own transaction and cleanup budgets.
package lease

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

// Record preserves the established storage schema, including legacy coordinator rows.
type Record struct {
	DeviceID    int `gorm:"primaryKey;autoIncrement:false"`
	TargetID    int
	Token       string `gorm:"size:40"`
	ExpiresAt   time.Time
	Quarantined bool
}

func (Record) TableName() string { return "sys_delivery_device_lease" }

// Owner identifies a particular acquisition, not merely a device.
type Owner struct {
	DeviceID int
	Token    string
}
type Claim struct {
	Owner
	TargetID  int
	ExpiresAt time.Time
}

// TryAcquire never replaces an existing row, even after expiry.
func TryAcquire(db *gorm.DB, claim Claim) (bool, error) {
	q := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&Record{DeviceID: claim.DeviceID, Token: claim.Token, TargetID: claim.TargetID, ExpiresAt: claim.ExpiresAt})
	return q.RowsAffected == 1, q.Error
}

// Complete removes only a confirmed-closed owner. Missing or replaced owners are no-ops.
func Complete(db *gorm.DB, owner Owner, closed bool) error {
	q := db.Where("device_id = ? AND token = ?", owner.DeviceID, owner.Token)
	if closed {
		return q.Delete(&Record{}).Error
	}
	return q.Model(&Record{}).Update("quarantined", true).Error
}

// CheckOwner preserves Fabric's inclusive expiration check for ordinary reads.
func CheckOwner(db *gorm.DB, owner Owner, now func() time.Time) (bool, error) {
	var row Record
	if err := db.First(&row, "device_id = ? AND token = ?", owner.DeviceID, owner.Token).Error; err != nil {
		return false, err
	}
	return !row.Quarantined && !now().After(row.ExpiresAt), nil
}

// LockOwner requires a live owner strictly beyond at. The caller must supply its
// publication transaction so that the lock lasts through publication.
func LockOwner(db *gorm.DB, owner Owner, at time.Time) error {
	var row Record
	return db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("device_id = ? AND token = ? AND quarantined = ? AND expires_at > ?", owner.DeviceID, owner.Token, false, at).First(&row).Error
}
func LockDevice(db *gorm.DB, id int) (Record, error) {
	var row Record
	err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error
	return row, err
}

// QuarantineExpired retains the legacy whole-table sweep, including coordinator
// rows. Fabric separately reclaims its expired coordinators before this call.
func QuarantineExpired(db *gorm.DB, before time.Time) error {
	return db.Model(&Record{}).Where("expires_at < ?", before).Update("quarantined", true).Error
}
func OccupiedDevices(db *gorm.DB) *gorm.DB { return db.Model(&Record{}).Select("device_id") }
func ActiveDevices(db *gorm.DB, at time.Time) *gorm.DB {
	return OccupiedDevices(db).Where("quarantined = ? AND expires_at > ?", false, at)
}
