package version

import (
	"runtime"

	"gorm.io/gorm"

	"go-admin/app/admin/models"
	"go-admin/cmd/migrate/migration"
	common "go-admin/common/models"
)

func init() {
	_, fileName, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(fileName), _1786700013000AuthVersion)
}

// AddColumn uses the runtime field's portable non-null/default-zero definition.
// A retried migration preserves existing revocations and does not reset counters.
func _1786700013000AuthVersion(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if !tx.Migrator().HasColumn(&models.SysUser{}, "AuthVersion") {
			if err := tx.Migrator().AddColumn(&models.SysUser{}, "AuthVersion"); err != nil {
				return err
			}
		}
		if err := tx.Table("sys_user").Where("auth_version IS NULL").UpdateColumn("auth_version", 0).Error; err != nil {
			return err
		}
		return tx.Where("version = ?", version).FirstOrCreate(&common.Migration{Version: version}).Error
	})
}
