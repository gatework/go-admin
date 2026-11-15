package version

import (
	"fmt"
	"runtime"
	"strings"

	"gorm.io/gorm"

	"github.com/gatework/motor/cmd/migrate/migration"
	common "github.com/gatework/motor/common/models"
)

//nolint:gochecknoinits // Frozen migration registration must complete before the host seals its startup registry.
func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), migrateConfigKeyUnique)
}

// configKeyIndex makes sys_config.config_key unique among rows that are not
// deleted, as uk_sys_user_username and the other natural keys already are.
// Lookups by key (/configKey, the settings form, app-config) assume it.
const configKeyIndex = "uk_sys_config_config_key"

func migrateConfigKeyUnique(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := addConfigKeyIndex(tx); err != nil {
			return err
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}

// addConfigKeyIndex refuses before altering anything when active rows share a
// key, naming them, so the operator knows exactly what to reconcile.
func addConfigKeyIndex(db *gorm.DB) error {
	var duplicates []string
	if err := db.Table("sys_config").Select("config_key").Where("deleted_at = 0").
		Group("config_key").Having("COUNT(*) > 1").Pluck("config_key", &duplicates).Error; err != nil {
		return fmt.Errorf("checking sys_config for duplicate keys: %w", err)
	}
	if len(duplicates) > 0 {
		return fmt.Errorf("sys_config has active rows sharing a config_key (%s); delete or rename the extras, then run the migration again",
			strings.Join(duplicates, ", "))
	}
	if db.Migrator().HasIndex("sys_config", configKeyIndex) {
		return nil
	}
	return db.Exec("CREATE UNIQUE INDEX " + configKeyIndex + " ON sys_config (config_key, deleted_at)").Error
}
