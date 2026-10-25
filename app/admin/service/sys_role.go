package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/casbin/casbin/v3"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"go-admin/app/admin/models"
	"go-admin/app/admin/service/dto"
	cDto "go-admin/common/dto"
)

type SysRole struct {
	service.Service
}

// GetPage 获取SysRole列表
func (e *SysRole) GetPage(c *dto.SysRoleGetPageReq, list *[]models.SysRole, count *int64) error {
	var err error
	var data models.SysRole

	err = e.Orm.Model(&data).Preload("SysMenu").
		Scopes(
			cDto.MakeCondition(c.GetNeedSearch()),
			cDto.Paginate(c.GetPageSize(), c.GetPageIndex()),
		).
		Find(list).Limit(-1).Offset(-1).
		Count(count).Error
	if err != nil {
		e.Log.Errorf("db error:%s", err)
		return err
	}
	return nil
}

// Get 获取SysRole对象
func (e *SysRole) Get(d *dto.SysRoleGetReq, model *models.SysRole) error {
	var err error
	db := e.Orm.First(model, d.GetId())
	err = db.Error
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		err = errors.New("查看对象不存在或无权查看")
		e.Log.Errorf("db error:%s", err)
		return err
	}
	if err != nil {
		e.Log.Errorf("db error:%s", err)
		return err
	}
	model.MenuIds, err = e.GetRoleMenuId(model.RoleId)
	if err != nil {
		e.Log.Errorf("get menuIds error, %s", err.Error())
		return err
	}
	return nil
}

// Insert 创建SysRole对象
func (e *SysRole) Insert(c *dto.SysRoleInsertReq, cb *casbin.SyncedEnforcer) error {
	if strings.TrimSpace(c.RoleKey) == "" {
		return errors.New("角色标识不能为空")
	}
	var data models.SysRole
	err := e.Orm.Transaction(func(tx *gorm.DB) error {
		var menus []models.SysMenu
		if err := tx.Preload("SysApi").Where("menu_id IN ?", c.MenuIds).Find(&menus).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&data).Where("role_key = ?", c.RoleKey).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("roleKey已存在，需更换在提交！")
		}
		c.SysMenu = menus
		c.Generate(&data)
		data.CreateBy = c.CreateBy
		if err := tx.Create(&data).Error; err != nil {
			return err
		}
		return replaceRolePolicies(tx, data.RoleKey, data.RoleKey, menus)
	})
	if err != nil {
		return err
	}
	c.RoleId = data.RoleId
	return reloadRolePolicies(cb)
}

// Update 修改SysRole对象
func (e *SysRole) Update(c *dto.SysRoleUpdateReq, cb *casbin.SyncedEnforcer) error {
	if strings.TrimSpace(c.RoleKey) == "" {
		return errors.New("角色标识不能为空")
	}
	err := e.Orm.Transaction(func(tx *gorm.DB) error {
		var model models.SysRole
		if err := lockAdminRows(tx, (&models.SysRole{}).TableName()).First(&model, c.GetId()).Error; err != nil {
			return err
		}
		oldKey := model.RoleKey
		oldStatus := model.Status
		oldScope := model.DataScope
		var count int64
		if err := tx.Model(&models.SysRole{}).Where("role_key = ? AND role_id <> ?", c.RoleKey, model.RoleId).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errors.New("roleKey已存在，需更换在提交！")
		}
		var menus []models.SysMenu
		if err := tx.Preload("SysApi").Where("menu_id IN ?", c.MenuIds).Find(&menus).Error; err != nil {
			return err
		}
		c.Generate(&model)
		if oldStatus != model.Status || oldKey != model.RoleKey || oldScope != model.DataScope {
			if err := revokeRoleSessions(tx, model.RoleId); err != nil {
				return err
			}
		}
		model.UpdateBy = c.UpdateBy
		if err := tx.Omit(clause.Associations).Select("*").Updates(&model).Error; err != nil {
			return err
		}
		if err := tx.Model(&model).Association("SysMenu").Replace(menus); err != nil {
			return err
		}
		return replaceRolePolicies(tx, oldKey, model.RoleKey, menus)
	})
	if err != nil {
		return err
	}
	return reloadRolePolicies(cb)
}

// replaceRolePolicies 将权限与角色关联写入同一事务，不通过另开连接的 adapter 写入。
func replaceRolePolicies(tx *gorm.DB, oldKey, roleKey string, menus []models.SysMenu) error {
	if err := tx.Where("ptype = ? AND v0 IN ?", "p", []string{oldKey, roleKey}).Delete(&gormadapter.CasbinRule{}).Error; err != nil {
		return err
	}
	policies := make([]gormadapter.CasbinRule, 0)
	seen := make(map[[2]string]bool)
	for _, menu := range menus {
		for _, api := range menu.SysApi {
			key := [2]string{api.Path, api.Action}
			if seen[key] {
				continue
			}
			seen[key] = true
			policies = append(policies, gormadapter.CasbinRule{Ptype: "p", V0: roleKey, V1: api.Path, V2: api.Action})
		}
	}
	if len(policies) == 0 {
		return nil
	}
	return tx.Create(&policies).Error
}

