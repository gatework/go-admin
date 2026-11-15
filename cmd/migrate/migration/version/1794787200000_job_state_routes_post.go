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
	migration.Migrate.SetVersion(migration.GetFilename(file), migrateJobStateRoutesToPost)
}

// jobStateRoutes start and stop a scheduled job. They changed state over GET;
// the routes now answer POST, so the stored method of every grant follows, or
// each role holding them would lose access.
var jobStateRoutes = []string{"/api/v1/job/remove/:id", "/api/v1/job/start/:id"}

func migrateJobStateRoutesToPost(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := moveJobStateRoutesToPost(tx); err != nil {
			return err
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}

func moveJobStateRoutesToPost(db *gorm.DB) error {
	for _, path := range jobStateRoutes {
		// A POST row already present (registered by a newer process before this
		// ran) is kept; the GET one would only shadow it.
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
