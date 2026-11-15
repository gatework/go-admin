package version

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLeaseHolderColumnIsAddedOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE sys_delivery_device_lease (device_id INTEGER PRIMARY KEY, target_id INTEGER, token TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := addLeaseHolder(db); err != nil {
			t.Fatal(err)
		}
	}
	if !db.Migrator().HasColumn(&leaseHolderRow{}, "holder") {
		t.Fatal("holder column missing")
	}
}
