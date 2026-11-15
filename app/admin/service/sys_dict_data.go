package service

import (
	"errors"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/service"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
	cDto "github.com/gatework/motor/common/dto"
)

// SysDictData coordinates administration queries and mutations.
type SysDictData struct {
	service.Service
}

// GetPage 获取列表
func (e *SysDictData) GetPage(c *dto.SysDictDataGetPageReq, list *[]models.SysDictData, count *int64) error {
	var err error
	var data models.SysDictData

	err = e.DB.Model(&data).
		Scopes(
			cDto.MakeCondition(c.GetNeedSearch()),
			cDto.Paginate(c.GetPageSize(), c.GetPageIndex()),
		).
		Find(list).Limit(-1).Offset(-1).
		Count(count).Error
	if err != nil {
		e.Logger.Errorf("db error: %s", err)
		return err
	}
	return nil
}

// Get 获取对象
func (e *SysDictData) Get(d *dto.SysDictDataGetReq, model *models.SysDictData) error {
	var err error
	var data models.SysDictData

	db := e.DB.Model(&data).
		First(model, d.GetID())
	err = db.Error
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		err = errors.New("查看对象不存在或无权查看")
		e.Logger.Errorf("db error: %s", err)
		return err
	}
	if err = db.Error; err != nil {
		e.Logger.Errorf("db error: %s", err)
		return err
	}
	return nil
}

// Insert 创建对象
func (e *SysDictData) Insert(c *dto.SysDictDataInsertReq) error {
	var data models.SysDictData
	err := e.DB.Transaction(func(tx *gorm.DB) error {
		var parent models.SysDictType
		if err := lockAdminRows(tx, parent.TableName()).Where("dict_type = ?", c.DictType).First(&parent).Error; err != nil {
			return err
		}
		c.Generate(&data)
		data.CreateBy = c.CreateBy
		return tx.Create(&data).Error
	})
	if err == nil {
		c.ID = data.DictCode
	}
	return err
}

// Update 修改对象，锁定目标字典类型以免并发重命名或删除产生孤立数据。
func (e *SysDictData) Update(c *dto.SysDictDataUpdateReq) error {
	if c.ID <= 0 {
		return gorm.ErrRecordNotFound
	}
	return e.DB.Transaction(func(tx *gorm.DB) error {
		var parent models.SysDictType
		if err := lockAdminRows(tx, parent.TableName()).Where("dict_type = ?", c.DictType).First(&parent).Error; err != nil {
			return err
		}
		var model models.SysDictData
		if err := tx.First(&model, c.GetID()).Error; err != nil {
			return err
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
		return nil
	})
}

// Remove 删除
func (e *SysDictData) Remove(c *dto.SysDictDataDeleteReq) error {
	var err error
	var data models.SysDictData

	db := e.DB.Delete(&data, c.GetID())
	if err = db.Error; err != nil {
		e.Logger.Errorf("Delete error: %s", err)
		return err
	}
	if db.RowsAffected == 0 {
		err = errors.New("无权删除该数据")
		return err
	}
	return nil
}

// GetAll 获取所有
func (e *SysDictData) GetAll(c *dto.SysDictDataGetPageReq, list *[]models.SysDictData) error {
	var err error
	var data models.SysDictData

	query := e.DB.Model(&data)
	if c.DictType != "" {
		query = query.Where("dict_type = ?", c.DictType)
	}
	err = query.
		Scopes(
			cDto.MakeCondition(c.GetNeedSearch()),
		).
		Find(list).Error
	if err != nil {
		e.Logger.Errorf("db error: %s", err)
		return err
	}
	return nil
}
