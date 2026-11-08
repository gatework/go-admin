//go:build integration

package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go-admin/app/network"
	"go-admin/app/network/models"
)

func TestPostgresCircuitConcurrentCASAndFailedBindingAtomicity(t *testing.T) {
	d := postgresService(t)
	if err := d.Orm.AutoMigrate(&models.Circuit{}); err != nil {
		t.Fatal(err)
	}
	s := &Circuit{Service: d.Service, Source: knownCircuitSource()}
	ctx := context.Background()
	draft := circuitDraft("CAS-1")
	draft.AEndpoint.Monitoring = models.CircuitBinding{HostID: "1", InItemID: "10"}
	row, err := s.Create(ctx, draft, 7, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, name := range []string{"Winner A", "Winner B"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			req := draftFromCircuit(row)
			req.Name = name
			<-start
			_, err := s.Update(ctx, row.Id, req, 7, fullScope())
			results <- err
		}(name)
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, network.ErrConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("CAS winners=%d conflicts=%d", success, conflict)
	}
	current, err := s.Get(ctx, row.Id, fullScope())
	if err != nil || current.Version != 2 {
		t.Fatalf("version: %+v %v", current, err)
	}
	draft = draftFromCircuit(current)
	draft.Name = "must roll back"
	draft.AEndpoint.Monitoring.OutItemID = "20"
	if _, err = s.Update(ctx, row.Id, draft, 7, fullScope()); !errors.Is(err, network.ErrCircuitBinding) {
		t.Fatalf("binding accepted: %v", err)
	}
	after, err := s.Get(ctx, row.Id, fullScope())
	if err != nil || after.Name != current.Name || after.Version != current.Version || after.AEndpoint.Monitoring.OutItemID != "" {
		t.Fatalf("half update: %+v %v", after, err)
	}
	if err = s.Delete(ctx, row.Id, current.Version, 7, fullScope()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create(ctx, circuitDraft("CAS-1"), 7, fullScope()); !errors.Is(err, network.ErrCircuitDuplicate) {
		t.Fatalf("Postgres reused deleted number: %v", err)
	}
}
