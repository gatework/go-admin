package service

import (
	"testing"

	"github.com/go-admin-team/go-admin-core/v2/logger"
	"gorm.io/gorm"

	"go-admin/app/admin/models"
	"go-admin/app/admin/service/dto"
)

func TestDictDataUpdateBoundaries(t *testing.T) {
	checkConfigDictUpdateBoundaries(t, configDictUpdateCase{table: "sys_dict_data", model: &models.SysDictData{DictCode: 1, DictType: "one"}, update: func(db *gorm.DB, id int) error {
		s := &SysDictData{}
		s.Orm = db
		s.Log = logger.NewHelper(logger.DefaultLogger)
		return s.Update(&dto.SysDictDataUpdateReq{Id: id, DictType: "one", DictLabel: "changed"})
	}})
}

func TestDictDataUpdatePreservesCreatorAndWritesZeroValues(t *testing.T) {
	db := configDictDB(t)
	if err := db.Create(&models.SysDictType{DictType: "one"}).Error; err != nil {
		t.Fatal(err)
	}
	row := models.SysDictData{DictCode: 1, DictType: "one", Status: 2, DictSort: 8, Remark: "old"}
	row.CreateBy = 10
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	s := &SysDictData{}
	s.Orm = db
	s.Log = logger.NewHelper(logger.DefaultLogger)
	req := &dto.SysDictDataUpdateReq{Id: 1, DictType: "one"}
	req.CreateBy = 999
	req.UpdateBy = 20
	if err := s.Update(req); err != nil {
		t.Fatal(err)
	}
	var got models.SysDictData
	if err := db.First(&got, 1).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != 0 || got.DictSort != 0 || got.Remark != "" || got.CreateBy != 10 || got.UpdateBy != 20 {
		t.Fatalf("update fields = %+v", got)
	}
}

func TestDictDataRejectsMissingTargetType(t *testing.T) {
	for _, name := range []string{"insert", "update"} {
		t.Run(name, func(t *testing.T) {
			db := configDictDB(t)
			s := &SysDictData{}
			s.Orm = db
			s.Log = logger.NewHelper(logger.DefaultLogger)
			var err error
			if name == "insert" {
				err = s.Insert(&dto.SysDictDataInsertReq{DictType: "missing"})
			} else {
				if err := db.Create(&models.SysDictData{DictCode: 1, DictType: "original"}).Error; err != nil {
					t.Fatal(err)
				}
				err = s.Update(&dto.SysDictDataUpdateReq{Id: 1, DictType: "missing"})
			}
			if err == nil {
				t.Fatal("accepted missing dictionary parent")
			}
		})
	}
}
