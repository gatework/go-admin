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
	migration.Migrate.SetVersion(migration.GetFilename(file), migrateNetworkLock)
}

// networkLockRow is the named transaction lock table as this migration
// creates it. The device group hierarchy and firewall collection admission
// take their locks here; admission used to lock the first firewall domain row,
// and failed outright when no domain existed.
type networkLockRow struct {
	Name    string `gorm:"primaryKey;size:64"`
	Version int64  `gorm:"not null;default:0"`
}

func (networkLockRow) TableName() string { return "sys_network_lock" }

func migrateNetworkLock(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&networkLockRow{}); err != nil {
			return err
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}
