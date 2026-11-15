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
	migration.Migrate.SetVersion(migration.GetFilename(file), migrateDeliveryAuditDetail)
}

// deliveryAuditDetailRow is the delivery audit table as this migration knows
// it: the two columns it adds. An audit row now says which scripts an action
// approved and, on the "finish" row, how the order ended.
type deliveryAuditDetailRow struct {
	ScriptsSHA256 string `gorm:"column:scripts_sha256;size:64"`
	Result        string `gorm:"size:40"`
}

func (deliveryAuditDetailRow) TableName() string { return "sys_delivery_audit" }

func migrateDeliveryAuditDetail(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := addDeliveryAuditDetail(tx); err != nil {
			return err
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}

// addDeliveryAuditDetail adds each column that is missing; a database
// created after this change already has them from the delivery migration.
func addDeliveryAuditDetail(db *gorm.DB) error {
	m := db.Migrator()
	if !m.HasTable(&deliveryAuditDetailRow{}) {
		return nil
	}
	for field, column := range map[string]string{"ScriptsSHA256": "scripts_sha256", "Result": "result"} {
		if m.HasColumn(&deliveryAuditDetailRow{}, column) {
			continue
		}
		if err := m.AddColumn(&deliveryAuditDetailRow{}, field); err != nil {
			return err
		}
	}
	return nil
}