// reloadRolePolicies 提交后刷新内存权限；刷新失败时清空缓存，避免继续使用旧授权。
func reloadRolePolicies(cb *casbin.SyncedEnforcer) error {
	if cb == nil || cb.GetAdapter() == nil {
		return nil
	}
	if err := cb.LoadPolicy(); err != nil {
		cb.ClearPolicy()
		return fmt.Errorf("role changes committed but policy reload failed: %w", err)
	}
	return nil
}

// Remove 删除SysRole
func (e *SysRole) Remove(c *dto.SysRoleDeleteReq, cb *casbin.SyncedEnforcer) error {
	err := e.Orm.Transaction(func(tx *gorm.DB) error {
		var roles []models.SysRole
		if err := lockAdminRows(tx, (&models.SysRole{}).TableName()).Where("role_id IN ?", c.Ids).Order("role_id").Find(&roles).Error; err != nil {
			return err
		}
		if len(roles) == 0 {
			return errors.New("无权删除该数据")
		}
		for i := range roles {
			if err := tx.Select(clause.Associations).Delete(&roles[i]).Error; err != nil {
				return err
			}
			if err := tx.Where("ptype = ? AND v0 = ?", "p", roles[i].RoleKey).Delete(&gormadapter.CasbinRule{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return reloadRolePolicies(cb)
}

// GetRoleMenuId 获取角色对应的菜单ids
func (e *SysRole) GetRoleMenuId(roleId int) ([]int, error) {
	menuIds := make([]int, 0)
	model := models.SysRole{}
	model.RoleId = roleId
	if err := e.Orm.Model(&model).Preload("SysMenu").First(&model).Error; err != nil {
		return nil, err
	}
	l := *model.SysMenu
	for i := 0; i < len(l); i++ {
		menuIds = append(menuIds, l[i].MenuId)
	}
	return menuIds, nil
}

func (e *SysRole) UpdateDataScope(c *dto.RoleDataScopeReq) *SysRole {
	err := e.Orm.Transaction(func(tx *gorm.DB) error {
		var model models.SysRole
		if err := lockAdminRows(tx, (&models.SysRole{}).TableName()).Preload("SysDept").First(&model, c.RoleId).Error; err != nil {
			return err
		}
		var depts []models.SysDept
		if err := tx.Where("dept_id IN ?", c.DeptIds).Find(&depts).Error; err != nil {
			return err
		}
		previous := make(map[int]bool, len(model.SysDept))
		for _, dept := range model.SysDept {
			previous[dept.DeptId] = true
		}
		changed := len(previous) != len(depts)
		for _, dept := range depts {
			if !previous[dept.DeptId] {
				changed = true
			}
		}
		if model.DataScope != c.DataScope || changed {
			if err := revokeRoleSessions(tx, model.RoleId); err != nil {
				return err
			}
		}
		if err := tx.Model(&model).Update("data_scope", c.DataScope).Error; err != nil {
			return err
		}
		return tx.Model(&model).Association("SysDept").Replace(depts)
	})
	if err != nil {
		_ = e.AddError(err)
	}
	return e
}

// UpdateStatus 修改SysRole对象status
func (e *SysRole) UpdateStatus(c *dto.UpdateStatusReq) error {
	if c.RoleId <= 0 || (c.Status != "1" && c.Status != "2") {
		return errors.New("invalid role status")
	}
	return e.Orm.Transaction(func(tx *gorm.DB) error {
		var model models.SysRole
		if err := lockAdminRows(tx, model.TableName()).First(&model, c.RoleId).Error; err != nil {
			return err
		}
		if model.Status != c.Status {
			if err := revokeRoleSessions(tx, c.RoleId); err != nil {
				return err
			}
		}
		return tx.Model(&model).Updates(map[string]interface{}{"status": c.Status, "update_by": c.UpdateBy}).Error
	})
}

// Role state and assigned accounts' session versions commit together. This
// prevents a disabled role's old sessions reviving when it is re-enabled.
func revokeRoleSessions(tx *gorm.DB, roleID int) error {
	return tx.Table("sys_user").Where("role_id = ? AND deleted_at = 0", roleID).
		UpdateColumn("auth_version", gorm.Expr("auth_version + ?", 1)).Error
}

// GetWithName 获取SysRole对象
func (e *SysRole) GetWithName(d *dto.SysRoleByName, model *models.SysRole) *SysRole {
	var err error
	db := e.Orm.Where("role_name = ?", d.RoleName).First(model)
	err = db.Error
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		err = errors.New("查看对象不存在或无权查看")
		e.Log.Errorf("db error:%s", err)
		_ = e.AddError(err)
		return e
	}
	if err != nil {
		e.Log.Errorf("db error:%s", err)
		_ = e.AddError(err)
		return e
	}
	model.MenuIds, err = e.GetRoleMenuId(model.RoleId)
	if err != nil {
		e.Log.Errorf("get menuIds error, %s", err.Error())
		_ = e.AddError(err)
		return e
	}
	return e
}

// GetById 获取SysRole对象
func (e *SysRole) GetById(roleId int) ([]string, error) {
	permissions := make([]string, 0)
	model := models.SysRole{}
	model.RoleId = roleId
	if err := e.Orm.Model(&model).Preload("SysMenu").First(&model).Error; err != nil {
		return nil, err
	}
	l := *model.SysMenu
	for i := 0; i < len(l); i++ {
		if l[i].Permission != "" {
			permissions = append(permissions, l[i].Permission)
		}
	}
	return permissions, nil
}
