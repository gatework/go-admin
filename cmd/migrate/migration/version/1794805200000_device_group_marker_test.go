package version

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDeviceGroupMarkerIsAddedAndStandardGroupsKeepTheirRule(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE sys_network_device_group (id INTEGER PRIMARY KEY, code TEXT NOT NULL, name TEXT NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO sys_network_device_group (id, code, name) VALUES (1, 'site.shanghai', '上海职场'), (2, 'site.other', '其他区域'), (3, 'custom.lab', 'Lab')").Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := addDeviceGroupMarker(db); err != nil {
			t.Fatal(err)
		}
	}
	var markers []string
	if err := db.Raw("SELECT marker FROM sys_network_device_group ORDER BY id").Scan(&markers).Error; err != nil {
		t.Fatal(err)
	}
	if len(markers) != 3 || markers[0] != "sha" || markers[1] != "" || markers[2] != "" {
		t.Fatalf("markers %q; want only the standard site to get one", markers)
	}
}
