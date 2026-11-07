package service

import (
	"errors"
	"strings"

	log "github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg"
	"github.com/go-admin-team/go-admin-core/v2/sdk/service"
	"gorm.io/gorm"

	"go-admin/app/admin/models"
	"go-admin/app/admin/service/dto"
	cDto "go-admin/common/dto"
)

type SysDept struct {
	service.Service
}

// GetPage 获取SysDept列表
//func (e *SysDept) GetPage(c *dto.SysDeptGetPageReq, list *[]models.SysDept) error {
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
func (e *SysDept) Get(d *dto.SysDeptGetReq, model *models.SysDept) error {
	var err error
	var data models.SysDept

	err = e.Orm.Model(&data).
		FirstOrInit(model, d.GetId()).
		Error
	if err != nil {
		e.Log.Errorf("db error:%s", err)
		_ = e.AddError(err)
		return err
	}
	if model.DeptId == 0 {
		err = errors.New("查看对象不存在或无权查看")
		e.Log.Errorf("Service GetSysApi error: %s", err)
		_ = e.AddError(err)
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

// Insert 创建SysDept对象
func (e *SysDept) Insert(c *dto.SysDeptInsertReq) error {
	var data models.SysDept
	err := e.Orm.Transaction(func(tx *gorm.DB) error {
		if err := lockDeptTree(tx); err != nil {
			return err
		}
		c.Generate(&data)
		data.CreateBy = c.CreateBy
		parentPath, err := deptParentPath(tx, data.ParentId, data.DeptId)
		if err != nil {
			return err
		}
		if err := tx.Create(&data).Error; err != nil {
			return err
		}
		return tx.Model(&data).Update("dept_path", parentPath+pkg.IntToString(data.DeptId)+"/").Error
	})
	if err == nil {
		c.DeptId = data.DeptId
	}
	return err
}

// Update 修改SysDept对象及整棵子树的路径。
func (e *SysDept) Update(c *dto.SysDeptUpdateReq) error {
	return e.Orm.Transaction(func(tx *gorm.DB) error {
		if err := lockDeptTree(tx); err != nil {
			return err
		}
		var model models.SysDept
		if err := tx.First(&model, c.GetId()).Error; err != nil {
			return err
		}
		oldPath := model.DeptPath
		if oldPath == "" {
			return errors.New("部门路径为空")
		}
		parentPath, err := deptParentPath(tx, c.ParentId, model.DeptId)
		if err != nil {
			return err
		}
		c.Generate(&model)
		model.DeptPath = parentPath + pkg.IntToString(model.DeptId) + "/"
		model.UpdateBy = c.UpdateBy
		if err := tx.Select("*").Updates(&model).Error; err != nil {
			return err
		}
		if oldPath == model.DeptPath {
			return nil
		}
		var descendants []models.SysDept
		if err := tx.Where("dept_path LIKE ? AND dept_id <> ?", oldPath+"%", model.DeptId).Find(&descendants).Error; err != nil {
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

// Remove 删除SysDept
func (e *SysDept) Remove(d *dto.SysDeptDeleteReq) error {
	var err error
	var data models.SysDept

	db := e.Orm.Model(&data).Delete(&data, d.GetId())
	if err = db.Error; err != nil {
		e.Log.Errorf("Delete error: %s", err)
		return err
	}
	if db.RowsAffected == 0 {
		err = errors.New("无权删除该数据")
		return err
	}
	return nil
}

// GetSysDeptList 获取组织数据
func (e *SysDept) getList(c *dto.SysDeptGetPageReq, list *[]models.SysDept) error {
	var err error
	var data models.SysDept

	err = e.Orm.Model(&data).
		Scopes(
			cDto.MakeCondition(c.GetNeedSearch()),
		).
		Find(list).Error
	if err != nil {
		e.Log.Errorf("db error:%s", err)
		return err
	}
	return nil
}

// SetDeptTree 设置组织数据
func (e *SysDept) SetDeptTree(c *dto.SysDeptGetPageReq) (m []dto.DeptLabel, err error) {
	var list []models.SysDept
	err = e.getList(c, &list)

	m = make([]dto.DeptLabel, 0)
	for i := 0; i < len(list); i++ {
		if list[i].ParentId != 0 {
			continue
		}
		e := dto.DeptLabel{}
		e.Id = list[i].DeptId
		e.Label = list[i].DeptName
		deptsInfo := deptTreeCall(&list, e)

		m = append(m, deptsInfo)
	}
	return
}

// Call 递归构造组织数据
func deptTreeCall(deptList *[]models.SysDept, dept dto.DeptLabel) dto.DeptLabel {
	list := *deptList
	childrenList := make([]dto.DeptLabel, 0)
	for j := 0; j < len(list); j++ {
		if dept.Id != list[j].ParentId {
			continue
		}
		mi := dto.DeptLabel{Id: list[j].DeptId, Label: list[j].DeptName, Children: []dto.DeptLabel{}}
		ms := deptTreeCall(deptList, mi)
		childrenList = append(childrenList, ms)
	}
	dept.Children = childrenList
	return dept
}

// SetDeptPage 设置dept页面数据
func (e *SysDept) SetDeptPage(c *dto.SysDeptGetPageReq) (m []models.SysDept, err error) {
	var list []models.SysDept
	err = e.getList(c, &list)
	for i := 0; i < len(list); i++ {
		if list[i].ParentId != 0 {
			continue
		}
		info := e.deptPageCall(&list, list[i])
		m = append(m, info)
	}
	return
}

func (e *SysDept) deptPageCall(deptlist *[]models.SysDept, menu models.SysDept) models.SysDept {
	list := *deptlist
	childrenList := make([]models.SysDept, 0)
	for j := 0; j < len(list); j++ {
		if menu.DeptId != list[j].ParentId {
			continue
		}
		mi := models.SysDept{}
		mi.DeptId = list[j].DeptId
		mi.ParentId = list[j].ParentId
		mi.DeptPath = list[j].DeptPath
		mi.DeptName = list[j].DeptName
		mi.Sort = list[j].Sort
		mi.Leader = list[j].Leader
		mi.Phone = list[j].Phone
		mi.Email = list[j].Email
		mi.Status = list[j].Status
		mi.CreatedAt = list[j].CreatedAt
		mi.Children = []models.SysDept{}
		ms := e.deptPageCall(deptlist, mi)
		childrenList = append(childrenList, ms)
	}
	menu.Children = childrenList
	return menu
}

// GetWithRoleId 获取角色的部门ID集合
func (e *SysDept) GetWithRoleId(roleId int) ([]int, error) {
	deptIds := make([]int, 0)
	deptList := make([]dto.DeptIdList, 0)
	if err := e.Orm.Table("sys_role_dept").
		Select("sys_role_dept.dept_id").
		Joins("LEFT JOIN sys_dept on sys_dept.dept_id=sys_role_dept.dept_id").
		Where("role_id = ? ", roleId).
		Where(" sys_role_dept.dept_id not in(select sys_dept.parent_id from sys_role_dept LEFT JOIN sys_dept on sys_dept.dept_id=sys_role_dept.dept_id where role_id =? )", roleId).
		Find(&deptList).Error; err != nil {
		return nil, err
	}
	for i := 0; i < len(deptList); i++ {
		deptIds = append(deptIds, deptList[i].DeptId)
	}
	return deptIds, nil
}

func (e *SysDept) SetDeptLabel() (m []dto.DeptLabel, err error) {
	list := make([]models.SysDept, 0)
	err = e.Orm.Find(&list).Error
	if err != nil {
		log.Errorf("find dept list error, %s", err.Error())
		return
	}
	m = make([]dto.DeptLabel, 0)
	var item dto.DeptLabel
	for i := range list {
		if list[i].ParentId != 0 {
			continue
		}
		item = dto.DeptLabel{}
		item.Id = list[i].DeptId
		item.Label = list[i].DeptName
		deptInfo := deptLabelCall(&list, item)
		m = append(m, deptInfo)
	}
	return
}

// deptLabelCall
func deptLabelCall(deptList *[]models.SysDept, dept dto.DeptLabel) dto.DeptLabel {
	list := *deptList
	var mi dto.DeptLabel
	childrenList := make([]dto.DeptLabel, 0)
	for j := 0; j < len(list); j++ {
		if dept.Id != list[j].ParentId {
			continue
		}
		mi = dto.DeptLabel{Id: list[j].DeptId, Label: list[j].DeptName, Children: []dto.DeptLabel{}}
		ms := deptLabelCall(deptList, mi)
		childrenList = append(childrenList, ms)
	}
	dept.Children = childrenList
	return dept
}
