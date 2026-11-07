package version

import (
	"gorm.io/driver/postgres"
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	common "go-admin/common/models"
	"gorm.io/gorm"
)

func TestAuthVersionMigrationBackfillsAndPreservesRevocations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	checkAuthVersionMigration(t, db)
}

func TestPostgresAuthVersionMigrationUpgrade(t *testing.T) {
	dsn := os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GO_ADMIN_TEST_POSTGRES_DSN is required")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.Close() })
	if err := db.Exec("DROP SCHEMA IF EXISTS auth_migration_test CASCADE").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE SCHEMA auth_migration_test").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("SET search_path TO auth_migration_test").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Exec("DROP SCHEMA auth_migration_test CASCADE").Error; err != nil {
			t.Error(err)
		}
	})
	checkAuthVersionMigration(t, db)
}

func checkAuthVersionMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec("CREATE TABLE sys_user (user_id integer primary key, username text)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO sys_user (user_id,username) VALUES (1,'existing')").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&common.Migration{}); err != nil {
		t.Fatal(err)
	}
	const version = "1786700013000"
	if err := _1786700013000AuthVersion(db, version); err != nil {
		t.Fatal(err)
	}
	var value int64
	if err := db.Table("sys_user").Select("auth_version").Where("user_id = 1").Scan(&value).Error; err != nil {
		t.Fatal(err)
	}
	if value != 0 {
		t.Fatalf("backfill=%d", value)
	}
	if err := db.Exec("UPDATE sys_user SET auth_version=7 WHERE user_id=1").Error; err != nil {
		t.Fatal(err)
	}
	if err := _1786700013000AuthVersion(db, version); err != nil {
		t.Fatal(err)
	}
	if err := db.Table("sys_user").Select("auth_version").Where("user_id = 1").Scan(&value).Error; err != nil {
		t.Fatal(err)
	}
	if value != 7 {
		t.Fatalf("rerun reset revocation=%d", value)
	}
	var count int64
	if err := db.Model(&common.Migration{}).Where("version = ?", version).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migration rows=%d", count)
	}
}
