package service

import (
	"errors"

	"github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"gorm.io/gorm"

	"go-admin/app/admin/models"
	"go-admin/app/admin/service/dto"
	cDto "go-admin/common/dto"
)

type SysConfig struct {
	service.Service
}

// GetPage 获取SysConfig列表
func (e *SysConfig) GetPage(c *dto.SysConfigGetPageReq, list *[]models.SysConfig, count *int64) error {
	err := e.Orm.
		Scopes(
			cDto.MakeCondition(c.GetNeedSearch()),
			cDto.Paginate(c.GetPageSize(), c.GetPageIndex()),
		).
		Find(list).Limit(-1).Offset(-1).
		Count(count).Error
	if err != nil {
		e.Log.Errorf("Service GetSysConfigPage error:%s", err)
		return err
	}
	return nil
}

// Get 获取SysConfig对象
func (e *SysConfig) Get(d *dto.SysConfigGetReq, model *models.SysConfig) error {
	err := e.Orm.
		FirstOrInit(model, d.GetId()).
		Error
	if err != nil {
		e.Log.Errorf("db error:%s", err)
		_ = e.AddError(err)
		return err
	}
	if model.Id == 0 {
		err = errors.New("查看对象不存在或无权查看")
		e.Log.Errorf("Service GetSysApi error: %s", err)
		_ = e.AddError(err)
		return err
	}
	return nil
}

// Insert 创建SysConfig对象
func (e *SysConfig) Insert(c *dto.SysConfigControl) error {
	var err error
	var data models.SysConfig
	c.Generate(&data)
	data.CreateBy = c.CreateBy
	err = e.Orm.Create(&data).Error
	if err != nil {
		e.Log.Errorf("Service InsertSysConfig error:%s", err)
		return err
	}
	c.Id = data.Id
	return nil
}

// Update 修改SysConfig对象
func (e *SysConfig) Update(c *dto.SysConfigControl) error {
	if c.Id <= 0 {
		return gorm.ErrRecordNotFound
	}
	var model models.SysConfig
	if err := e.Orm.First(&model, c.GetId()).Error; err != nil {
		return err
	}
	c.Generate(&model)
	model.UpdateBy = c.UpdateBy
	db := e.Orm.Omit("create_by", "created_at").Select("*").Updates(&model)
	if db.Error != nil {
		return db.Error
	}
	if db.RowsAffected == 0 {
		return errors.New("无权更新该数据")
	}
	return nil
}

// SetSysConfig 修改已有配置，保留跳过不存在配置键的历史行为。
func (e *SysConfig) SetSysConfig(c *[]dto.GetSetSysConfigReq) error {
	return e.setValues(c, true)
}

func (e *SysConfig) GetForSet(c *[]dto.GetSetSysConfigReq) error {
	var err error
	var data models.SysConfig

	err = e.Orm.Model(&data).
		Find(c).Error
	if err != nil {
		e.Log.Errorf("Service GetSysConfigPage error:%s", err)
		return err
	}
	return nil
}

// UpdateForSet 原子更新设置页中的全部配置，不存在的键视为错误。
func (e *SysConfig) UpdateForSet(c *[]dto.GetSetSysConfigReq) error {
	return e.setValues(c, false)
}

func (e *SysConfig) setValues(c *[]dto.GetSetSysConfigReq, skipMissing bool) error {
	return e.Orm.Transaction(func(tx *gorm.DB) error {
		for _, req := range *c {
			var model models.SysConfig
			if err := tx.Where("config_key = ?", req.ConfigKey).First(&model).Error; err != nil {
				if skipMissing && errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				return err
			}
			if model.ConfigValue == req.ConfigValue {
				continue
			}
			db := tx.Model(&model).Update("config_value", req.ConfigValue)
			if db.Error != nil {
				return db.Error
			}
			if db.RowsAffected == 0 {
				return errors.New("无权更新该数据")
			}
		}
		return nil
	})
}

// Remove 删除SysConfig
func (e *SysConfig) Remove(d *dto.SysConfigDeleteReq) error {
	var err error
	var data models.SysConfig

	db := e.Orm.Delete(&data, d.Ids)
	if err = db.Error; err != nil {
		e.Log.Errorf("Service RemoveSysConfig error:%s", err)
		return err
	}
	if db.RowsAffected == 0 {
		err = errors.New("无权删除该数据")
		return err
	}
	return nil
}

// GetWithKey 根据Key获取SysConfig
func (e *SysConfig) GetWithKey(c *dto.SysConfigByKeyReq, resp *dto.GetSysConfigByKEYForServiceResp) error {
	var err error
	var data models.SysConfig
	err = e.Orm.Model(&data).Where("config_key = ?", c.ConfigKey).First(resp).Error
	if err != nil {
		e.Log.Errorf("At Service GetSysConfigByKEY Error:%s", err)
		return err
	}

	return nil
}

func (e *SysConfig) GetWithKeyList(c *dto.SysConfigGetToSysAppReq, list *[]models.SysConfig) error {
	err := e.Orm.
		Scopes(
			cDto.MakeCondition(c.GetNeedSearch()),
		).
		Find(list).Error
	if err != nil {
		e.Log.Errorf("Service GetSysConfigByKey error:%s", err)
		return err
	}
	return nil
}
