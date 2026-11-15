package wireless

import (
	"context"
	"testing"

	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/contract/actions"
	network "github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/wireless"
	"github.com/gatework/motor/internal/testconfig"
)

func TestGroupsContainOnlyAuthorizedControllersAndTheirAncestors(t *testing.T) {
	s, _ := setup(t)
	if err := s.DB.AutoMigrate(&network.DeviceGroup{}); err != nil {
		t.Fatal(err)
	}
	group := func(code string, parent *int) network.DeviceGroup {
		row := network.DeviceGroup{Code: code, Name: code, ParentID: parent, Enabled: true}
		if err := s.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	root := group("region", nil)
	site := group("site", &root.ID)
	leaf := group("controllers", &site.ID)
	hidden := group("hidden-controllers", &root.ID)
	wired := group("wired-only", nil)
	group("empty", nil)
	for id, groupID := range map[int]int{1: leaf.ID, 2: hidden.ID} {
		if err := s.DB.Model(&network.Device{}).Where("id = ?", id).Update("group_id", groupID).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i, item := range []struct {
		kind  string
		group *int
	}{{"switch", &root.ID}, {"switch", &wired.ID}, {"switch", nil}, {"wireless_controller", nil}} {
		d := network.Device{IP: []string{"192.0.2.3", "192.0.2.4", "192.0.2.5", "192.0.2.6"}[i], Name: item.kind, SourceTags: []string{}, DeviceType: item.kind, GroupID: item.group, Status: "active", Tags: []string{}, Version: 1}
		d.SetCreateBy(1)
		if err := s.DB.Create(&d).Error; err != nil {
			t.Fatal(err)
		}
	}

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	ctx := context.Background()
	p := &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 1}
	tree, err := s.Groups(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if tree.TotalDeviceCount != 2 || tree.UngroupedDeviceCount != 1 || len(tree.Items) != 1 {
		t.Fatalf("tree includes non-controller or inaccessible inventory: %+v", tree)
	}
	node := tree.Items[0]
	if node.ID != root.ID || node.DirectDeviceCount != 0 || node.DeviceCount != 1 || len(node.Children) != 1 || node.Children[0].ID != site.ID || len(node.Children[0].Children) != 1 || node.Children[0].Children[0].ID != leaf.ID {
		t.Fatalf("controller ancestry/counts differ: %+v", node)
	}
	zero := 0
	for _, selection := range []struct {
		id    *int
		count int64
	}{{nil, tree.TotalDeviceCount}, {&root.ID, node.DeviceCount}, {&leaf.ID, 1}, {&zero, tree.UngroupedDeviceCount}, {&wired.ID, 0}} {
		_, count, err := s.Controllers(ctx, &dto.PageReq{GroupID: selection.id}, p)
		if err != nil || count != selection.count {
			t.Fatalf("controller list and tree disagree: %d, expected %d: %v", count, selection.count, err)
		}
	}
	denied, err := s.Groups(ctx, &actions.DataPermission{})
	if err != nil || len(denied.Items) != 0 || denied.TotalDeviceCount != 0 || denied.UngroupedDeviceCount != 0 {
		t.Fatalf("empty permission exposed tree: %+v %v", denied, err)
	}
}
