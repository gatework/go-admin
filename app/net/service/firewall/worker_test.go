package firewall

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/firewall/types"
	network "github.com/gatework/motor/app/net/models/inventory"
	"github.com/gatework/motor/app/net/service/firewall/engine"
)

const syntheticConfig = `zone inside
exit
zone outside
exit
interface eth0
 zone inside
 ip address 192.0.2.1/24
exit
interface eth1
 zone outside
 ip address 198.51.100.1/24
exit
policy from inside to outside
 rule id 1
  src-addr any
  dst-addr any
  service any
  action permit
  log session-end
 exit
exit
`

func TestImportAnalysisAndDraftPersistWithoutBrowserPolling(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	did := device(t, s, 7, "192.0.2.254")
	i, e := s.CreateInstance(ctx, InstanceInput{DeviceID: did, DomainID: 1, Vendor: "hillstone", Mode: "routed"}, all())
	if e != nil {
		t.Fatal(e)
	}
	in := ImportInput{RequestKey: "capture-1", Text: syntheticConfig, Filename: "synthetic.cfg", Complete: true}
	job, e := s.EnqueueImport(ctx, i.ID, in, all())
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.EnqueueImport(ctx, i.ID, in, all())
	if e != nil || again.ID != job.ID {
		t.Fatalf("duplicate import: %+v %v", again, e)
	}
	in.Text += "\n"
	if _, e = s.EnqueueImport(ctx, i.ID, in, all()); !errors.Is(e, ConflictErr) {
		t.Fatalf("different payload reused key: %v", e)
	}
	w := NewWorker(s)
	if e = w.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	job, e = s.Import(ctx, job.ID, all())
	if e != nil || job.Status != "succeeded" || job.Revision == "" {
		t.Fatalf("import: %+v %v", job, e)
	}
	_, e = s.SaveSettings(ctx, i.ID, types.Settings{Version: 1, Zones: []types.Binding{{Zone: "inside", DomainID: 1, Protected: true}, {Zone: "outside", DomainID: 1}}}, all())
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.SaveRequest(ctx, 0, RequestInput{Title: "existing access", DomainID: 1, Input: types.Input{DefaultProtocol: "tcp", Rows: []types.InputRow{{Key: "r1", Source: "192.0.2.10", Destination: "198.51.100.10", Service: "443"}}}}, all())
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.EnqueueAnalysis(ctx, r.ID, JobInput{RequestKey: "check-1", Version: r.Version}, all())
	if e != nil {
		t.Fatal(e)
	}
	if e = w.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	a, e = s.Analysis(ctx, a.ID, all())
	if e != nil || len(a.Result.Items) == 0 {
		t.Fatalf("analysis: %+v %v", a, e)
	}
	allowed := false
	for _, part := range a.Result.Items {
		allowed = allowed || len(part.Allowed) > 0
	}
	if !allowed {
		t.Fatalf("known permit lost: %+v", a.Result)
	}
	d, e := s.EnqueueDraft(ctx, DraftInput{RequestKey: "draft-1", AnalysisID: a.ID, Title: "existing access"}, all())
	if e != nil {
		t.Fatal(e)
	}
	if e = w.Tick(ctx); e != nil {
		t.Fatal(e)
	}
	d, e = s.Draft(ctx, d.ID, all())
	if e != nil || d.Status == "queued" || d.Status == "running" || len(d.Result.Items) == 0 {
		t.Fatalf("draft: %+v %v", d, e)
	}
	if _, e = s.SaveSettings(ctx, i.ID, types.Settings{Version: 2, Zones: []types.Binding{{Zone: "inside", DomainID: 1, Protected: true}}}, all()); e != nil {
		t.Fatal(e)
	}
	a, e = s.Analysis(ctx, a.ID, all())
	if e != nil || !a.Stale {
		t.Fatalf("binding change not invalidated: %+v %v", a, e)
	}
}

func importedInstance(t *testing.T, s *Firewall) (models.Instance, models.Import) {
	t.Helper()
	ctx := context.Background()
	i, err := s.CreateInstance(ctx, InstanceInput{DeviceID: device(t, s, 7, "203.0.113.10"), DomainID: 1, Vendor: "hillstone", Mode: "routed"}, all())
	if err != nil {
		t.Fatal(err)
	}
	job, err := s.EnqueueImport(ctx, i.ID, ImportInput{RequestKey: "fixture", Text: syntheticConfig, Complete: true}, all())
	if err != nil {
		t.Fatal(err)
	}
	if err = NewWorker(s).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	job, err = s.Import(ctx, job.ID, all())
	if err != nil || job.Status != "succeeded" {
		t.Fatalf("import: %+v %v", job, err)
	}
	return i, job
}

