package version

import (
	"runtime"
	"strings"

	"gorm.io/gorm"

	"github.com/gatework/motor/cmd/migrate/migration"
	common "github.com/gatework/motor/common/models"
)

//nolint:gochecknoinits // Frozen migration registration must complete before the host seals its startup registry.
func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), migrateMotorIdentity)
}

func migrateMotorIdentity(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := normalizeMotorIdentity(tx); err != nil {
			return err
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}

// Normalize shipped identities after frozen migrations have seeded their rows.
// Custom application handlers and user-selected product names are untouched.
func normalizeMotorIdentity(db *gorm.DB) error {
	var rows []struct {
		ID     int
		Handle string
		Action string
		Path   string
	}
	if err := db.Table("sys_api").Select("id", "handle", "action", "path").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		handle := row.Handle
		for _, prefix := range []string{"go-admin/", "motor/"} {
			if after, ok := strings.CutPrefix(handle, prefix); ok {
				handle = "github.com/gatework/motor/" + after
				break
			}
		}
		if strings.HasPrefix(handle, "github.com/gatework/motor/") && row.Action != "" && row.Path != "" {
			handle = strings.ToUpper(row.Action) + " " + row.Path
		}
		if handle != row.Handle {
			if err := db.Table("sys_api").Where("id = ?", row.ID).Update("handle", handle).Error; err != nil {
				return err
			}
		}
	}
	return db.Table("sys_config").Where("config_key = ? AND config_value IN ?", "sys_app_name", []string{"go-admin", "go-admin管理系统", "motor管理系统"}).Update("config_value", "motor").Error
}
