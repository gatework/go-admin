package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"go-admin/app/network"
	"go-admin/app/network/grouping"
	"go-admin/app/network/models"
	"go-admin/app/network/service/dto"
)

func groupFixture(t *testing.T, s *Device, code string, parent *int) models.DeviceGroup {
	t.Helper()
	g, err := (&DeviceGroup{Service: s.Service}).Create(context.Background(), &dto.DeviceGroupReq{Code: code, Name: code, ParentID: parent, Enabled: true}, 7)
	if err != nil {
		t.Fatal(err)
	}
	return g.DeviceGroup
}
func ptr(v int) *int { return &v }
func assignment(rows []models.Device, id *int, source string) *dto.DeviceGroupAssignReq {
	r := &dto.DeviceGroupAssignReq{GroupID: id, GroupSource: source}
	for _, row := range rows {
		r.Devices = append(r.Devices, dto.DeviceVersion{ID: row.Id, Version: row.Version})
	}
	return r
}

func TestGroupTreeCountsAndListExportUseTheSameAuthorizedSubtree(t *testing.T) {
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	s := testService(t)
	ctx := context.Background()
	gs := &DeviceGroup{Service: s.Service}
	root := groupFixture(t, s, "root", nil)
	child := groupFixture(t, s, "child", &root.Id)
	for i, item := range []struct {
		owner int
		group *int
		name  string
	}{{7, &root.Id, "SZX9-CSW1"}, {7, &child.Id, "SZX9-SW1"}, {8, &child.Id, "hidden"}, {7, nil, "ungrouped"}} {
		row := models.Device{SourceFacts: fact([]string{"192.0.2.1", "192.0.2.2", "192.0.2.3", "192.0.2.4"}[i], item.name), GroupID: item.group, GroupSource: "manual", ClassificationSource: "auto", Tags: []string{}, Version: 1}
		row.DeviceRole, row.DeviceType = grouping.Classification(row.Name, row.Serial)
		row.SetCreateBy(item.owner)
		if err := s.Orm.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	p := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 7}
	tree, err := gs.Tree(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if tree.TotalDeviceCount != 3 || tree.UngroupedDeviceCount != 1 || len(tree.Items) != 1 || tree.Items[0].DeviceCount != 2 || tree.Items[0].DirectDeviceCount != 1 || tree.Items[0].Children[0].DeviceCount != 1 {
		t.Fatalf("wrong scoped tree: %+v", tree)
	}
	for _, query := range []struct {
		r        dto.DevicePageReq
		count    int64
		expected string
	}{{dto.DevicePageReq{GroupID: &root.Id}, 2, "SZX9-CSW1"}, {dto.DevicePageReq{GroupID: &child.Id}, 1, "SZX9-SW1"}, {dto.DevicePageReq{GroupID: ptr(0)}, 1, "ungrouped"}, {dto.DevicePageReq{GroupID: ptr(999)}, 0, ""}, {dto.DevicePageReq{GroupID: &root.Id, DeviceRole: "core_switch"}, 1, "SZX9-CSW1"}} {
		rows, n, err := s.Page(ctx, &query.r, p)
		if err != nil || n != query.count {
			t.Fatalf("page %+v: %d %v", query.r, n, err)
		}
		csv, err := s.Export(ctx, &query.r, p)
		if err != nil || strings.Contains(string(csv), "hidden") {
			t.Fatalf("export leaked: %v %s", err, csv)
		}
		if query.count > 0 && (len(rows) == 0 || !strings.Contains(string(csv), query.expected)) {
			t.Fatalf("export differs from list: %s", csv)
		}
	}
	denied, err := gs.Tree(ctx, &actions.DataPermission{})
	if err != nil || denied.TotalDeviceCount != 0 || denied.Items[0].DeviceCount != 0 {
		t.Fatalf("missing scope did not fail closed: %+v %v", denied, err)
	}
}

func TestGroupsRejectCyclesStaleDeletionAndDisabledAssignments(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	gs := &DeviceGroup{Service: s.Service}
	a := groupFixture(t, s, "a", nil)
	b := groupFixture(t, s, "b", &a.Id)
	if _, err := gs.Update(ctx, a.Id, &dto.DeviceGroupReq{Code: "changed-code", Name: a.Name, Enabled: true, Version: a.Version}, 7); !errors.Is(err, network.ErrInvalidInput) {
		t.Fatalf("mutable stable code: %v", err)
	}
	if _, err := gs.Update(ctx, a.Id, &dto.DeviceGroupReq{Code: a.Code, Name: a.Name, ParentID: &b.Id, Enabled: true, Version: a.Version}, 7); !errors.Is(err, network.ErrInvalidInput) {
		t.Fatalf("cycle accepted: %v", err)
	}
	if err := gs.Delete(ctx, a.Id, a.Version, 7); !errors.Is(err, network.ErrGroupNotEmpty) {
		t.Fatalf("deleted parent: %v", err)
	}
	s.Source = inventory(fact("192.0.2.1", "normal"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	rows, _, _ := s.Page(ctx, &dto.DevicePageReq{}, fullScope())
	if _, err := s.AssignGroup(ctx, assignment(rows, &b.Id, "manual"), 7, fullScope()); err != nil {
		t.Fatal(err)
	}
	if err := gs.Delete(ctx, b.Id, b.Version, 7); !errors.Is(err, network.ErrGroupNotEmpty) {
		t.Fatalf("deleted assigned group: %v", err)
	}
	if _, err := gs.Update(ctx, b.Id, &dto.DeviceGroupReq{Code: b.Code, Name: b.Name, ParentID: b.ParentID, Enabled: false, Version: b.Version}, 7); err != nil {
		t.Fatal(err)
	}
	row, _ := s.Get(ctx, rows[0].Id, fullScope())
	req := dto.DeviceUpdateReq{Version: row.Version, Status: row.Status, GroupID: row.GroupID, GroupSource: row.GroupSource, ClassificationSource: row.ClassificationSource}
	if _, err := s.Update(ctx, row.Id, &req, 7, fullScope()); err != nil {
		t.Fatalf("unchanged disabled membership rejected: %v", err)
	}
	s.Source = inventory(fact("192.0.2.2", "normal"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	rows, _, _ = s.Page(ctx, &dto.DevicePageReq{Keyword: "192.0.2.2"}, fullScope())
	if _, err := s.AssignGroup(ctx, assignment(rows, &b.Id, "manual"), 7, fullScope()); !errors.Is(err, network.ErrInvalidInput) {
		t.Fatalf("disabled assignment accepted: %v", err)
	}
	if err := gs.Delete(ctx, b.Id, b.Version, 7); !errors.Is(err, network.ErrConflict) {
		t.Fatalf("stale delete accepted: %v", err)
	}
}

func TestBatchAssignmentIsAtomicScopedAndVersioned(t *testing.T) {
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	s := testService(t)
	ctx := context.Background()
	g := groupFixture(t, s, "target", nil)
	s.Source = inventory(fact("192.0.2.1", "a"), fact("192.0.2.2", "b"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	rows, _, _ := s.Page(ctx, &dto.DevicePageReq{}, fullScope())
	p := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 7}
	req := assignment(rows, &g.Id, "manual")
	req.Devices[1].Version++
	if _, err := s.AssignGroup(ctx, req, 7, p); !errors.Is(err, network.ErrConflict) {
		t.Fatalf("stale batch: %v", err)
	}
	unchanged, _, _ := s.Page(ctx, &dto.DevicePageReq{}, p)
	for _, row := range unchanged {
		if row.GroupID != nil || row.Version != 1 {
			t.Fatalf("partial batch: %+v", row)
		}
	}
	s.Orm.Model(&models.Device{}).Where("id = ?", rows[1].Id).Update("create_by", 8)
	if _, err := s.AssignGroup(ctx, assignment(rows, &g.Id, "manual"), 7, p); !errors.Is(err, network.ErrNotFound) {
		t.Fatalf("inaccessible batch: %v", err)
	}
	visible, _ := s.Get(ctx, rows[0].Id, p)
	if visible.GroupID != nil || visible.Version != 1 {
		t.Fatal("partial inaccessible batch")
	}
	if n, err := s.AssignGroup(ctx, assignment(rows[:1], &g.Id, "manual"), 7, p); err != nil || n != 1 {
		t.Fatalf("assignment: %d %v", n, err)
	}
	visible, _ = s.Get(ctx, rows[0].Id, p)
	if visible.GroupID == nil || *visible.GroupID != g.Id || visible.GroupSource != "manual" || visible.Version != 2 {
		t.Fatalf("missing assignment: %+v", visible)
	}
}

func TestSyncPreservesManualModesAndRecalculatesOnlyAutomaticFields(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	if err := grouping.SeedAndBackfill(s.Orm); err != nil {
		t.Fatal(err)
	}
	s.Source = inventory(fact("192.0.2.1", "SZX9-CSW1"), fact("192.0.2.2", "SHA-ASW1"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	a, _, _ := s.Page(ctx, &dto.DevicePageReq{Keyword: "192.0.2.1"}, fullScope())
	b, _, _ := s.Page(ctx, &dto.DevicePageReq{Keyword: "192.0.2.2"}, fullScope())
	if a[0].Group != "传音大厦" || a[0].DeviceRole != "core_switch" || a[0].GroupSource != "auto" || a[0].ClassificationSource != "auto" {
		t.Fatalf("new inference: %+v", a[0])
	}
	manual := dto.DeviceUpdateReq{Version: b[0].Version, Status: "active", GroupSource: "manual", ClassificationSource: "manual", DeviceType: "router", DeviceRole: "other"}
	if _, err := s.Update(ctx, b[0].Id, &manual, 7, fullScope()); err != nil {
		t.Fatal(err)
	}
	s.Source = inventory(fact("192.0.2.1", "SHA-FW1"), fact("192.0.2.2", "CKG-AP1"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Get(ctx, a[0].Id, fullScope())
	preserved, _ := s.Get(ctx, b[0].Id, fullScope())
	if after.Group != "上海职场" || after.DeviceRole != "firewall" || after.DeviceType != "firewall" || after.Version <= a[0].Version {
		t.Fatalf("auto stale: %+v", after)
	}
	if preserved.GroupID != nil || preserved.GroupSource != "manual" || preserved.DeviceType != "router" || preserved.DeviceRole != "other" || preserved.ClassificationSource != "manual" {
		t.Fatalf("manual overwritten: %+v", preserved)
	}
	manual.Version = preserved.Version
	manual.GroupSource = "auto"
	manual.ClassificationSource = "auto"
	restored, err := s.Update(ctx, preserved.Id, &manual, 7, fullScope())
	if err != nil || restored.Group != "重庆职场" || restored.DeviceRole != "wireless_ap" {
		t.Fatalf("return to auto: %+v %v", restored, err)
	}
	gs := &DeviceGroup{Service: s.Service}
	var target models.DeviceGroup
	s.Orm.First(&target, *after.GroupID)
	if _, err := gs.Update(ctx, target.Id, &dto.DeviceGroupReq{Code: target.Code, Name: target.Name, Enabled: false, Version: target.Version}, 7); err != nil {
		t.Fatal(err)
	}
	s.Source = inventory(fact("192.0.2.1", "SHA-SW1"), fact("192.0.2.3", "SHA-SW2"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	kept, _ := s.Get(ctx, after.Id, fullScope())
	fresh, _, _ := s.Page(ctx, &dto.DevicePageReq{Keyword: "192.0.2.3"}, fullScope())
	if kept.GroupID == nil || *kept.GroupID != target.Id || fresh[0].GroupID != nil {
		t.Fatalf("disabled target: old=%+v new=%+v", kept, fresh[0])
	}
	var other models.DeviceGroup
	if err := s.Orm.Where("code = ?", "site.other").First(&other).Error; err != nil {
		t.Fatal(err)
	}
	if err := gs.Delete(ctx, other.Id, other.Version, 7); err != nil {
		t.Fatal(err)
	}
	s.Source = inventory(fact("192.0.2.4", "ordinary"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	var recreated int64
	if err := s.Orm.Model(&models.DeviceGroup{}).Where("code = ?", "site.other").Count(&recreated).Error; err != nil || recreated != 0 {
		t.Fatalf("sync recreated deleted standard group: %d %v", recreated, err)
	}
	unmatched, _, err := s.Page(ctx, &dto.DevicePageReq{Keyword: "192.0.2.4"}, fullScope())
	if err != nil || len(unmatched) != 1 || unmatched[0].GroupID != nil || unmatched[0].GroupSource != "auto" {
		t.Fatalf("deleted target assignment: %+v %v", unmatched, err)
	}
}

func TestAutomaticMembershipUsesStableCodeAfterStandardGroupDeletion(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	groups := &DeviceGroup{Service: s.Service}
	if err := grouping.SeedAndBackfill(s.Orm); err != nil {
		t.Fatal(err)
	}
	var standard models.DeviceGroup
	if err := s.Orm.Where("code = ?", "site.shanghai").First(&standard).Error; err != nil {
		t.Fatal(err)
	}
	if err := groups.Delete(ctx, standard.Id, standard.Version, 7); err != nil {
		t.Fatal(err)
	}
	custom, err := groups.Create(ctx, &dto.DeviceGroupReq{Code: "custom.archive", Name: standard.Name, Enabled: true}, 7)
	if err != nil {
		t.Fatal(err)
	}
	s.Source = inventory(fact("192.0.2.1", "SHA-SW1"))
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	rows, _, err := s.Page(ctx, &dto.DevicePageReq{}, fullScope())
	if err != nil || len(rows) != 1 {
		t.Fatalf("inventory: %+v %v", rows, err)
	}
	if rows[0].GroupID != nil {
		t.Errorf("sync assigned a same-named custom group: %+v", rows[0])
	}
	if _, err := s.AssignGroup(ctx, assignment(rows, &custom.Id, "manual"), 7, fullScope()); err != nil {
		t.Fatal(err)
	}
	manual, err := s.Get(ctx, rows[0].Id, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	automatic, err := s.Update(ctx, manual.Id, &dto.DeviceUpdateReq{Version: manual.Version, Status: manual.Status, GroupSource: "auto", ClassificationSource: "auto"}, 7, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	if automatic.GroupID != nil || automatic.GroupSource != "auto" {
		t.Errorf("editor return-to-auto assigned a same-named custom group: %+v", automatic)
	}
	if _, err := s.AssignGroup(ctx, assignment([]models.Device{automatic}, &custom.Id, "manual"), 7, fullScope()); err != nil {
		t.Fatal(err)
	}
	manual, err = s.Get(ctx, rows[0].Id, fullScope())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AssignGroup(ctx, assignment([]models.Device{manual}, nil, "auto"), 7, fullScope()); err != nil {
		t.Fatal(err)
	}
	automatic, err = s.Get(ctx, rows[0].Id, fullScope())
	if err != nil || automatic.GroupID != nil || automatic.GroupSource != "auto" {
		t.Errorf("batch return-to-auto assigned a same-named custom group: %+v %v", automatic, err)
	}
	recreated, err := groups.Create(ctx, &dto.DeviceGroupReq{Code: standard.Code, Name: "Renamed region", Enabled: true}, 7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Sync(ctx, 7, fullScope(), false); err != nil {
		t.Fatal(err)
	}
	restored, err := s.Get(ctx, rows[0].Id, fullScope())
	if err != nil || restored.GroupID == nil || *restored.GroupID != recreated.Id {
		t.Fatalf("stable-code recreation did not restore automatic membership: %+v %v", restored, err)
	}
}
