package version

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/gatework/motor/cmd/migrate/migration"
	common "github.com/gatework/motor/common/models"
)

// A fresh install runs every registered migration, in order, on an empty
// database, as `migrate` does. Each migration's own test starts from a schema
// it builds by hand, so only this catches a migration that depends on a table
// a later one creates - which is easy to do when a frozen migration calls
// runtime code.
func TestEveryMigrationRunsOnAnEmptyDatabase(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "fresh.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), TranslateError: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&common.Migration{}); err != nil {
		t.Fatal(err)
	}
	createCasbinRule(t, db)
	previous := migration.Migrate.GetDB()
	migration.Migrate.SetDB(db)
	t.Cleanup(func() { migration.Migrate.SetDB(previous) })

	if err := migration.Migrate.Migrate(); err != nil {
		t.Fatalf("fresh install: %v", err)
	}
	var applied int64
	if err := db.Model(&common.Migration{}).Count(&applied).Error; err != nil {
		t.Fatal(err)
	}
	if registered := len(migration.Migrate.RegisteredVersions()); int(applied) != registered {
		t.Fatalf("%d of %d migrations recorded", applied, registered)
	}
	if err := migration.Migrate.Migrate(); err != nil {
		t.Fatalf("second run on an up-to-date database: %v", err)
	}
}
