package firewall

import (
	"context"
	"errors"
	"testing"

	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/firewall/types"
	"github.com/gatework/motor/app/net/service/firewall/dto"
)

func TestDraftCreationRetryReturnsAcceptedDraftAfterBasisBecomesStale(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	requestInput := dto.RequestInput{
		Title: "existing intent", DomainID: 1,
		Input: types.Input{Rows: []types.InputRow{{Key: "row-1", Source: "192.0.2.1", Destination: "198.51.100.1", Service: "443"}}},
	}
	r, err := s.SaveRequest(ctx, 0, requestInput, all())
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.EnqueueAnalysis(ctx, r.ID, dto.JobInput{RequestKey: "initial-analysis", Version: r.Version}, all())
	if err != nil {
		t.Fatal(err)
	}
	if err = NewWorker(s).Tick(ctx); err != nil {
		t.Fatal(err)
	}
	in := dto.DraftInput{RequestKey: "lost-create-response", AnalysisID: a.ID, Title: "original draft"}
	draft, err := s.EnqueueDraft(ctx, in, all())
	if err != nil {
		t.Fatal(err)
	}
	// A subsequent request edit invalidates the evidence after the first create
	// committed, while the caller still has no acknowledgement of that create.
	requestInput.Version = r.Version
	requestInput.Input.Rows[0].Service = "8443"
	if _, err = s.SaveRequest(ctx, r.ID, requestInput, all()); err != nil {
		t.Fatal(err)
	}
	retry, err := s.EnqueueDraft(ctx, in, all())
	if err != nil || retry.ID != draft.ID || !retry.Stale {
		t.Fatalf("accepted retry lost or current basis misrepresented: id=%d want=%d stale=%t err=%v", retry.ID, draft.ID, retry.Stale, err)
	}
	var count int64
	if err = s.DB.Model(&models.Draft{}).Where("analysis_id = ?", a.ID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("retry duplicated draft: count=%d err=%v", count, err)
	}
	changed := in
	changed.Title = "different payload"
	if _, err = s.EnqueueDraft(ctx, changed, all()); !errors.Is(err, ConflictErr) {
		t.Fatalf("changed payload reused accepted key: %v", err)
	}
	changed = in
	changed.RequestKey = "new-create-on-stale-basis"
	if _, err = s.EnqueueDraft(ctx, changed, all()); !errors.Is(err, ConflictErr) {
		t.Fatalf("new create accepted stale evidence: %v", err)
	}
}
