package firewall

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	native "github.com/gatework/firemon/device"
	"github.com/gatework/motor/app/net/models/delivery"
	models "github.com/gatework/motor/app/net/models/firewall"
)

type uncertainCommitPool struct {
	*sql.DB
	fail *atomic.Bool
}

func (p uncertainCommitPool) BeginTx(ctx context.Context, options *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return &uncertainCommitTx{Tx: tx, fail: p.fail}, nil
}

type uncertainCommitTx struct {
	*sql.Tx
	fail *atomic.Bool
}

func (tx uncertainCommitTx) Commit() error {
	if err := tx.Tx.Commit(); err != nil {
		return err
	}
	if tx.fail.Swap(false) {
		return errors.New("fixture commit acknowledgement lost")
	}
	return nil
}

func TestCollectionReconcilesPublishErrorBeforeTerminalAcknowledgement(t *testing.T) {
	s := testService(t)
	collectionCredentials(t, s)
	ctx := context.Background()
	i := collectionInstance(t, s, "192.0.2.41")
	b := enqueueCapture(t, s, i, "uncertain-publish")
	pool, err := s.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	var fail atomic.Bool
	wrapped := uncertainCommitPool{DB: pool, fail: &fail}
	s.DB.ConnPool = wrapped
	s.DB.Statement.ConnPool = wrapped
	if err = s.DB.Callback().Update().After("gorm:update").Register("fixture:lose-publication-commit-ack", func(tx *gorm.DB) {
		values, ok := tx.Statement.Dest.(map[string]any)
		if ok && tx.Statement.Table == (models.CollectionTarget{}).TableName() && values["stage"] == "publishing" {
			fail.Store(true)
		}
	}); err != nil {
		t.Fatal(err)
	}
	opens := 0
	w := NewCollectionWorker(s, func(_ context.Context, source CollectionSource) (native.Reader, error) {
		opens++
		return &captureReader{source: source, complete: true}, nil
	})
	if err = w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	b, err = s.Collection(ctx, b.ID, all())
	if err != nil || b.Targets[0].Revision == "" || b.Targets[0].SnapshotID == 0 || !b.Targets[0].Complete {
		t.Fatalf("durable publication lost from terminal result: %+v %v", b, err)
	}
	if err = w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	again, err := s.EnqueueCollection(ctx, CollectionInput{RequestKey: "uncertain-publish", Kind: "capture", InstanceIDs: []int{i.ID}}, all())
	if err != nil || opens != 1 || again.Targets[0].Revision != b.Targets[0].Revision {
		t.Fatalf("retry did not retain publication: %+v %v opens=%d", again, err, opens)
	}
}

func TestCollectionSkipsBusyQueuePrefix(t *testing.T) {
	s := testService(t)
	collectionCredentials(t, s)
	ctx := context.Background()
	busy := collectionInstance(t, s, "192.0.2.42")
	idle := collectionInstance(t, s, "192.0.2.43")
	// A hundred separate manual requests for one busy device must not consume
	// the worker's entire candidate window and starve an unrelated idle device.
	source, err := collectionSource(ctx, s.DB, busy.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	for n := range MaxCollectionTargets {
		b := models.CollectionBatch{ActorID: s.Actor, Kind: "capture", RequestKey: fmt.Sprintf("busy-%d", n), Status: "queued", Targets: []models.CollectionTarget{{InstanceID: busy.ID, DeviceID: busy.DeviceID, SourceHash: source.fingerprint(), Vendor: busy.Vendor, Status: "queued", Stage: "queued", SessionClosed: true}}}
		if err = s.DB.Create(&b).Error; err != nil {
			t.Fatal(err)
		}
	}
	b := enqueueCapture(t, s, idle, "idle")
	if err = s.DB.Create(&delivery.DeviceLease{DeviceID: busy.DeviceID, Token: "another-workflow", ExpiresAt: time.Now().UTC().Add(time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	w := NewCollectionWorker(s, func(_ context.Context, source CollectionSource) (native.Reader, error) {
		if source.Device.ID != idle.DeviceID {
			t.Error("busy device opened")
		}
		return &captureReader{source: source, complete: true}, nil
	})
	if err = w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	b, err = s.Collection(ctx, b.ID, all())
	if err != nil || b.Targets[0].Status != "succeeded" {
		t.Fatalf("idle device starved: %+v %v", b, err)
	}
}
