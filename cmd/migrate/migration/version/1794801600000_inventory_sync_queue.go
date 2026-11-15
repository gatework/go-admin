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
	migration.Migrate.SetVersion(migration.GetFilename(file), migrateInventorySyncQueue)
}

// inventorySyncQueueRow is the sync state table as this migration knows it:
// only the columns it adds. A sync is now queued by a user and run by a
// background worker, so the row records who asked and what the run changed.
type inventorySyncQueueRow struct {
	Imported    int `gorm:"not null;default:0"`
	Updated     int `gorm:"not null;default:0"`
	RequestedBy int `gorm:"not null;default:0"`
}

func (inventorySyncQueueRow) TableName() string { return "sys_network_device_sync_state" }

func migrateInventorySyncQueue(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := addInventorySyncQueueColumns(tx); err != nil {
			return err
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}

// addInventorySyncQueueColumns adds each column that is missing. A database
// created after this change already has them, because the inventory migration
// builds the table from the runtime model.
func addInventorySyncQueueColumns(db *gorm.DB) error {
	m := db.Migrator()
	for field, column := range map[string]string{"Imported": "imported", "Updated": "updated", "RequestedBy": "requested_by"} {
		if m.HasColumn(&inventorySyncQueueRow{}, column) {
			continue
		}
		if err := m.AddColumn(&inventorySyncQueueRow{}, field); err != nil {
			return err
		}
	}
	return nil
}
