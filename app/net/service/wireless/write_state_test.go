package wireless

import (
	"errors"
	"testing"
	"time"

	models "github.com/gatework/motor/app/net/models/wireless"
)

// GORM Save upserts, so a state row deleted after it was read came back on the
// next write. writeState must report that, and a stale version, as a conflict.
func TestWriteStateNeverResurrectsOrOverwritesStaleRows(t *testing.T) {
	s, _ := setup(t)
	db := s.DB
	now := time.Now().UTC()

	state := models.NeighborState{ControllerID: 1, APResourceID: 9, APName: "ap", Version: 1, Status: "partial", ObservedAt: now}
	if err := writeState(db, &state, true, 0); err != nil {
		t.Fatal(err)
	}

	fresh := state
	fresh.Version, fresh.Status = 2, "complete"
	if err := writeState(db, &fresh, false, 1); err != nil {
		t.Fatal(err)
	}
	var stored models.NeighborState
	if err := db.First(&stored, state.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Version != 2 || stored.Status != "complete" {
		t.Fatalf("update not applied: %+v", stored)
	}

	stale := state
	stale.Version, stale.Status = 5, "partial"
	if err := writeState(db, &stale, false, 1); !errors.Is(err, ConflictErr) {
		t.Fatalf("stale version accepted: %v", err)
	}

	if err := db.Delete(&models.NeighborState{}, state.ID).Error; err != nil {
		t.Fatal(err)
	}
	gone := fresh
	gone.Version = 3
	if err := writeState(db, &gone, false, 2); !errors.Is(err, ConflictErr) {
		t.Fatalf("write to a deleted row accepted: %v", err)
	}
	var count int64
	if err := db.Model(&models.NeighborState{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("deleted row was written back")
	}

	// Zero values must be written too: an update that skips them would keep a
	// status the collector has just cleared.
	blank := models.NeighborState{ControllerID: 1, APResourceID: 10, Version: 1, Status: "complete", ObservedAt: now}
	if err := writeState(db, &blank, true, 0); err != nil {
		t.Fatal(err)
	}
	blank.Status = ""
	if err := writeState(db, &blank, false, 1); err != nil {
		t.Fatal(err)
	}
	var cleared models.NeighborState
	if err := db.First(&cleared, blank.ID).Error; err != nil {
		t.Fatal(err)
	}
	if cleared.Status != "" {
		t.Fatalf("zero value skipped: %q", cleared.Status)
	}
}
