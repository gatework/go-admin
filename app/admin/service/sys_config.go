package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/service"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
	cDto "github.com/gatework/motor/common/dto"
)

// Config coordinates administration queries and mutations.
type Config struct {
	service.Service
}

// UpdateExistingSettings 修改已有配置，保留跳过不存在配置键的历史行为。
func (e *Config) UpdateExistingSettings(ctx context.Context, c *[]dto.ConfigSetting) error {
	return e.setValues(ctx, c, true)
}

// Settings loads configuration values for the settings form.
func (e *Config) Settings(ctx context.Context, c *[]dto.ConfigSetting) error {
	var err error
	var data models.SysConfig

	err = e.DB.WithContext(ctx).Model(&data).
		Find(c).Error
	if err != nil {
		e.Logger.Errorf("loading settings: %s", err)
		return err
	}
	return nil
}

// ReplaceSettings 原子更新设置页中的全部配置，不存在的键视为错误。
func (e *Config) ReplaceSettings(ctx context.Context, c *[]dto.ConfigSetting) error {
	return e.setValues(ctx, c, false)
}

// ByKey 根据Key获取SysConfig
func (e *Config) ByKey(ctx context.Context, c *dto.ConfigByKey, resp *dto.ConfigEntry) error {
	var err error
	var data models.SysConfig
	err = e.DB.WithContext(ctx).Model(&data).Where("config_key = ?", c.ConfigKey).First(resp).Error
	if err != nil {
		e.Logger.Errorf("loading configuration by key: %s", err)
		return err
	}

	return nil
}

// Values loads the configuration values selected by their keys.
func (e *Config) Values(ctx context.Context, c *dto.ConfigVisibility, list *[]models.SysConfig) error {
	err := e.DB.WithContext(ctx).
		Scopes(
			cDto.MakeCondition(c.GetNeedSearch()),
		).
		Find(list).Error
	if err != nil {
		e.Logger.Errorf("loading configuration values: %s", err)
		return err
	}
	return nil
}

func (e *Config) setValues(ctx context.Context, c *[]dto.ConfigSetting, skipMissing bool) error {
	return e.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
