// Package txlock serializes work across processes with named locks that last
// as long as a database transaction.
//
// It is for short critical sections inside one transaction: counting running
// jobs before admitting another, or rewriting a hierarchy. Work that outlives
// a transaction - a session to a device - takes a device lease instead (see
// app/net/models/lease and GLOSSARY.md).
package txlock

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Lock names. Each names one critical section; two sections never share one.
const (
	// DeviceGroups guards the device group hierarchy and memberships.
	DeviceGroups = "inventory.device_groups"
	// FirewallCollectionAdmission guards the count of running firewall
	// collections against the concurrency limit.
	FirewallCollectionAdmission = "firewall.collection_admission"
)

// Row is one named lock.
type Row struct {
	Name    string `gorm:"primaryKey;size:64"`
	Version int64  `gorm:"not null;default:0"`
}

// TableName returns the stable database table for Row.
func (Row) TableName() string { return "sys_network_lock" }

// Hold takes the named lock for the rest of tx: another transaction holding
// the same name waits until tx commits or rolls back. It writes the row, so it
// works the same on PostgreSQL, MySQL and SQLite.
func Hold(tx *gorm.DB, name string) error {
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&Row{Name: name}).Error; err != nil {
		return err
	}
	return tx.Model(&Row{}).Where("name = ?", name).Update("version", gorm.Expr("version + 1")).Error
}
