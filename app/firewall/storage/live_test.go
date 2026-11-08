package storage_test

import (
	"context"
	"testing"

	"github.com/gatework/firemon/projection"
	"go-admin/app/firewall/models"
	"go-admin/app/firewall/storage"
)

// A live partial capture is an authoritative new observation, unlike a partial
// offline import. The SQL projection must follow its native outbox generation.
func TestLiveIncompleteProjectionReplacesCurrentGeneration(t *testing.T) {
	s, db := setup(t)
	ctx := context.Background()
	previous, err := s.Import(ctx, 1, graph(t, false), storage.ImportOptions{PublicationID: "previous"})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&models.Coverage{InstanceID: 1, Revision: previous.Revision, Network: "10.0.0.0/24", Known: true}).Error; err != nil {
		t.Fatal(err)
	}
	partial := graph(t, true)
	archived, err := s.Import(ctx, 1, partial, storage.ImportOptions{PublicationID: "partial-archive"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Apply(ctx, projection.Batch{Device: storage.DeviceKey(1), Revision: archived.Revision, Generation: previous.Generation + 1, Complete: false, Firewall: partial}); err != nil {
		t.Fatal(err)
	}
	var instance models.Instance
	if err = db.First(&instance, 1).Error; err != nil {
		t.Fatal(err)
	}
	if instance.CurrentRevision != archived.Revision {
		t.Fatal("live incomplete capture left the old complete snapshot current")
	}
	var current []models.Snapshot
	if err = db.Where("current = ?", true).Find(&current).Error; err != nil {
		t.Fatal(err)
	}
	if len(current) != 1 || current[0].Complete || current[0].Revision != archived.Revision {
		t.Fatalf("invalid current evidence: %+v", current)
	}
	var count int64
	if err = db.Model(&models.Coverage{}).Where("instance_id = ?", 1).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("old coverage remained current")
	}
	if err = s.Apply(ctx, projection.Batch{Device: storage.DeviceKey(1), Revision: previous.Revision, Generation: previous.Generation, Complete: true, Firewall: graph(t, false)}); err != nil {
		t.Fatal(err)
	}
	if err = db.First(&instance, 1).Error; err != nil {
		t.Fatal(err)
	}
	if instance.CurrentRevision != archived.Revision {
		t.Fatal("older generation restored complete state")
	}
}
