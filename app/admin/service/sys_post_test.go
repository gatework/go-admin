package service

import (
	"testing"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
)

func TestPostUpdateMissingDoesNotInsert(t *testing.T) {
	db := integrityDB(t)
	s := &SysPost{}
	s.DB = db
	err := s.Update(&dto.SysPostUpdateReq{PostID: 999, PostName: "unexpected", PostCode: "unexpected", Status: 2})
	if err == nil {
		var count int64
		db.Model(&models.SysPost{}).Where("post_id = ?", 999).Count(&count)
		t.Fatalf("update nonexistent post returned nil; inserted=%d", count)
	}
}

func TestPostUpdatePreservesCreator(t *testing.T) {
	db := integrityDB(t)
	row := models.SysPost{PostID: 1, PostName: "original",
		CreateBy: 10}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	s := &SysPost{}
	s.DB = db
	req := &dto.SysPostUpdateReq{PostID: 1, PostName: "changed"}
	req.CreateBy = 999
	req.UpdateBy = 20
	if err := s.Update(req); err != nil {
		t.Fatal(err)
	}
	var got models.SysPost
	if err := db.First(&got, 1).Error; err != nil {
		t.Fatal(err)
	}
	if got.CreateBy != 10 || got.UpdateBy != 20 || got.PostName != "changed" {
		t.Fatalf("update metadata = %+v", got)
	}
}
