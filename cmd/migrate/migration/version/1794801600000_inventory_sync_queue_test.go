package version

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInventorySyncQueueColumnsAreAddedOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE sys_network_device_sync_state (id INTEGER PRIMARY KEY, status TEXT NOT NULL, device_count INTEGER NOT NULL DEFAULT 0)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO sys_network_device_sync_state (id, status) VALUES (1, 'succeeded')").Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := addInventorySyncQueueColumns(db); err != nil {
			t.Fatal(err)
		}
	}
	var row struct{ Imported, Updated, RequestedBy int }
	if err := db.Raw("SELECT imported, updated, requested_by FROM sys_network_device_sync_state WHERE id = 1").Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Imported != 0 || row.Updated != 0 || row.RequestedBy != 0 {
		t.Fatalf("existing row got %+v, want zeros", row)
	}
}
