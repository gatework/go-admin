package service

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/gatework/motor-core/sdk/service"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
	"github.com/gatework/motor/common/actions"
	cDto "github.com/gatework/motor/common/dto"
)

// Route coordinates administration queries and mutations.
type Route struct {
	service.Service
}

// List 获取SysAPI列表
func (e *Route) List(ctx context.Context, c *dto.RouteSearch, p *actions.DataPermission, list *[]models.SysAPI, count *int64) error {
	var err error
	var data models.SysAPI

	orm := e.DB.WithContext(ctx).Debug().Model(&data).
		Scopes(
			cDto.MakeCondition(c.GetNeedSearch()),
			cDto.Paginate(c.GetPageSize(), c.GetPageIndex()),
			actions.Permission(data.TableName(), p),
		)
	if c.Type != "" {
		qType := c.Type
		if qType == "暂无" {
			qType = ""
		}
		orm = orm.Where(clause.Eq{Column: clause.Column{Name: "type"}, Value: qType})

	}
	err = orm.Find(list).Limit(-1).Offset(-1).
		Count(count).Error
	if err != nil {
		e.Logger.Errorf("listing routes: %s", err)
		return err
	}
	return nil
}

// Get 获取SysAPI对象with id
func (e *Route) Get(ctx context.Context, d *dto.RouteByID, p *actions.DataPermission, model *models.SysAPI) error {
	var data models.SysAPI
	err := e.DB.WithContext(ctx).Model(&data).
		Scopes(
			actions.Permission(data.TableName(), p),
		).
		FirstOrInit(model, d.GetID()).Error
	if err != nil {
		e.Logger.Errorf("db error:%s", err)
		return err
	}
	if model.ID == 0 {
		err = errors.New("查看对象不存在或无权查看")
		e.Logger.Errorf("loading route: %s", err)
		return err
	}
	return nil
}

// Update 修改SysAPI对象
func (e *Route) Update(ctx context.Context, c *dto.RouteUpdate, p *actions.DataPermission) error {
	var model = models.SysAPI{}
	db := e.DB.WithContext(ctx).Scopes(
		actions.Permission(model.TableName(), p),
	).First(&model, c.GetID())
	if err := db.Error; err != nil {
		// First reports a row the data permission excluded exactly as it
		// reports one that does not exist, and the caller should not be able
		// to tell those apart either.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("无权更新该数据")
		}
		e.Logger.Errorf("updating route: %s", err)
		return err
	}
	c.Generate(&model)
	model.UpdateBy = c.UpdateBy
	db = e.DB.WithContext(ctx).Scopes(actions.Permission(model.TableName(), p)).
		Omit("create_by", "created_at").Select("*").Updates(&model)
	if err := db.Error; err != nil {
		e.Logger.Errorf("updating route: %s", err)
		return err
	}

	if db.RowsAffected == 0 {
		return errors.New("无权更新该数据")
	}
	return nil
}
