package version

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	common "github.com/gatework/motor/common/models"
)

func TestNetworkLockTableIsCreated(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&common.Migration{}); err != nil {
		t.Fatal(err)
	}
	if err := migrateNetworkLock(db, "1794812400000"); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable("sys_network_lock") {
		t.Fatal("sys_network_lock missing")
	}
}
