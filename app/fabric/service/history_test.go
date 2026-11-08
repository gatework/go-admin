package service

import (
	"context"
	"fmt"
	"testing"

	sdkconfig "github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"go-admin/app/fabric/models"
	"go-admin/app/fabric/service/dto"
	network "go-admin/app/network/models"
	"gorm.io/gorm"
)

func TestHistoricalPagesBoundSnapshotReadsAndCountOnlyVisibleRecords(t *testing.T) {
	for _, kind := range []string{"changes", "collections"} {
		for _, scope := range []string{actions.DataScopeSelf, actions.DataScopeAll} {
			t.Run(kind+"/"+scope, func(t *testing.T) {
				s := operationsFixture(t)
				ctx := context.Background()
				old := sdkconfig.ApplicationConfig.EnableDP
				sdkconfig.ApplicationConfig.EnableDP = true
				t.Cleanup(func() { sdkconfig.ApplicationConfig.EnableDP = old })
				other := 2
				if err := s.Orm.Create(&models.Fabric{ID: other, Code: "OTHER", Version: 1, RulesVersion: 1}).Error; err != nil {
					t.Fatal(err)
				}
				if err := s.Orm.Model(&models.Membership{}).Where("device_id=2").Updates(map[string]any{"fabric_id": other, "mode": "manual"}).Error; err != nil {
					t.Fatal(err)
				}
				if err := s.Orm.Model(&network.Device{}).Where("id=2").Update("create_by", 8).Error; err != nil {
					t.Fatal(err)
				}
				expected := []int{}
				for i := 1; i <= 225; i++ {
					target := 1
					if i%3 == 0 {
						target = 2
					}
					if kind == "changes" {
						if err := s.Orm.Create(&models.Change{ID: i, FabricID: 1, SourceDeviceID: 1, CreateBy: 7, RequestKey: fmt.Sprintf("history-%d", i), Targets: []models.ChangeTarget{{DeviceID: target}}}).Error; err != nil {
							t.Fatal(err)
						}
					} else {
						if err := s.Orm.Create(&models.Collection{ID: i, FabricID: 1, CreateBy: 7, RequestKey: fmt.Sprintf("history-%d", i), Targets: []models.CollectionTarget{{DeviceID: target}}}).Error; err != nil {
							t.Fatal(err)
						}
					}
				}
				for i := 225; i >= 1; i-- {
					if scope == actions.DataScopeAll || i%3 != 0 {
						expected = append(expected, i)
					}
				}
				// Observe actual materialized snapshots: a small requested page must never
				// deserialize the entire history, even when an accurate filtered count is needed.
				largestRead := 0
				if err := s.Orm.Callback().Query().After("gorm:query").Register("test_history_batch", func(tx *gorm.DB) {
					size := 0
					switch rows := tx.Statement.Dest.(type) {
					case *[]models.Change:
						size = len(*rows)
					case *[]models.Collection:
						size = len(*rows)
					}
					if size > largestRead {
						largestRead = size
					}
				}); err != nil {
					t.Fatal(err)
				}
				req := &dto.PageReq{}
				req.PageIndex = 7
				req.PageSize = 17
				p := &actions.DataPermission{DataScope: scope, UserId: 7, RoleId: 1}
				ids := []int{}
				var count int64
				var err error
				if kind == "changes" {
					var rows []models.Change
					rows, count, err = s.Changes(ctx, 1, req, p)
					for _, row := range rows {
						ids = append(ids, row.ID)
					}
				} else {
					var rows []models.Collection
					rows, count, err = s.Collections(ctx, 1, req, p)
					for _, row := range rows {
						ids = append(ids, row.ID)
					}
				}
				if err != nil {
					t.Fatal(err)
				}
				want := expected[102:119]
				if count != int64(len(expected)) || fmt.Sprint(ids) != fmt.Sprint(want) {
					t.Fatalf("wrong filtered page/count: got=%v/%d want=%v/%d", ids, count, want, len(expected))
				}
				bound := 100
				if scope == actions.DataScopeAll {
					bound = req.PageSize
				}
				if largestRead > bound {
					t.Fatalf("materialized %d historical target snapshots at once; bound=%d", largestRead, bound)
				}
			})
		}
	}
}

func TestActiveTaskDiscoveryFiltersBeforePagingAndScope(t *testing.T) {
	for _, scope := range []string{actions.DataScopeSelf, actions.DataScopeAll} {
		t.Run(scope, func(t *testing.T) {
			s := operationsFixture(t)
			old := sdkconfig.ApplicationConfig.EnableDP
			sdkconfig.ApplicationConfig.EnableDP = true
			t.Cleanup(func() { sdkconfig.ApplicationConfig.EnableDP = old })
			if err := s.Orm.Model(&network.Device{}).Where("id=2").Update("create_by", 8).Error; err != nil {
				t.Fatal(err)
			}
			for i, status := range []string{"queued", "running", "running", "complete", "failed", "cancelled"} {
				target := 1
				if i == 2 {
					target = 2
				}
				if err := s.Orm.Create(&models.Collection{ID: i + 1, FabricID: 1, CreateBy: 7, RequestKey: fmt.Sprint(i), Status: status, Targets: []models.CollectionTarget{{DeviceID: target}}}).Error; err != nil {
					t.Fatal(err)
				}
				if err := s.Orm.Create(&models.Change{ID: i + 1, FabricID: 1, SourceDeviceID: 1, CreateBy: 7, RequestKey: fmt.Sprint(i), Status: status, Targets: []models.ChangeTarget{{DeviceID: target}}}).Error; err != nil {
					t.Fatal(err)
				}
			}
			req := &dto.PageReq{Status: "active"}
			req.PageIndex = 1
			req.PageSize = 1
			permission := &actions.DataPermission{DataScope: scope, UserId: 7, RoleId: 1}
			collections, count, err := historicalPage(s, context.Background(), 1, req, permission, func(c models.Collection) int { return c.ID }, collectionDeviceIDs)
			changes, changeCount, changeErr := historicalPage(s, context.Background(), 1, req, permission, func(c models.Change) int { return c.ID }, changeDeviceIDs)
			wantID, wantCount := 2, int64(2)
			if scope == actions.DataScopeAll {
				wantID, wantCount = 3, 3
			}
			if err != nil || changeErr != nil || count != wantCount || changeCount != wantCount || len(collections) != 1 || len(changes) != 1 || collections[0].ID != wantID || changes[0].ID != wantID {
				t.Fatalf("active discovery: collections=%v changes=%v counts=%d/%d errors=%v/%v", collections, changes, count, changeCount, err, changeErr)
			}
		})
	}
}
