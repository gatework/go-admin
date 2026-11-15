package firewall

import (
	"context"
	"errors"
	"testing"

	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/inventory"
)

func collectionInstance(t *testing.T, s *Firewall, ip string) models.Instance {
	t.Helper()
	id := device(t, s, 7, ip)
	i, err := s.CreateInstance(context.Background(), InstanceInput{DeviceID: id, DomainID: 1, Vendor: "hillstone", Mode: "routed"}, all())
	if err != nil {
		t.Fatal(err)
	}
	return i
}

func TestCollectionIdempotencyAndPartialAdmission(t *testing.T) {
	s := testService(t)
	collectionCredentials(t, s)
	ctx := context.Background()
	a, b := collectionInstance(t, s, "192.0.2.11"), collectionInstance(t, s, "192.0.2.12")
	if err := s.DB.Model(&inventory.Device{}).Where("id = ?", b.DeviceID).Update("status", "retired").Error; err != nil {
		t.Fatal(err)
	}
	in := CollectionInput{RequestKey: "same", Kind: "capture", InstanceIDs: []int{a.ID, b.ID}}
	first, err := s.EnqueueCollection(ctx, in, all())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Targets) != 2 || first.Targets[0].Status != "queued" || first.Targets[1].ErrorCode != "device_unavailable" {
		t.Fatalf("lost per-device result: %+v", first)
	}
	again, err := s.EnqueueCollection(ctx, in, all())
	if err != nil || again.ID != first.ID {
		t.Fatalf("not idempotent: %+v %v", again, err)
	}
	in.Kind = "connection_test"
	if _, err = s.EnqueueCollection(ctx, in, all()); !errors.Is(err, ConflictErr) {
		t.Fatalf("key reused for different work: %v", err)
	}
	cancelled, err := s.CancelCollection(ctx, first.ID, all())
	if err != nil || cancelled.Targets[0].Status != "cancelled" {
		t.Fatalf("cancel: %+v %v", cancelled, err)
	}
	s.Actor = 8
	if _, err = s.Collection(ctx, first.ID, all()); !errors.Is(err, NotFoundErr) {
		t.Fatalf("other actor read batch: %v", err)
	}
}

func TestCollectionRejectsUnsafeContextAndPayload(t *testing.T) {
	s := testService(t)
	collectionCredentials(t, s)
	ctx := context.Background()
	i := collectionInstance(t, s, "192.0.2.13")
	if err := s.DB.Model(&i).Update("native_context", "vsys2").Error; err != nil {
		t.Fatal(err)
	}
	b, err := s.EnqueueCollection(ctx, CollectionInput{RequestKey: "context", Kind: "capture", InstanceIDs: []int{i.ID}}, all())
	if err != nil || b.Targets[0].ErrorCode != "context_unsupported" {
		t.Fatalf("unsafe context: %+v %v", b, err)
	}
	for _, in := range []CollectionInput{{RequestKey: "bad", Kind: "apply", InstanceIDs: []int{i.ID}}, {RequestKey: "duplicate", Kind: "capture", InstanceIDs: []int{i.ID, i.ID}}, {RequestKey: "empty", Kind: "capture"}} {
		if _, err = s.EnqueueCollection(ctx, in, all()); !errors.Is(err, InvalidErr) {
			t.Fatalf("invalid accepted: %+v %v", in, err)
		}
	}
}
