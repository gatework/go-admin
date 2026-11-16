package service

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/gatework/motor/app/net/service/netconfig"
)

// Every physical tenant database gets one complete worker set; tenants that
// share a database share its workers, and a tenant without one gets none.
func TestWorkersBuildOneCompleteSetPerDatabase(t *testing.T) {
	t.Setenv("FIREWALL_SNAPSHOT_DIR", t.TempDir())
	open := func(name string) *gorm.DB {
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), name)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
		if err != nil {
			t.Fatal(err)
		}
		return db
	}
	shared, other := open("shared.db"), open("other.db")
	workers := Workers(map[string]*gorm.DB{"b.example": shared, "a.example": shared, "c.example": other, "none.example": nil}, netconfig.Config{})

	want := []string{"fabric", "wireless", "delivery", "firewall offline", "inventory sync", "firewall collection"}
	if len(workers) != 2*len(want) {
		t.Fatalf("%d workers, want %d (two databases)", len(workers), 2*len(want))
	}
	for set := range 2 {
		names := []string{}
		for _, w := range workers[set*len(want) : (set+1)*len(want)] {
			if w.Run == nil {
				t.Errorf("worker %q has no Run", w.Name)
			}
			names = append(names, w.Name)
		}
		if !slices.Equal(names, want) {
			t.Errorf("set %d is %v, want %v", set, names, want)
		}
	}
}
