package service

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	log "github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/pkg"
	"github.com/gatework/motor-core/sdk/service"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
	cDto "github.com/gatework/motor/common/dto"
)

// Department coordinates administration queries and mutations.
type Department struct {
	service.Service
}

// GetPage 获取SysDept列表
//func (e *SysDept) GetPage(c *dto.DepartmentSearch, list *[]models.SysDept) error {
//	var err error
//	var data models.SysDept
//
//	err = e.Orm.Model(&data).
//		Scopes(
//			cDto.MakeCondition(c.GetNeedSearch()),
//		).
//		Find(list).Error
//	if err != nil {
//		e.Log.Errorf("db error:%s", err)
//		return err
//	}
//	return nil
//}

// Get 获取SysDept对象
func (e *Department) Get(ctx context.Context, d *dto.DepartmentByID, model *models.SysDept) error {
	var err error
	var data models.SysDept

	err = e.DB.WithContext(ctx).Model(&data).
		FirstOrInit(model, d.GetID()).
		Error
	if err != nil {
		e.Logger.Errorf("db error:%s", err)

		return err
	}
	if model.DeptID == 0 {
		err = errors.New("查看对象不存在或无权查看")
		e.Logger.Errorf("loading department: %s", err)

		return err
	}
	return nil
}

// lockDeptTree 按固定顺序锁定树节点；后续查询使用加锁完成后的最新路径。
func lockDeptTree(tx *gorm.DB) error {
	var ids []models.SysDept
	return lockAdminRows(tx, (&models.SysDept{}).TableName()).Select("dept_id").Order("dept_id").Find(&ids).Error
}

// deptParentPath 验证父节点存在，拒绝自指和后代作为父节点。
func deptParentPath(tx *gorm.DB, parentID, deptID int) (string, error) {
	if parentID == 0 {
		return "/0/", nil
	}
	if parentID == deptID {
		return "", errors.New("部门不能设置自身为父级")
	}
	var parent models.SysDept
	if err := tx.First(&parent, parentID).Error; err != nil {
		return "", err
	}
	if parent.DeptPath == "" {
		return "", errors.New("父级部门路径为空")
	}
	if deptID != 0 && strings.Contains(parent.DeptPath, "/"+pkg.IntToString(deptID)+"/") {
		return "", errors.New("部门不能移动到自己的子部门")
	}
	return parent.DeptPath, nil
}

// Create 创建SysDept对象
func (e *Department) Create(ctx context.Context, c *dto.DepartmentCreate) error {
	var data models.SysDept
	err := e.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDeptTree(tx); err != nil {
			return err
		}
		c.Generate(&data)
		data.CreateBy = c.CreateBy
		parentPath, err := deptParentPath(tx, data.ParentID, data.DeptID)
		if err != nil {
			return err
		}
		if err := tx.Create(&data).Error; err != nil {
			return err
		}
		return tx.Model(&data).Update("dept_path", parentPath+pkg.IntToString(data.DeptID)+"/").Error
	})
	if err == nil {
		c.DeptID = data.DeptID
	}
	return err
}

// Update 修改SysDept对象及整棵子树的路径。
func (e *Department) Update(ctx context.Context, c *dto.DepartmentUpdate) error {
	return e.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockDeptTree(tx); err != nil {
			return err
		}
		var model models.SysDept
		if err := tx.First(&model, c.GetID()).Error; err != nil {
			return err
		}
		oldPath := model.DeptPath
		if oldPath == "" {
			return errors.New("部门路径为空")
		}
		parentPath, err := deptParentPath(tx, c.ParentID, model.DeptID)
		if err != nil {
			return err
		}
		c.Generate(&model)
		model.DeptPath = parentPath + pkg.IntToString(model.DeptID) + "/"
		model.UpdateBy = c.UpdateBy
		if err := tx.Select("*").Updates(&model).Error; err != nil {
			return err
		}
		if oldPath == model.DeptPath {
			return nil
		}
		var descendants []models.SysDept
		if err := tx.Where("dept_path LIKE ? AND dept_id <> ?", oldPath+"%", model.DeptID).Find(&descendants).Error; err != nil {
			return err
		}
		for _, child := range descendants {
			path := model.DeptPath + strings.TrimPrefix(child.DeptPath, oldPath)
			if err := tx.Model(&child).Update("dept_path", path).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Delete 删除SysDept
func (e *Department) Delete(ctx context.Context, d *dto.DepartmentDelete) error {
	var err error
	var data models.SysDept

	db := e.DB.WithContext(ctx).Model(&data).Delete(&data, d.GetID())
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

// Tree 设置组织数据
func (e *Department) Tree(ctx context.Context, c *dto.DepartmentSearch) (m []dto.DepartmentLabel, err error) {
	var list []models.SysDept
	if err = e.getList(ctx, c, &list); err != nil {
		return nil, err
	}
	return forest(list, departmentID, departmentParent, always[models.SysDept], departmentLabel), nil
}

// List 设置dept页面数据
func (e *Department) List(ctx context.Context, c *dto.DepartmentSearch) (m []models.SysDept, err error) {
	var list []models.SysDept
	if err = e.getList(ctx, c, &list); err != nil {
		return nil, err
	}
	return forest(list, departmentID, departmentParent, always[models.SysDept], func(d models.SysDept, children []models.SysDept) models.SysDept {
		d.Children = children
		return d
	}), nil
}

// IDsForRole performs the get with role id administration operation.
func (e *Department) IDsForRole(ctx context.Context, roleID int) ([]int, error) {
	deptIDs := make([]int, 0)
	deptList := make([]dto.DepartmentRef, 0)
	if err := e.DB.WithContext(ctx).Table("sys_role_dept").
		Select("sys_role_dept.dept_id").
		Joins("LEFT JOIN sys_dept on sys_dept.dept_id=sys_role_dept.dept_id").
		Where("role_id = ? ", roleID).
		Where(" sys_role_dept.dept_id not in(select sys_dept.parent_id from sys_role_dept LEFT JOIN sys_dept on sys_dept.dept_id=sys_role_dept.dept_id where role_id =? )", roleID).
		Find(&deptList).Error; err != nil {
		return nil, err
	}
	for i := range deptList {
		deptIDs = append(deptIDs, deptList[i].DeptID)
	}
	return deptIDs, nil
}

// Labels sets the department display label.
func (e *Department) Labels(ctx context.Context) (m []dto.DepartmentLabel, err error) {
	list := make([]models.SysDept, 0)
	err = e.DB.WithContext(ctx).Find(&list).Error
	if err != nil {
		log.NewHelper(log.Default()).Errorf("find dept list error, %s", err.Error())
		return
	}
	return forest(list, departmentID, departmentParent, always[models.SysDept], departmentLabel), nil
}

// GetSysDeptList 获取组织数据
func (e *Department) getList(ctx context.Context, c *dto.DepartmentSearch, list *[]models.SysDept) error {
	var err error
	var data models.SysDept

	err = e.DB.WithContext(ctx).Model(&data).
		Scopes(
			cDto.MakeCondition(c.GetNeedSearch()),
		).
		Find(list).Error
	if err != nil {
		e.Logger.Errorf("db error:%s", err)
		return err
	}
	return nil
}

func departmentID(d models.SysDept) int     { return d.DeptID }
func departmentParent(d models.SysDept) int { return d.ParentID }

func departmentLabel(d models.SysDept, children []dto.DepartmentLabel) dto.DepartmentLabel {
	return dto.DepartmentLabel{ID: d.DeptID, Label: d.DeptName, Children: children}
}