func TestZoneDomainInheritanceSurvivesSavingSettings(t *testing.T) {
	s := testService(t)
	i, _ := importedInstance(t, s)
	ctx := context.Background()
	d, err := s.SaveDomain(ctx, 0, DomainInput{Code: "isolated", Name: "隔离网络"})
	if err != nil {
		t.Fatal(err)
	}
	settings, err := s.SaveSettings(ctx, i.ID, types.Settings{Version: 1, Scopes: []types.ScopeBinding{{RoutingScope: "", DomainID: d.ID}}, Zones: []types.Binding{{Zone: "inside", Protected: true}}}, all())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Zones[0].DomainID != 0 {
		t.Fatalf("inheritance replaced by instance domain: %+v", settings)
	}
	coverage, err := s.Coverage(ctx, i.ID, all())
	if err != nil || len(coverage) == 0 {
		t.Fatalf("coverage: %+v %v", coverage, err)
	}
	for _, c := range coverage {
		if c.Protected && c.DomainID != d.ID {
			t.Fatalf("VRF mapping ignored: %+v", c)
		}
	}
}

func TestOversizedCoverageDoesNotCommitRejectedSettings(t *testing.T) {
	s := testService(t)
	i, _ := importedInstance(t, s)
	in := types.Settings{Version: 1, Zones: []types.Binding{{Zone: "inside", Protected: true}, {Zone: "outside", Protected: true}}}
	for z := range in.Zones {
		for n := range 2048 {
			in.Zones[z].Networks = append(in.Zones[z].Networks, fmt.Sprintf("10.%d.%d.%d", z, n/256, n%256))
		}
	}
	_, err := s.SaveSettings(context.Background(), i.ID, in, all())
	if !errors.Is(err, engine.BudgetErr) {
		t.Fatalf("coverage cap: %v", err)
	}
	after, err := s.Settings(context.Background(), i.ID, all())
	if err != nil || after.Version != 1 || len(after.Zones) != 0 {
		t.Fatalf("rejected settings changed persisted version: version=%d zones=%d err=%v", after.Version, len(after.Zones), err)
	}
}

func TestImportRecoveryRetainsPublicationAfterInventoryChanges(t *testing.T) {
	s := testService(t)
	i, job := importedInstance(t, s)
	ctx := context.Background()
	// Simulate a process crash after native publication, before the durable receipt.
	if err := s.DB.Model(&models.Import{}).Where("id = ?", job.ID).Updates(map[string]any{"status": "running", "revision": "", "lease_token": "expired-worker", "lease_until": time.Now().UTC().Add(-time.Minute)}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Model(&network.Device{}).Where("id = ?", i.DeviceID).Update("os_version", "new inventory version").Error; err != nil {
		t.Fatal(err)
	}
	if err := NewWorker(s).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.Import(ctx, job.ID, all())
	if err != nil || recovered.Status != "succeeded" || recovered.Revision != job.Revision {
		t.Fatalf("replay changed immutable publication: %+v %v", recovered, err)
	}
	var snapshots int64
	if err = s.DB.Model(&models.Snapshot{}).Where("instance_id = ?", i.ID).Count(&snapshots).Error; err != nil || snapshots != 1 {
		t.Fatalf("replay duplicated evidence: %d %v", snapshots, err)
	}
}

func TestExpiredAnalysisWorkerCannotOverwriteReclaimedJob(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	r, err := s.SaveRequest(ctx, 0, RequestInput{Title: "lease", DomainID: 1, Input: types.Input{Rows: []types.InputRow{{Source: "192.0.2.1", Destination: "198.51.100.1", Service: "443"}}}}, all())
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.EnqueueAnalysis(ctx, r.ID, JobInput{RequestKey: "lease", Version: r.Version}, all())
	if err != nil {
		t.Fatal(err)
	}
	w := NewWorker(s)
	old, claimed, err := w.claim(ctx, &models.Analysis{}, a.ID)
	if err != nil || !claimed {
		t.Fatalf("claim: %t %v", claimed, err)
	}
	if err = s.DB.Model(&models.Analysis{}).Where("id = ?", a.ID).Update("lease_until", time.Now().UTC().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	current, claimed, err := w.claim(ctx, &models.Analysis{}, a.ID)
	if err != nil || !claimed {
		t.Fatalf("reclaim: %t %v", claimed, err)
	}
	if err = w.analysisJob(ctx, a, old); err != nil {
		t.Fatal(err)
	}
	var stored models.Analysis
	if err = s.DB.First(&stored, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "running" || stored.LeaseToken != current {
		t.Fatalf("expired worker committed: %+v", stored)
	}
	if err = w.analysisJob(ctx, a, current); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.First(&stored, a.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status == "running" || stored.LeaseToken != "" {
		t.Fatalf("new worker could not finish: %+v", stored)
	}
}

func TestCancelledQueuedAnalysisNeverRuns(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	r, e := s.SaveRequest(ctx, 0, RequestInput{Title: "unlocated", DomainID: 1, Input: types.Input{Rows: []types.InputRow{{Key: "a", Source: "192.0.2.1", Destination: "198.51.100.1", Service: "443"}}}}, all())
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.EnqueueAnalysis(ctx, r.ID, JobInput{RequestKey: "cancel-1", Version: r.Version}, all())
	if e != nil {
		t.Fatal(e)
	}
	if e = s.CancelAnalysis(ctx, a.ID, all()); e != nil {
		t.Fatal(e)
	}
	if e = NewWorker(s).Tick(ctx); e != nil {
		t.Fatal(e)
	}
	var stored models.Analysis
	if e = s.DB.First(&stored, a.ID).Error; e != nil {
		t.Fatal(e)
	}
	if stored.Status != "cancelled" {
		t.Fatal(stored.Status)
	}
}
