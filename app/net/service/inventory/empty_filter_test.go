package inventory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin/binding"

	"github.com/gatework/motor-core/sdk/config"
	"github.com/gatework/motor-core/sdk/contract/actions"
	models "github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/inventory"
	"github.com/gatework/motor/internal/testconfig"
)

func TestExplicitEmptyInventoryFiltersComposeWithScope(t *testing.T) {
	s := testService(t)

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	for i, values := range []struct {
		kind, role, vendor string
		owner              int
	}{
		{"", "", "", 7}, {"switch", "", "Cisco", 7}, {"switch", "leaf_switch", "", 7}, {"", "", "", 8},
	} {
		d := models.Device{SourceFacts: fact([]string{"192.0.2.1", "192.0.2.2", "192.0.2.3", "192.0.2.4"}[i], "fixture"), DeviceType: values.kind, DeviceRole: values.role, Status: "active", Tags: []string{}}
		d.Vendor, d.CreateBy = values.vendor, values.owner
		if err := s.DB.Create(&d).Error; err != nil {
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
		if err := binding.Query.Bind(httptest.NewRequest(http.MethodGet, "/?"+tc.query, nil), r); err != nil {
			t.Fatal(err)
		}
		rows, count, err := s.Page(context.Background(), r, &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 7})
		if err != nil || count != tc.count || len(rows) == 0 || rows[0].ID != tc.first {
			t.Errorf("%s: count=%d rows=%+v err=%v", tc.query, count, rows, err)
		}
	}
}
