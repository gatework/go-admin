package fabric

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gatework/motor/app/net/models/lease"

	"gorm.io/gorm"

	"github.com/gatework/motor/app/net/models/delivery"
	models "github.com/gatework/motor/app/net/models/fabric"
	network "github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
)

func TestRecoveryTerminalizesHistoricalTasksAfterMembershipChanges(t *testing.T) {
	for _, membership := range []string{"moved", "unassigned", "removed"} {
		for _, kind := range []string{"collection", "change", "stop_change"} {
			t.Run(membership+"/"+kind, func(t *testing.T) {
				s := operationsFixture(t)
				ctx := context.Background()
				past := time.Now().UTC().Add(-time.Minute)
				fake := &fakeFabricTransport{bodies: map[int]string{1: gatewayConfig}}
				w := NewWorker(s, fake)
				w.lastSchedule = time.Now()
				var job models.Collection
				var change models.Change
				deviceID := 2
				if kind == "collection" {
					var err error
					job, err = s.CreateCollection(ctx, 1, &dto.CollectionCreateReq{RequestKey: "recovery-collection", Mode: "configuration"}, 7, operationsScope())
					if err != nil {
						t.Fatal(err)
					}
					if err := s.DB.Model(&job).Updates(map[string]any{"status": "running", "claim_until": past}).Error; err != nil {
						t.Fatal(err)
					}
				} else {
					change = previewGateway(t, s)
					change.Targets[0].Status = "running"
					change.Targets[0].Attempted = true
					if err := s.DB.Model(&change).Updates(map[string]any{"status": "running", "targets": jsonText(change.Targets), "claim_until": past}).Error; err != nil {
						t.Fatal(err)
					}
				}
				if err := s.DB.Create(&models.Fabric{ID: 2, Code: "OTHER", Version: 1, RulesVersion: 1}).Error; err != nil {
					t.Fatal(err)
				}
				switch membership {
				case "moved":
					if err := s.DB.Model(&models.Membership{}).Where("device_id = ?", deviceID).Updates(map[string]any{"fabric_id": 2, "mode": "manual"}).Error; err != nil {
						t.Fatal(err)
					}
				case "unassigned":
					if err := s.DB.Model(&models.Membership{}).Where("device_id = ?", deviceID).Updates(map[string]any{"fabric_id": nil, "status": "unassigned", "mode": "manual"}).Error; err != nil {
						t.Fatal(err)
					}
				case "removed":
					if err := s.DB.Delete(&models.Membership{}, "device_id = ?", deviceID).Error; err != nil {
						t.Fatal(err)
					}
				}
				if err := s.DB.Create(&delivery.DeviceLease{DeviceID: deviceID, Token: "lost", ExpiresAt: past}).Error; err != nil {
					t.Fatal(err)
				}
				var before int64
				if err := s.DB.Model(&models.Observation{}).Where("device_id = ?", deviceID).Count(&before).Error; err != nil {
					t.Fatal(err)
				}
				if kind == "stop_change" {
					if err := w.stopChange(ctx, &change, "source_changed"); err != nil {
						t.Fatal(err)
					}
				}
				if err := w.Recover(ctx); err != nil {
					t.Fatal(err)
				}
				if kind == "collection" {
					if err := s.DB.First(&job, job.ID).Error; err != nil {
						t.Fatal(err)
					}
					if job.Status != "interrupted" || job.Targets[1].ErrorCode != "worker_lost" || job.FinishedAt == nil {
						t.Fatalf("collection not terminal: %+v", job)
					}
				} else {
					if err := s.DB.First(&change, change.ID).Error; err != nil {
						t.Fatal(err)
					}
					if change.Status != "uncertain" || change.Targets[0].Status != "uncertain" || !change.Targets[0].Attempted || change.Targets[1].Status != "cancelled" || change.FinishedAt == nil {
						t.Fatalf("change not terminal: %+v", change)
					}
				}
				var after int64
				if err := s.DB.Model(&models.Observation{}).Where("device_id = ?", deviceID).Count(&after).Error; err != nil {
					t.Fatal(err)
				}
				if after != before {
					t.Fatal("historical failure wrote evidence after membership changed")
				}
				var lease delivery.DeviceLease
				if err := s.DB.First(&lease, "device_id = ?", deviceID).Error; err != nil {
					t.Fatal(err)
				}
				if !lease.Quarantined {
					t.Fatal("unknown transport lease not quarantined")
				}
				if len(fake.opens) != 0 || len(fake.writes) != 0 {
					t.Fatal("recovery opened transport or replayed write")
				}
				// An unrelated Fabric must still be runnable on the next Tick.
				if err := s.DB.Model(&models.Membership{}).Where("device_id = ?", 1).Updates(map[string]any{"fabric_id": 2, "mode": "manual"}).Error; err != nil {
					t.Fatal(err)
				}
				var device network.Device
				if err := s.DB.First(&device, 1).Error; err != nil {
					t.Fatal(err)
				}
				other, err := s.createCollection(ctx, 2, &dto.CollectionCreateReq{RequestKey: "other-fabric-collection", Mode: "configuration"}, 7, []network.Device{device})
				if err != nil {
					t.Fatal(err)
				}
				if err := w.Tick(ctx); err != nil {
					t.Fatal(err)
				}
				if err := s.DB.First(&other, other.ID).Error; err != nil {
					t.Fatal(err)
				}
				if other.Status != "complete" {
					t.Fatalf("other Fabric blocked: %s", other.Status)
				}
				if len(fake.writes) != 0 {
					t.Fatal("write replayed")
				}
			})
		}
	}
}

