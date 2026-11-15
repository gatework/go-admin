package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/service"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
	commondto "github.com/gatework/motor/common/dto"
)

// DictionaryEntry coordinates administration queries and mutations.
type DictionaryEntry struct {
	service.Service
}

// List 获取列表
func (e *DictionaryEntry) List(ctx context.Context, c *dto.DictionaryEntrySearch, list *[]models.SysDictData, count *int64) error {
	var err error
	var data models.SysDictData

	err = e.DB.WithContext(ctx).Model(&data).
		Scopes(
			commondto.MakeCondition(c.GetNeedSearch()),
			commondto.Paginate(c.GetPageSize(), c.GetPageIndex()),
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
func (e *DictionaryEntry) Get(ctx context.Context, d *dto.DictionaryEntryByID, model *models.SysDictData) error {
	var err error
	var data models.SysDictData

	db := e.DB.WithContext(ctx).Model(&data).
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

// Create 创建对象
func (e *DictionaryEntry) Create(ctx context.Context, c *dto.DictionaryEntryCreate) error {
	var data models.SysDictData
	err := e.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
func (e *DictionaryEntry) Update(ctx context.Context, c *dto.DictionaryEntryUpdate) error {
	if c.ID <= 0 {
		return gorm.ErrRecordNotFound
	}
	return e.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

// Delete 删除
func (e *DictionaryEntry) Delete(ctx context.Context, c *dto.DictionaryEntryDelete) error {
	var err error
	var data models.SysDictData

	db := e.DB.WithContext(ctx).Delete(&data, c.GetID())
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

// All 获取所有
func (e *DictionaryEntry) All(ctx context.Context, c *dto.DictionaryEntrySearch, list *[]models.SysDictData) error {
	var err error
	var data models.SysDictData

	query := e.DB.WithContext(ctx).Model(&data)
	if c.DictType != "" {
		query = query.Where("dict_type = ?", c.DictType)
	}
	err = query.
		Scopes(
			commondto.MakeCondition(c.GetNeedSearch()),
		).
		Find(list).Error
	if err != nil {
		e.Logger.Errorf("db error: %s", err)
		return err
	}
	return nil
}
