package service

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"go-admin/app/network/models"
	"go-admin/app/network/service/dto"
)

func TestExplicitEmptyInventoryFiltersComposeWithScope(t *testing.T) {
	s := testService(t)
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	for i, values := range []struct {
		kind, role, vendor string
		owner              int
	}{
		{"", "", "", 7}, {"switch", "", "Cisco", 7}, {"switch", "leaf_switch", "", 7}, {"", "", "", 8},
	} {
		d := models.Device{SourceFacts: fact([]string{"192.0.2.1", "192.0.2.2", "192.0.2.3", "192.0.2.4"}[i], "fixture"), DeviceType: values.kind, DeviceRole: values.role, Status: "active", Tags: []string{}}
		d.Vendor, d.CreateBy = values.vendor, values.owner
		if err := s.Orm.Create(&d).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		query string
		count int64
		first int
	}{
		{"deviceTypeEmpty=true", 1, 1},
		{"deviceRoleEmpty=true&vendor=Cisco", 1, 2},
		{"vendorEmpty=true&deviceType=switch", 1, 3},
		{"deviceRole=leaf_switch", 1, 3},
		{"deviceTypeEmpty=true&deviceType=switch", 1, 1},
		{"deviceTypeEmpty=false", 3, 3},
	} {
		r := &dto.DevicePageReq{}
		if err := binding.Query.Bind(httptest.NewRequest("GET", "/?"+tc.query, nil), r); err != nil {
			t.Fatal(err)
		}
		rows, count, err := s.Page(context.Background(), r, &actions.DataPermission{DataScope: actions.DataScopeSelf, UserId: 7})
		if err != nil || count != tc.count || len(rows) == 0 || rows[0].Id != tc.first {
			t.Errorf("%s: count=%d rows=%+v err=%v", tc.query, count, rows, err)
		}
	}
}
