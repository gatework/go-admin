package version

import (
	"runtime"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/gatework/motor/cmd/migrate/migration"
	"github.com/gatework/motor/common/account"
	common "github.com/gatework/motor/common/models"
)

//nolint:gochecknoinits // Frozen migration registration must complete before the host seals its startup registry.
func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), accountAccessMigration)
}
func accountAccessMigration(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if e := tx.AutoMigrate(&account.State{}, &account.Challenge{}, &account.Recovery{}, &account.Limit{}, &account.Policy{}); e != nil {
			return e
		}
		if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&account.Policy{ID: 1, Enabled: false}).Error; e != nil {
			return e
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}
