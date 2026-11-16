package version

import (
	"runtime"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/gatework/motor/cmd/migrate/migration"
	common "github.com/gatework/motor/common/models"
)

//nolint:gochecknoinits // Frozen migration registration must complete before the host seals its startup registry.
func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), accountAccessMigration)
}

// The account tables as this migration created them (commit 35b0d6c1),
// frozen so that later changes to common/account cannot change what it does.
type (
	accountAccessState struct {
		UserID            int    `gorm:"primaryKey;autoIncrement:false"`
		Secret            string `gorm:"size:512"`
		PendingCredential string `gorm:"size:64"`
		PendingUntil      int64
		Enabled           bool
		LastStep          int64
		Version           int64
	}
	accountAccessChallenge struct {
		Hash       string `gorm:"primaryKey;size:64"`
		Kind       string `gorm:"size:16;index"`
		Email      string `gorm:"size:128;index"`
		UserID     int
		CodeHash   string `gorm:"size:64"`
		Credential string `gorm:"size:64"`
		ExpiresAt  int64
		Attempts   int
		Used       bool
	}
	accountAccessRecovery struct {
		Hash   string `gorm:"primaryKey;size:64"`
		UserID int    `gorm:"index"`
	}
	accountAccessLimit struct {
		Key       string `gorm:"primaryKey;size:80"`
		Count     int
		ExpiresAt int64
	}
	accountAccessPolicy struct {
		ID      int `gorm:"primaryKey;autoIncrement:false"`
		Enabled bool
	}
)

func (accountAccessState) TableName() string     { return "sys_account_security" }
func (accountAccessChallenge) TableName() string { return "sys_account_challenge" }
func (accountAccessRecovery) TableName() string  { return "sys_account_recovery" }
func (accountAccessLimit) TableName() string     { return "sys_account_limit" }
func (accountAccessPolicy) TableName() string    { return "sys_account_policy" }

func accountAccessMigration(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if e := tx.AutoMigrate(&accountAccessState{}, &accountAccessChallenge{}, &accountAccessRecovery{}, &accountAccessLimit{}, &accountAccessPolicy{}); e != nil {
			return e
		}
		if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&accountAccessPolicy{ID: 1, Enabled: false}).Error; e != nil {
			return e
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}
