package firewall

import (
	"context"
	"errors"
	"testing"
	"time"

	fm "github.com/gatework/firemon"
	"github.com/gatework/firemon/config"
	native "github.com/gatework/firemon/device"
	"github.com/gatework/firemon/snapshot"
	"github.com/gatework/firemon/vendors"
	"github.com/gatework/go-net-connector/connector"
	"github.com/gatework/motor/app/net/models/delivery"
	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/firewall/storage"
	"github.com/gatework/motor/app/net/models/firewall/types"
	"github.com/gatework/motor/app/net/models/inventory"
)

type captureReader struct {
	source   CollectionSource
	complete bool
	closeErr error
	readErr  error
	read     func(context.Context)
	closed   int
}

func (r *captureReader) Read(ctx context.Context, v native.View) (native.Capture, error) {
	if v != native.Running {
		panic("not running capture")
	}
	if r.read != nil {
		r.read(ctx)
	}
	return native.Capture{Source: config.Source{Text: syntheticConfig, Complete: r.complete}, Metadata: fm.Metadata{Device: storage.DeviceKey(r.source.Instance.ID), Vendor: r.source.Instance.Vendor, Context: r.source.Instance.NativeContext, CapturedAt: time.Now().UTC()}}, r.readErr
}
func (r *captureReader) Close() error { r.closed++; return r.closeErr }
func enqueueCapture(t *testing.T, s *Firewall, i models.Instance, key string) models.CollectionBatch {
	t.Helper()
	b, e := s.EnqueueCollection(context.Background(), CollectionInput{RequestKey: key, Kind: "capture", InstanceIDs: []int{i.ID}}, all())
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestLiveCollectorReplacesCompleteHeadWithInterruptedEvidence(t *testing.T) {
	s := testService(t)
	collectionCredentials(t, s)
	ctx := context.Background()
	i := collectionInstance(t, s, "192.0.2.21")
	complete := true
	var last *captureReader
	w := NewCollectionWorker(s, func(_ context.Context, source CollectionSource) (native.Reader, error) {
		last = &captureReader{source: source, complete: complete}
		if !complete {
			last.readErr = context.DeadlineExceeded
		}
		return last, nil
	})
	a := enqueueCapture(t, s, i, "complete")
	if err := w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	a, err := s.Collection(ctx, a.ID, all())
	if err != nil || a.Targets[0].Status != "succeeded" || last.closed != 1 {
		t.Fatalf("complete: %+v %v close=%d", a, err, last.closed)
	}
	complete = false
	b := enqueueCapture(t, s, i, "partial")
	if err = w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	b, err = s.Collection(ctx, b.ID, all())
	if err != nil || b.Targets[0].Status != "partial" || b.Targets[0].ErrorCode != "timeout" {
		t.Fatalf("partial: %+v %v", b, err)
	}
	current, _ := s.Instance(ctx, i.ID, all())
	if current.CurrentRevision != b.Targets[0].Revision || current.CurrentRevision == a.Targets[0].Revision {
		t.Fatal("stale complete head retained")
	}
	var n int64
	s.DB.Model(&delivery.DeviceLease{}).Count(&n)
	if n != 0 {
		t.Fatal("confirmed close retained lease")
	}
}
func TestCollectorSourceChangeAndUnconfirmedClose(t *testing.T) {
	s := testService(t)
	collectionCredentials(t, s)
	ctx := context.Background()
	i := collectionInstance(t, s, "192.0.2.22")
	b := enqueueCapture(t, s, i, "change")
	w := NewCollectionWorker(s, func(_ context.Context, source CollectionSource) (native.Reader, error) {
		return &captureReader{source: source, complete: true, closeErr: errors.New("private-close-output"), read: func(context.Context) {
			s.DB.Model(&inventory.Device{}).Where("id = ?", i.DeviceID).Update("ip", "192.0.2.23")
		}}, nil
	})
	if err := w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := s.Collection(ctx, b.ID, all())
	if err != nil || b.Targets[0].Revision != "" || b.Targets[0].ErrorCode != "session_close_unconfirmed" || b.Targets[0].SessionClosed {
		t.Fatalf("unsafe result: %+v %v", b, err)
	}
	var lease delivery.DeviceLease
	if err = s.DB.First(&lease, i.DeviceID).Error; err != nil || !lease.Quarantined {
		t.Fatalf("not quarantined: %+v %v", lease, err)
	}
	again := enqueueCapture(t, s, i, "blocked")
	if err = w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	again, _ = s.Collection(ctx, again.ID, all())
	if again.Targets[0].ErrorCode != "device_quarantined" {
		t.Fatalf("quarantine bypassed: %+v", again)
	}
}
func TestCollectorCancellationRetainsPartialCapture(t *testing.T) {
	s := testService(t)
	collectionCredentials(t, s)
	ctx := context.Background()
	i := collectionInstance(t, s, "192.0.2.24")
	b := enqueueCapture(t, s, i, "cancel")
	w := NewCollectionWorker(s, func(_ context.Context, source CollectionSource) (native.Reader, error) {
		return &captureReader{source: source, complete: true, read: func(ctx context.Context) {
			if _, err := s.CancelCollection(context.Background(), b.ID, all()); err != nil {
				t.Error(err)
			}
			<-ctx.Done()
		}}, nil
	})
	if err := w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := s.Collection(ctx, b.ID, all())
	if err != nil || b.Targets[0].Status != "partial" || b.Targets[0].Complete || b.Targets[0].Revision == "" || !b.Targets[0].SessionClosed {
		t.Fatalf("cancel lost evidence: %+v %v", b, err)
	}
}
func TestOpenFailureClosureEvidenceIsTypedAndRedacted(t *testing.T) {
	s := testService(t)
	collectionCredentials(t, s)
	ctx := context.Background()
	i := collectionInstance(t, s, "192.0.2.25")
	b := enqueueCapture(t, s, i, "auth")
	w := NewCollectionWorker(s, func(context.Context, CollectionSource) (native.Reader, error) {
		return nil, errors.Join(connector.ErrAuthentication, &connector.Error{Code: "cleanup_failed", Phase: "cleanup"})
	})
	if err := w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := s.Collection(ctx, b.ID, all())
	if err != nil || b.Targets[0].ErrorCode != "session_close_unconfirmed" {
		t.Fatalf("failed open released uncertain connection: %+v %v", b, err)
	}
}

func TestCollectionRecoveryDoesNotReopenPublishedCapture(t *testing.T) {
	s := testService(t)
	collectionCredentials(t, s)
	ctx := context.Background()
	i := collectionInstance(t, s, "192.0.2.26")
	b := enqueueCapture(t, s, i, "recover")
	opens := 0
	w := NewCollectionWorker(s, func(_ context.Context, source CollectionSource) (native.Reader, error) {
		opens++
		return &captureReader{source: source, complete: true}, nil
	})
	if err := w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	b, _ = s.Collection(ctx, b.ID, all())
	target := b.Targets[0]
	revision := target.Revision
	expired := time.Now().UTC().Add(-time.Minute)
	// The native publication and closure acknowledgement survived; final job
	// acknowledgement did not. Recovery must use its original publication ID.
	if err := s.DB.Model(&target).Updates(map[string]any{"status": "running", "revision": "", "lease_until": expired, "finished_at": nil}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Create(&delivery.DeviceLease{DeviceID: i.DeviceID, Token: target.LeaseToken, ExpiresAt: expired}).Error; err != nil {
		t.Fatal(err)
	}
	if err := w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := s.Collection(ctx, b.ID, all())
	if err != nil || b.Targets[0].Revision != revision || b.Targets[0].Status != "succeeded" || opens != 1 {
		t.Fatalf("recovery redialed or lost result: %+v %v opens=%d", b, err, opens)
	}
}

func TestCollectionSharesPhysicalLeaseAndFencesSource(t *testing.T) {
	s := testService(t)
	collectionCredentials(t, s)
	ctx := context.Background()
	i := collectionInstance(t, s, "192.0.2.27")
	b := enqueueCapture(t, s, i, "shared")
	lease := delivery.DeviceLease{DeviceID: i.DeviceID, Token: "other-workflow", ExpiresAt: time.Now().UTC().Add(time.Hour)}
	if err := s.DB.Create(&lease).Error; err != nil {
		t.Fatal(err)
	}
	opens := 0
	w := NewCollectionWorker(s, func(context.Context, CollectionSource) (native.Reader, error) {
		opens++
		return nil, connector.ErrAuthentication
	})
	if err := w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	b, _ = s.Collection(ctx, b.ID, all())
	if opens != 0 || b.Targets[0].Stage != "waiting_device" {
		t.Fatalf("shared lease bypassed: %+v", b)
	}
	if err := s.DB.Delete(&lease).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Model(&inventory.Device{}).Where("id = ?", i.DeviceID).Update("ip", "192.0.2.28").Error; err != nil {
		t.Fatal(err)
	}
	if err := w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	b, _ = s.Collection(ctx, b.ID, all())
	if opens != 0 || b.Targets[0].ErrorCode != "source_changed" {
		t.Fatalf("changed device opened: %+v", b)
	}
}

func TestCollectionTenantAdmissionAndExpiredPublicationFence(t *testing.T) {
	s := testService(t)
	collectionCredentials(t, s)
	ctx := context.Background()
	var targets []models.CollectionTarget
	for n, ip := range []string{"192.0.2.31", "192.0.2.32", "192.0.2.33"} {
		i := collectionInstance(t, s, ip)
		b := enqueueCapture(t, s, i, []string{"one", "two", "three"}[n])
		targets = append(targets, b.Targets[0])
	}
	w := NewCollectionWorker(s, nil)
	first, _, ok, err := w.claim(ctx, targets[0])
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, _, ok, err = w.claim(ctx, targets[1]); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if _, _, ok, err = w.claim(ctx, targets[2]); err != nil || ok {
		t.Fatalf("tenant concurrency exceeded: %v %v", ok, err)
	}
	expired := time.Now().UTC().Add(-time.Minute)
	if err = s.DB.Model(&first).Update("lease_until", expired).Error; err != nil {
		t.Fatal(err)
	}
	st, _ := s.store()
	fw, err := (vendors.Parser{}).Parse(ctx, config.Source{Text: syntheticConfig, Complete: true}, fm.Metadata{Device: storage.DeviceKey(first.InstanceID), Vendor: "hillstone"})
	if err != nil {
		t.Fatal(err)
	}
	guard := collectionStore{Store: st, worker: w, target: first}
	if _, err = guard.Publish(ctx, fw, snapshot.PublishOptions{Expected: first.ExpectedRevision, PublicationID: publicationID(first), Live: true}); !errors.Is(err, ConflictErr) {
		t.Fatalf("expired worker published: %v", err)
	}
	if err = w.recover(ctx, st); err != nil {
		t.Fatal(err)
	}
	var lease delivery.DeviceLease
	if err = s.DB.First(&lease, first.DeviceID).Error; err != nil || !lease.Quarantined {
		t.Fatalf("interrupted session released: %+v %v", lease, err)
	}
}

func TestLivePartialCaptureStalesSavedAnalysisAndDraft(t *testing.T) {
	s := testService(t)
	collectionCredentials(t, s)
	ctx := context.Background()
	i := collectionInstance(t, s, "192.0.2.34")
	complete := true
	w := NewCollectionWorker(s, func(_ context.Context, source CollectionSource) (native.Reader, error) {
		return &captureReader{source: source, complete: complete}, nil
	})
	enqueueCapture(t, s, i, "complete")
	if err := w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveSettings(ctx, i.ID, types.Settings{Version: 1, Zones: []types.Binding{{Zone: "inside", Protected: true}}}, all()); err != nil {
		t.Fatal(err)
	}
	r, err := s.SaveRequest(ctx, 0, RequestInput{Title: "pinned", DomainID: 1, Input: types.Input{Rows: []types.InputRow{{Key: "one", Source: "192.0.2.10", Destination: "198.51.100.10", Service: "443"}}}}, all())
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.EnqueueAnalysis(ctx, r.ID, JobInput{RequestKey: "analysis", Version: r.Version}, all())
	if err != nil {
		t.Fatal(err)
	}
	offline := NewWorker(s)
	if err = offline.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	d, err := s.EnqueueDraft(ctx, DraftInput{RequestKey: "draft", AnalysisID: a.ID, Title: "pinned"}, all())
	if err != nil {
		t.Fatal(err)
	}
	if err = offline.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	complete = false
	enqueueCapture(t, s, i, "partial")
	if err = w.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	a, err = s.Analysis(ctx, a.ID, all())
	if err != nil || !a.Stale {
		t.Fatalf("old analysis remained current: %+v %v", a, err)
	}
	d, err = s.Draft(ctx, d.ID, all())
	if err != nil || !d.Stale {
		t.Fatalf("old draft remained current: %+v %v", d, err)
	}
}
