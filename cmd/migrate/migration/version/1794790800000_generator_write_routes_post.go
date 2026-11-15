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
	migration.Migrate.SetVersion(migration.GetFilename(file), migrateGeneratorWritesToPost)
}

// generatorWriteRoutes write source files, a migration and menu rows. They
// answered GET; the routes now answer POST, and the stored method of their
// API rows and grants follows, or roles holding them would lose access.
var generatorWriteRoutes = []string{
	"/api/v1/gen/toproject/:tableId",
	"/api/v1/gen/apitofile/:tableId",
	"/api/v1/gen/todb/:tableId",
}

func migrateGeneratorWritesToPost(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := moveGeneratorWritesToPost(tx); err != nil {
			return err
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}

func moveGeneratorWritesToPost(db *gorm.DB) error {
	for _, path := range generatorWriteRoutes {
		var posts int64
		if err := db.Table("sys_api").Where("path = ? AND action = 'POST'", path).Count(&posts).Error; err != nil {
			return err
		}
		if posts == 0 {
			if err := db.Exec("UPDATE sys_api SET action = 'POST', handle = ? WHERE path = ? AND action = 'GET'",
				"POST "+path, path).Error; err != nil {
				return err
			}
		}
		if err := db.Exec("UPDATE casbin_rule SET v2 = 'POST' WHERE ptype = 'p' AND v1 = ? AND v2 = 'GET'", path).Error; err != nil {
			return err
		}
	}
	return nil
}
