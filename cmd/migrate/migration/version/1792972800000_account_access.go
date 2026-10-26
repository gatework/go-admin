package version

import (
	"go-admin/cmd/migrate/migration"
	"go-admin/common/account"
	common "go-admin/common/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"runtime"
)

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