type cancelledCollectionTransport struct {
	cancel context.CancelFunc
	closed bool
}

func (f cancelledCollectionTransport) Open(context.Context, delivery.DeviceSnapshot, delivery.Connection, func(Session) error) error {
	f.cancel()
	return transportReceiptError{context.Canceled, f.closed}
}

func TestCollectionCancellationCleansLeaseUsingClosureEvidence(t *testing.T) {
	for _, closed := range []bool{true, false} {
		t.Run(map[bool]string{true: "confirmed_closed", false: "unknown_closure"}[closed], func(t *testing.T) {
			s := operationsFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var device network.Device
			if err := s.DB.First(&device, 1).Error; err != nil {
				t.Fatal(err)
			}
			job, err := s.createCollection(ctx, 1, &dto.CollectionCreateReq{RequestKey: "cancelled-collection", Mode: "configuration"}, 7, []network.Device{device})
			if err != nil {
				t.Fatal(err)
			}
			w := NewWorker(s, cancelledCollectionTransport{cancel: cancel, closed: closed})
			if err := w.runCollection(ctx, job); !errors.Is(err, context.Canceled) {
				t.Fatalf("expected cancellation, got %v", err)
			}
			var lease delivery.DeviceLease
			err = s.DB.First(&lease, "device_id = ?", 1).Error
			if closed && !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("closed transport stranded lease: %+v, %v", lease, err)
			}
			if !closed && (err != nil || !lease.Quarantined) {
				t.Fatalf("unknown transport not quarantined: %+v, %v", lease, err)
			}
		})
	}
}

func TestLeaseCleanupCannotReleaseAnotherOwnerAfterCancellation(t *testing.T) {
	s := operationsFixture(t)
	if err := s.DB.Create(&delivery.DeviceLease{DeviceID: 1, Token: "new-owner", ExpiresAt: time.Now().Add(time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewWorker(s, nil).release(ctx, lease.Lease{DeviceID: 1, Token: "old-owner"}, true); err != nil {
		t.Fatal(err)
	}
	var row delivery.DeviceLease
	if err := s.DB.First(&row, "device_id = ?", 1).Error; err != nil {
		t.Fatal(err)
	}
	if row.Token != "new-owner" {
		t.Fatal("another owner's lease changed")
	}
}

func TestRecoverySurfacesDatabaseFailure(t *testing.T) {
	s := operationsFixture(t)
	past := time.Now().UTC().Add(-time.Minute)
	job := models.Collection{FabricID: 1, RequestKey: "database-error", Status: "running", Mode: "configuration", ClaimUntil: &past, Targets: []models.CollectionTarget{{DeviceID: 1, Status: "pending", Device: delivery.DeviceSnapshot{ID: 1}}}}
	if err := s.DB.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Migrator().DropTable(&models.Membership{}); err != nil {
		t.Fatal(err)
	}
	if err := NewWorker(s, nil).Recover(context.Background()); err == nil {
		t.Fatal("database failure was hidden")
	}
	if err := s.DB.First(&job, job.ID).Error; err != nil {
		t.Fatal(err)
	}
	if job.Status != "running" {
		t.Fatal("task terminalized despite failed persistence")
	}
}
