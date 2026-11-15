package service

import (
	"testing"

	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
)

func TestMenuMoveUpdatesOnlyDescendants(t *testing.T) {
	db := integrityDB(t)
	menus := []models.SysMenu{{MenuID: 1, Paths: "/0/1"}, {MenuID: 2, Paths: "/0/2"}, {MenuID: 3, ParentID: 1, Paths: "/0/1/3"}, {MenuID: 4, ParentID: 3, Paths: "/0/1/3/4"}, {MenuID: 30, ParentID: 1, Paths: "/0/1/30"}}
	if err := db.Create(&menus).Error; err != nil {
		t.Fatal(err)
	}
	s := &Menu{}
	s.DB = db
	s.Logger = logger.NewHelper(logger.Default())
	if err := s.Update(t.Context(), &dto.MenuUpdate{MenuID: 3, ParentID: 2}); err != nil {
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
			menus := []models.SysMenu{{MenuID: 1, Paths: "/0/1"}, {MenuID: 2, ParentID: 1, Paths: "/0/1/2"}}
			if err := db.Create(&menus).Error; err != nil {
				t.Fatal(err)
			}
			s := &Menu{}
			s.DB = db
			s.Logger = logger.NewHelper(logger.Default())
			if err := s.Update(t.Context(), &dto.MenuUpdate{MenuID: tc.id, ParentID: tc.parent}); err == nil {
				t.Fatal("invalid update succeeded")
			}
			var got models.SysMenu
			if err := db.First(&got, 1).Error; err != nil {
				t.Fatal(err)
			}
			if got.ParentID != 0 || got.Paths != "/0/1" {
				t.Fatalf("failed update changed root: %+v", got)
			}
		})
	}
}

func TestMenuUpdatePreservesCreator(t *testing.T) {
	db := integrityDB(t)
	row := models.SysMenu{MenuID: 1, MenuName: "original", Paths: "/0/1",
		CreateBy: 10}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	s := &Menu{}
	s.DB = db
	req := &dto.MenuUpdate{MenuID: 1, MenuName: "changed"}
	req.CreateBy = 999
	req.UpdateBy = 20
	if err := s.Update(t.Context(), req); err != nil {
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
