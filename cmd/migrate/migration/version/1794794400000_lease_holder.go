package version

import (
	"runtime"

	"gorm.io/gorm"

	"github.com/gatework/motor/cmd/migrate/migration"
	common "github.com/gatework/motor/common/models"
)

//nolint:gochecknoinits // Frozen migration registration must complete before the host seals its startup registry.
func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), migrateLeaseHolder)
}

// leaseHolderRow is the lease table as this migration knows it: only the
// column it adds. It names the job holding a lease ("fabric.change:12"), which
// target_id alone could not, since every capability's job IDs overlap.
type leaseHolderRow struct {
	Holder string `gorm:"size:64"`
}

func (leaseHolderRow) TableName() string { return "sys_delivery_device_lease" }

func migrateLeaseHolder(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := addLeaseHolder(tx); err != nil {
			return err
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}

func addLeaseHolder(db *gorm.DB) error {
	if db.Migrator().HasColumn(&leaseHolderRow{}, "holder") {
		return nil
	}
	return db.Migrator().AddColumn(&leaseHolderRow{}, "Holder")
}
