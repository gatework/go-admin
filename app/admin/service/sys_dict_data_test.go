package service

import (
	"testing"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/logger"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
)

func TestDictDataUpdateBoundaries(t *testing.T) {
	checkConfigDictUpdateBoundaries(t, configDictUpdateCase{table: "sys_dict_data", model: &models.SysDictData{DictCode: 1, DictType: "one"}, update: func(db *gorm.DB, id int) error {
		s := &DictionaryEntry{}
		s.DB = db
		s.Logger = logger.NewHelper(logger.Default())
		return s.Update(&dto.DictionaryEntryUpdate{ID: id, DictType: "one", DictLabel: "changed"})
	}})
}

func TestDictDataUpdatePreservesCreatorAndWritesZeroValues(t *testing.T) {
	db := configDictDB(t)
	if err := db.Create(&models.SysDictType{DictType: "one"}).Error; err != nil {
		t.Fatal(err)
	}
	row := models.SysDictData{DictCode: 1, DictType: "one", Status: 2, DictSort: 8, Remark: "old",
		CreateBy: 10}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	s := &DictionaryEntry{}
	s.DB = db
	s.Logger = logger.NewHelper(logger.Default())
	req := &dto.DictionaryEntryUpdate{ID: 1, DictType: "one"}
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
			s := &DictionaryEntry{}
			s.DB = db
			s.Logger = logger.NewHelper(logger.Default())
			var err error
			if name == "insert" {
				err = s.Create(&dto.DictionaryEntryCreate{DictType: "missing"})
			} else {
				if err := db.Create(&models.SysDictData{DictCode: 1, DictType: "original"}).Error; err != nil {
					t.Fatal(err)
				}
				err = s.Update(&dto.DictionaryEntryUpdate{ID: 1, DictType: "missing"})
			}
			if err == nil {
				t.Fatal("accepted missing dictionary parent")
			}
		})
	}
}
