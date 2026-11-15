package connection

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestLoadManyGivesEveryDeviceItsSettingsOrTheDefaults(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Settings{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Settings{DeviceID: 2, Profile: "h3c", Protocol: "telnet", Port: 23, Version: 3}).Error; err != nil {
		t.Fatal(err)
	}
	got, err := LoadMany(context.Background(), db, []int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if c := got[1]; c.DeviceID != 1 || c.Protocol != "ssh" || c.Port != 22 || c.Profile != "" || c.Version != 0 {
		t.Errorf("unconfigured device: %+v, want the defaults", c)
	}
	if c := got[2]; c.Protocol != "telnet" || c.Port != 23 || c.Profile != "h3c" || c.Version != 3 {
		t.Errorf("configured device: %+v", c)
	}
	for _, id := range []int{1, 2} {
		one, err := Load(context.Background(), db, id)
		if err != nil || one != got[id] {
			t.Errorf("Load(%d) = %+v %v; LoadMany gave %+v", id, one, err, got[id])
		}
	}
	if none, err := LoadMany(context.Background(), db, nil); err != nil || len(none) != 0 {
		t.Errorf("no IDs: %v %v", none, err)
	}
}
