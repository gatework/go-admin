package version

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestWirelessResourceSearchIsAddedAndBackfilled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE sys_wireless_resource (id INTEGER PRIMARY KEY, facts TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO sys_wireless_resource (id, facts) VALUES (1, '{"name":"Lab-AP","ip_address":"10.2.0.12","mac_address":"AABB.ccdd.EEFF","operational_status":"run"}'), (2, '')`).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := addWirelessResourceSearch(db); err != nil {
			t.Fatal(err)
		}
	}
	var row wirelessSearchRow
	if err := db.First(&row, 1).Error; err != nil {
		t.Fatal(err)
	}
	if row.SearchIP != "10.2.0.12" || row.SearchMAC != "aabb.ccdd.eeff" || row.SearchAP != "lab-ap" || row.SearchStatus != "run" || row.SearchName != "Lab-AP" || row.SearchText == "" {
		t.Fatalf("backfilled %+v", row)
	}
}
