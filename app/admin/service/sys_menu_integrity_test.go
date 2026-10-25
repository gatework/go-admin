package service

import (
	"github.com/go-admin-team/go-admin-core/v2/logger"
	"go-admin/app/admin/models"
	"go-admin/app/admin/service/dto"
	"testing"
)

func TestMenuMoveUpdatesOnlyDescendants(t *testing.T) {
	db := integrityDB(t)
	menus := []models.SysMenu{{MenuId: 1, Paths: "/0/1"}, {MenuId: 2, Paths: "/0/2"}, {MenuId: 3, ParentId: 1, Paths: "/0/1/3"}, {MenuId: 4, ParentId: 3, Paths: "/0/1/3/4"}, {MenuId: 30, ParentId: 1, Paths: "/0/1/30"}}
	if err := db.Create(&menus).Error; err != nil {
		t.Fatal(err)
	}
	s := &SysMenu{}
	s.Orm = db
	s.Log = logger.NewHelper(logger.DefaultLogger)
	if err := s.Update(&dto.SysMenuUpdateReq{MenuId: 3, ParentId: 2}).Error; err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int]string{3: "/0/2/3", 4: "/0/2/3/4", 30: "/0/1/30"} {
		var got models.SysMenu
		if err := db.First(&got, id).Error; err != nil {
			t.Fatal(err)
		}
		if got.Paths != want {
			t.Errorf("menu %d path = %q, want %q", id, got.Paths, want)
		}
	}
}

func TestMenuRejectsMissingUpdateAndInvalidParent(t *testing.T) {
	for _, tc := range []struct {
		name       string
		id, parent int
	}{{"missing", 999, 0}, {"self", 1, 1}, {"descendant", 1, 2}, {"missing parent", 1, 999}} {
		t.Run(tc.name, func(t *testing.T) {
			db := integrityDB(t)
			menus := []models.SysMenu{{MenuId: 1, Paths: "/0/1"}, {MenuId: 2, ParentId: 1, Paths: "/0/1/2"}}
			if err := db.Create(&menus).Error; err != nil {
				t.Fatal(err)
			}
			s := &SysMenu{}
			s.Orm = db
			s.Log = logger.NewHelper(logger.DefaultLogger)
			if err := s.Update(&dto.SysMenuUpdateReq{MenuId: tc.id, ParentId: tc.parent}).Error; err == nil {
				t.Fatal("invalid update succeeded")
			}
			var got models.SysMenu
			if err := db.First(&got, 1).Error; err != nil {
				t.Fatal(err)
			}
			if got.ParentId != 0 || got.Paths != "/0/1" {
				t.Fatalf("failed update changed root: %+v", got)
			}
		})
	}
}

func TestMenuUpdatePreservesCreator(t *testing.T) {
	db := integrityDB(t)
	row := models.SysMenu{MenuId: 1, MenuName: "original", Paths: "/0/1"}
	row.CreateBy = 10
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	s := &SysMenu{}
	s.Orm = db
	req := &dto.SysMenuUpdateReq{MenuId: 1, MenuName: "changed"}
	req.CreateBy = 999
	req.UpdateBy = 20
	if err := s.Update(req).Error; err != nil {
		t.Fatal(err)
	}
	var got models.SysMenu
	if err := db.First(&got, 1).Error; err != nil {
		t.Fatal(err)
	}
	if got.CreateBy != 10 || got.UpdateBy != 20 || got.MenuName != "changed" {
		t.Fatalf("update metadata = %+v", got)
	}
}
