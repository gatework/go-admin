//go:build integration

package inventory

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/gatework/motor/app/net/models/grouping"
	"github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/inventory"
)

func TestPostgresGroupOpposingMovesCannotCreateCycle(t *testing.T) {
	s := postgresService(t)
	gs := &DeviceGroup{Service: s.Service}
	ctx := context.Background()
	a := groupFixture(t, s, "a", nil)
	b := groupFixture(t, s, "b", nil)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, move := range []struct {
		row    inventory.DeviceGroup
		parent int
	}{{a, b.ID}, {b, a.ID}} {
		go func(row inventory.DeviceGroup, parent int) {
			<-start
			_, err := gs.Update(ctx, row.ID, &dto.DeviceGroupReq{Code: row.Code, Name: row.Name, ParentID: &parent, Enabled: true, Version: row.Version}, 7)
			results <- err
		}(move.row, move.parent)
	}
	close(start)
	success, rejected := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			success++
		} else if errors.Is(err, inventory.InvalidInputErr) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || rejected != 1 {
		t.Fatalf("moves success=%d rejected=%d", success, rejected)
	}
	tree, err := gs.Tree(ctx, fullScope())
	if err != nil || len(tree.Items) != 1 || len(tree.Items[0].Children) != 1 {
		t.Fatalf("invalid graph %+v %v", tree, err)
	}
}

func TestPostgresGroupDeletionAndAssignmentCannotLeaveDanglingMembership(t *testing.T) {
	s := postgresService(t)
	gs := &DeviceGroup{Service: s.Service}
	ctx := context.Background()
	s.Source = sourceWithFacts(fact("192.0.2.1", "SW1"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	for iteration := 0; iteration < 10; iteration++ {
		group := groupFixture(t, s, "target", nil)
		rows, _, err := s.Page(ctx, &dto.DevicePageReq{}, fullScope())
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		assigned := make(chan error, 1)
		deleted := make(chan error, 1)
		go func() {
			<-start
			_, err := s.AssignGroup(ctx, assignment(rows, &group.ID, "manual"), 7, fullScope())
			assigned <- err
		}()
		go func() { <-start; deleted <- gs.Delete(ctx, group.ID, group.Version, 7) }()
		close(start)
		assignErr, deleteErr := <-assigned, <-deleted
		if assignErr == nil {
			if !errors.Is(deleteErr, inventory.GroupNotEmptyErr) {
				t.Fatalf("assigned deleted group: %v", deleteErr)
			}
			current, err := s.Get(ctx, rows[0].ID, fullScope())
			if err != nil || current.GroupID == nil || *current.GroupID != group.ID {
				t.Fatalf("membership lost: %+v %v", current, err)
			}
			if _, err := s.AssignGroup(ctx, assignment([]inventory.Device{current}, nil, "manual"), 7, fullScope()); err != nil {
				t.Fatal(err)
			}
			if err := gs.Delete(ctx, group.ID, group.Version, 7); err != nil {
				t.Fatal(err)
			}
		} else if !errors.Is(assignErr, inventory.InvalidInputErr) || deleteErr != nil {
			t.Fatalf("unexpected race result: assign=%v delete=%v", assignErr, deleteErr)
		}
		var dangling int64
		if err := s.DB.Table("sys_network_device d").Joins("LEFT JOIN sys_network_device_group g ON g.id = d.group_id").Where("d.group_id IS NOT NULL AND g.id IS NULL").Count(&dangling).Error; err != nil || dangling != 0 {
			t.Fatalf("dangling membership=%d %v", dangling, err)
		}
	}
}

func TestPostgresBatchAssignmentCASHasOneCompleteWinner(t *testing.T) {
	s := postgresService(t)
	ctx := context.Background()
	a := groupFixture(t, s, "a", nil)
	b := groupFixture(t, s, "b", nil)
	s.Source = sourceWithFacts(fact("192.0.2.1", "SW1"), fact("192.0.2.2", "SW2"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	rows, _, _ := s.Page(ctx, &dto.DevicePageReq{}, fullScope())
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []int{a.ID, b.ID} {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			_, err := s.AssignGroup(ctx, assignment(rows, &id, "manual"), 7, fullScope())
			results <- err
		}(id)
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, inventory.ConflictErr) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	after, _, _ := s.Page(ctx, &dto.DevicePageReq{}, fullScope())
	if success != 1 || conflict != 1 || after[0].GroupID == nil || after[1].GroupID == nil || *after[0].GroupID != *after[1].GroupID || after[0].Version != 2 || after[1].Version != 2 {
		t.Fatalf("batch fractured: success=%d conflict=%d rows=%+v", success, conflict, after)
	}
}

func TestPostgresSyncRechecksModesAfterConcurrentManualSave(t *testing.T) {
	s := postgresService(t)
	ctx := context.Background()
	if err := grouping.SeedAndBackfill(s.DB); err != nil {
		t.Fatal(err)
	}
	s.Source = sourceWithFacts(fact("192.0.2.1", "SZX9-CSW1"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	rows, _, err := s.Page(ctx, &dto.DevicePageReq{}, fullScope())
	if err != nil || len(rows) != 1 {
		t.Fatalf("initial inventory: %+v %v", rows, err)
	}
	before := rows[0]
	fetching, release := make(chan struct{}), make(chan struct{})
	s.Source = inventorySource{configured: true, fetch: func(context.Context) ([]inventory.SourceFacts, error) {
		close(fetching)
		<-release
		return []inventory.SourceFacts{fact("192.0.2.1", "SHA-FW1")}, nil
	}}
	finished := make(chan error, 1)
	go func() { _, err := s.Sync(ctx, 8, fullScope(), false); finished <- err }()
	<-fetching
	request := dto.DeviceUpdateReq{Version: before.Version, Status: "active", GroupSource: "manual", ClassificationSource: "manual", DeviceType: "router", DeviceRole: "other", Owner: "Alice"}
	saved, saveErr := s.Update(ctx, before.ID, &request, 7, fullScope())
	close(release)
	syncErr := <-finished
	if saveErr != nil || syncErr != nil {
		t.Fatalf("concurrent save/sync: save=%v sync=%v", saveErr, syncErr)
	}
	after, err := s.Get(ctx, before.ID, fullScope())
	if err != nil || after.Name != "SHA-FW1" || after.GroupID != nil || after.GroupSource != "manual" || after.ClassificationSource != "manual" || after.DeviceType != "router" || after.DeviceRole != "other" || after.Owner != "Alice" || after.Version != saved.Version+1 {
		t.Fatalf("sync overwrote the in-flight manual override: %+v %v", after, err)
	}
	request.Version = saved.Version
	request.GroupSource = "auto"
	request.ClassificationSource = "auto"
	if _, err := s.Update(ctx, before.ID, &request, 7, fullScope()); !errors.Is(err, inventory.ConflictErr) {
		t.Fatalf("automatic sync did not fence stale editors: %v", err)
	}
}
