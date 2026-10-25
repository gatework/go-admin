package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"gorm.io/gorm"

	"go-admin/app/admin/models"
	"go-admin/app/admin/service/dto"
	cDto "go-admin/common/dto"
)

type SysDictType struct {
	service.Service
}

// GetPage 获取列表
func (e *SysDictType) GetPage(c *dto.SysDictTypeGetPageReq, list *[]models.SysDictType, count *int64) error {
	var err error
	var data models.SysDictType

	err = e.Orm.Model(&data).
		Scopes(
			cDto.MakeCondition(c.GetNeedSearch()),
			cDto.Paginate(c.GetPageSize(), c.GetPageIndex()),
		).
		Find(list).Limit(-1).Offset(-1).
		Count(count).Error
	if err != nil {
		e.Log.Errorf("db error: %s", err)
		return err
	}
	return nil
}

// Get 获取对象
func (e *SysDictType) Get(d *dto.SysDictTypeGetReq, model *models.SysDictType) error {
	var err error

	db := e.Orm.First(model, d.GetId())
	err = db.Error
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		err = errors.New("查看对象不存在或无权查看")
		e.Log.Errorf("db error: %s", err)
		return err
	}
	if err = db.Error; err != nil {
		e.Log.Errorf("db error: %s", err)
		return err
	}
	return nil
}

// Insert 创建对象
func (e *SysDictType) Insert(c *dto.SysDictTypeInsertReq) error {
	if strings.TrimSpace(c.DictType) == "" {
		return errors.New("字典类型不能为空")
	}
	var err error
	var data models.SysDictType
	c.Generate(&data)
	data.CreateBy = c.CreateBy
	var count int64
	// The error was dropped, so a query that failed left count at zero and the
	// insert went ahead as though the name were free.
	if err = e.Orm.Model(&data).Where("dict_type = ?", data.DictType).Count(&count).Error; err != nil {
		e.Log.Errorf("db error: %s", err)
		return err
	}
	if count > 0 {
		return fmt.Errorf("当前字典类型[%s]已经存在！", data.DictType)
	}
	err = e.Orm.Create(&data).Error
	if err != nil {
		e.Log.Errorf("db error: %s", err)
		return err
	}
	c.Id = data.ID
	return nil
}

// Update 修改对象
func (e *SysDictType) Update(c *dto.SysDictTypeUpdateReq) error {
	if c.Id <= 0 {
		return gorm.ErrRecordNotFound
	}
	if strings.TrimSpace(c.DictType) == "" {
		return errors.New("字典类型不能为空")
	}
	return e.Orm.Transaction(func(tx *gorm.DB) error {
		var model models.SysDictType
		if err := lockAdminRows(tx, model.TableName()).First(&model, c.GetId()).Error; err != nil {
			return err
		}
		oldKey := model.DictType
		var count int64
		if err := tx.Model(&models.SysDictType{}).Where("dict_type = ? AND dict_id <> ?", c.DictType, model.ID).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("当前字典类型[%s]已经存在！", c.DictType)
		}
		c.Generate(&model)
		model.UpdateBy = c.UpdateBy
		db := tx.Omit("create_by", "created_at").Select("*").Updates(&model)
		if db.Error != nil {
			return db.Error
		}
		if db.RowsAffected == 0 {
			return errors.New("无权更新该数据")
		}
		if oldKey == model.DictType {
			return nil
		}
		return tx.Model(&models.SysDictData{}).Where("dict_type = ?", oldKey).Update("dict_type", model.DictType).Error
	})
}

// Remove 删除
func (e *SysDictType) Remove(d *dto.SysDictTypeDeleteReq) error {
	return e.Orm.Transaction(func(tx *gorm.DB) error {
		var types []models.SysDictType
		if err := lockAdminRows(tx, (&models.SysDictType{}).TableName()).Where("dict_id IN ?", d.Ids).Order("dict_id").Find(&types).Error; err != nil {
			return err
		}
		if len(types) == 0 {
			return errors.New("无权删除该数据")
		}
		for _, dict := range types {
			var count int64
			if err := tx.Model(&models.SysDictData{}).Where("dict_type = ?", dict.DictType).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return fmt.Errorf("字典类型[%s]仍有字典数据，不能删除", dict.DictType)
			}
		}
		return tx.Delete(&types).Error
	})
}

// GetAll 获取所有
func (e *SysDictType) GetAll(c *dto.SysDictTypeGetPageReq, list *[]models.SysDictType) error {
	var err error
	var data models.SysDictType

	err = e.Orm.Model(&data).
		Scopes(
			cDto.MakeCondition(c.GetNeedSearch()),
		).
		Find(list).Error
	if err != nil {
		e.Log.Errorf("db error: %s", err)
		return err
	}
	return nil
}
