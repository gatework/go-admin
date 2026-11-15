package service

import (
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
func (e *Department) Get(d *dto.DepartmentByID, model *models.SysDept) error {
	var err error
	var data models.SysDept

	err = e.DB.Model(&data).
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
func (e *Department) Create(c *dto.DepartmentCreate) error {
	var data models.SysDept
	err := e.DB.Transaction(func(tx *gorm.DB) error {
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
func (e *Department) Update(c *dto.DepartmentUpdate) error {
	return e.DB.Transaction(func(tx *gorm.DB) error {
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
func (e *Department) Delete(d *dto.DepartmentDelete) error {
	var err error
	var data models.SysDept

	db := e.DB.Model(&data).Delete(&data, d.GetID())
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
func (e *Department) Tree(c *dto.DepartmentSearch) (m []dto.DepartmentLabel, err error) {
	var list []models.SysDept
	err = e.getList(c, &list)

	m = make([]dto.DepartmentLabel, 0)
	for i := range list {
		if list[i].ParentID != 0 {
			continue
		}
		e := dto.DepartmentLabel{}
		e.ID = list[i].DeptID
		e.Label = list[i].DeptName
		deptsInfo := deptTreeCall(&list, e)

		m = append(m, deptsInfo)
	}
	return
}

// Call 递归构造组织数据
func deptTreeCall(deptList *[]models.SysDept, dept dto.DepartmentLabel) dto.DepartmentLabel {
	list := *deptList
	childrenList := make([]dto.DepartmentLabel, 0)
	for j := range list {
		if dept.ID != list[j].ParentID {
			continue
		}
		mi := dto.DepartmentLabel{ID: list[j].DeptID, Label: list[j].DeptName, Children: []dto.DepartmentLabel{}}
		ms := deptTreeCall(deptList, mi)
		childrenList = append(childrenList, ms)
	}
	dept.Children = childrenList
	return dept
}

// List 设置dept页面数据
func (e *Department) List(c *dto.DepartmentSearch) (m []models.SysDept, err error) {
	var list []models.SysDept
	err = e.getList(c, &list)
	for i := range list {
		if list[i].ParentID != 0 {
			continue
		}
		info := e.deptPageCall(&list, list[i])
		m = append(m, info)
	}
	return
}

// IDsForRole performs the get with role id administration operation.
func (e *Department) IDsForRole(roleID int) ([]int, error) {
	deptIDs := make([]int, 0)
	deptList := make([]dto.DepartmentRef, 0)
	if err := e.DB.Table("sys_role_dept").
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
func (e *Department) Labels() (m []dto.DepartmentLabel, err error) {
	list := make([]models.SysDept, 0)
	err = e.DB.Find(&list).Error
	if err != nil {
		log.NewHelper(log.Default()).Errorf("find dept list error, %s", err.Error())
		return
	}
	m = make([]dto.DepartmentLabel, 0)
	var item dto.DepartmentLabel
	for i := range list {
		if list[i].ParentID != 0 {
			continue
		}
		item = dto.DepartmentLabel{}
		item.ID = list[i].DeptID
		item.Label = list[i].DeptName
		deptInfo := deptLabelCall(&list, item)
		m = append(m, deptInfo)
	}
	return
}

// deptLabelCall
func deptLabelCall(deptList *[]models.SysDept, dept dto.DepartmentLabel) dto.DepartmentLabel {
	list := *deptList
	var mi dto.DepartmentLabel
	childrenList := make([]dto.DepartmentLabel, 0)
	for j := range list {
		if dept.ID != list[j].ParentID {
			continue
		}
		mi = dto.DepartmentLabel{ID: list[j].DeptID, Label: list[j].DeptName, Children: []dto.DepartmentLabel{}}
		ms := deptLabelCall(deptList, mi)
		childrenList = append(childrenList, ms)
	}
	dept.Children = childrenList
	return dept
}

// GetSysDeptList 获取组织数据
func (e *Department) getList(c *dto.DepartmentSearch, list *[]models.SysDept) error {
	var err error
	var data models.SysDept

	err = e.DB.Model(&data).
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

func (e *Department) deptPageCall(deptlist *[]models.SysDept, menu models.SysDept) models.SysDept {
	list := *deptlist
	childrenList := make([]models.SysDept, 0)
	for j := range list {
		if menu.DeptID != list[j].ParentID {
			continue
		}
		mi := models.SysDept{
			DeptID:    list[j].DeptID,
			ParentID:  list[j].ParentID,
			DeptPath:  list[j].DeptPath,
			DeptName:  list[j].DeptName,
			Sort:      list[j].Sort,
			Leader:    list[j].Leader,
			Phone:     list[j].Phone,
			Email:     list[j].Email,
			Status:    list[j].Status,
			CreatedAt: list[j].CreatedAt,
			Children:  []models.SysDept{}}
		ms := e.deptPageCall(deptlist, mi)
		childrenList = append(childrenList, ms)
	}
	menu.Children = childrenList
	return menu
}
