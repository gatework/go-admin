package version

import (
	"runtime"
	"time"

	"gorm.io/gorm"

	"github.com/gatework/motor/cmd/migrate/migration"
	common "github.com/gatework/motor/common/models"
)

//nolint:gochecknoinits // Frozen migration registration must complete before the host seals its startup registry.
func init() {
	_, file, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(file), migrateDiagnosticRun)
}

// diagnosticRunRow is the diagnostic audit table as this migration creates
// it. Diagnostic jobs live only in process memory; this table records who ran
// which tool against which target.
type diagnosticRunRow struct {
	ID        int    `gorm:"primaryKey"`
	UserID    int    `gorm:"index"`
	JobID     string `gorm:"size:32"`
	Tool      string `gorm:"size:32"`
	Target    string `gorm:"size:255;index"`
	Family    string `gorm:"size:8"`
	Mode      string `gorm:"size:16"`
	Port      int
	Command   string    `gorm:"type:text"`
	StartedAt time.Time `gorm:"index"`
}

func (diagnosticRunRow) TableName() string { return "sys_diagnostic_run" }

func migrateDiagnosticRun(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.AutoMigrate(&diagnosticRunRow{}); err != nil {
			return err
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}
