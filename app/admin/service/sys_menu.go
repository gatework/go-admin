package service

import (
	"fmt"
	"sort"
	"strings"

	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/gatework/motor-core/sdk/pkg"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
	cDto "github.com/gatework/motor/common/dto"
	cModels "github.com/gatework/motor/common/models"

	"github.com/gatework/motor-core/sdk/service"
)

// SysMenu coordinates administration queries and mutations.
type SysMenu struct {
	service.Service
}

// GetPage 获取SysMenu列表
func (e *SysMenu) GetPage(c *dto.SysMenuGetPageReq, menus *[]models.SysMenu) error {
	var menu = make([]models.SysMenu, 0)
	err := e.getPage(c, &menu)
	if err != nil {
		return err
	}
	for i := range menu {
		if menu[i].ParentID != 0 {
			continue
		}
		menusInfo := menuCall(&menu, menu[i])
		*menus = append(*menus, menusInfo)
	}
	return nil
}

// Get 获取SysMenu对象
func (e *SysMenu) Get(d *dto.SysMenuGetReq, model *models.SysMenu) error {
	var err error
	var data models.SysMenu

	db := e.DB.Model(&data).Preload("SysAPI").
		First(model, d.GetID())
	err = db.Error
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		err = errors.New("查看对象不存在或无权查看")
		e.Logger.Errorf("GetSysMenu error:%s", err)
		return err
	}
	if err != nil {
		e.Logger.Errorf("db error:%s", err)
		return err
	}
	apis := make([]int, 0)
	for _, v := range model.SysAPI {
		apis = append(apis, v.ID)
	}
	model.APIs = apis
	return nil
}

// lockMenuTree 固定锁顺序，防止并发移动让父子关系与路径不一致。
func lockMenuTree(tx *gorm.DB) error {
	var menus []models.SysMenu
	return lockAdminRows(tx, (&models.SysMenu{}).TableName()).Select("menu_id").Order("menu_id").Find(&menus).Error
}

// Insert 创建SysMenu对象
func (e *SysMenu) Insert(c *dto.SysMenuInsertReq) error {
	var data models.SysMenu
	err := e.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockMenuTree(tx); err != nil {
			return err
		}
		c.Generate(&data)
		if err := tx.Where("id IN ?", c.APIs).Find(&data.SysAPI).Error; err != nil {
			return err
		}
		if err := tx.Create(&data).Error; err != nil {
			return err
		}
		return e.initPaths(tx, &data)
	})
	if err != nil {
		return err
	}
	c.MenuID = data.MenuID
	return nil
}

// Update 修改SysMenu对象
func (e *SysMenu) Update(c *dto.SysMenuUpdateReq) error {
	err := e.DB.Transaction(func(tx *gorm.DB) error {
		if err := lockMenuTree(tx); err != nil {
			return err
		}
		var model models.SysMenu
		if err := tx.First(&model, c.GetID()).Error; err != nil {
			return err
		}
		oldPath := model.Paths
		if oldPath == "" {
			return errors.New("菜单路径为空")
		}
		var apis []models.SysAPI
		if err := tx.Where("id IN ?", c.APIs).Find(&apis).Error; err != nil {
			return err
		}
		c.Generate(&model)
		if err := e.initPaths(tx, &model); err != nil {
			return err
		}
		if err := tx.Omit(clause.Associations, "create_by").Select("*").Updates(&model).Error; err != nil {
			return err
		}
		if err := tx.Model(&model).Association("SysAPI").Replace(apis); err != nil {
			return err
		}
		if oldPath == model.Paths {
			return nil
		}
		var descendants []models.SysMenu
		if err := tx.Where("paths LIKE ?", oldPath+"/%").Find(&descendants).Error; err != nil {
			return err
		}
		for _, child := range descendants {
			path := model.Paths + strings.TrimPrefix(child.Paths, oldPath)
			if err := tx.Model(&child).Update("paths", path).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// Remove 删除SysMenu
func (e *SysMenu) Remove(d *dto.SysMenuDeleteReq) error {
	var err error
	var data models.SysMenu

	db := e.DB.Model(&data).Delete(&data, d.IDs)
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

// GetList 获取菜单数据
func (e *SysMenu) GetList(c *dto.SysMenuGetPageReq, list *[]models.SysMenu) error {
	var err error
	var data models.SysMenu

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

// SetLabel 修改角色中 设置菜单基础数据
func (e *SysMenu) SetLabel() (m []dto.MenuLabel, err error) {
	var list []models.SysMenu
	err = e.GetList(&dto.SysMenuGetPageReq{}, &list)

	m = make([]dto.MenuLabel, 0)
	for i := range list {
		if list[i].ParentID != 0 {
			continue
		}
		e := dto.MenuLabel{}
		e.ID = list[i].MenuID
		e.Label = list[i].Title
		deptsInfo := menuLabelCall(&list, e)

		m = append(m, deptsInfo)
	}
	return
}

// menuLabelCall 递归构造组织数据
func menuLabelCall(eList *[]models.SysMenu, dept dto.MenuLabel) dto.MenuLabel {
	list := *eList

	children := make([]dto.MenuLabel, 0)
	for j := range list {

		if dept.ID != list[j].ParentID {
			continue
		}
		mi := dto.MenuLabel{}
		mi.ID = list[j].MenuID
		mi.Label = list[j].Title
		mi.Children = []dto.MenuLabel{}
		if list[j].MenuType != "F" {
			ms := menuLabelCall(eList, mi)
			children = append(children, ms)
		} else {
			children = append(children, mi)
		}
	}
	if len(children) > 0 {
		dept.Children = children
	} else {
		dept.Children = nil
	}
	return dept
}

// menuCall 构建菜单树
func menuCall(menuList *[]models.SysMenu, menu models.SysMenu) models.SysMenu {
	list := *menuList

	children := make([]models.SysMenu, 0)
	for j := range list {

		if menu.MenuID != list[j].ParentID {
			continue
		}
		mi := models.SysMenu{
			MenuID:     list[j].MenuID,
			MenuName:   list[j].MenuName,
			Title:      list[j].Title,
			Icon:       list[j].Icon,
			Path:       list[j].Path,
			MenuType:   list[j].MenuType,
			Action:     list[j].Action,
			Permission: list[j].Permission,
			ParentID:   list[j].ParentID,
			NoCache:    list[j].NoCache,
			Breadcrumb: list[j].Breadcrumb,
			Component:  list[j].Component,
			Sort:       list[j].Sort,
			Visible:    list[j].Visible,
			CreatedAt:  list[j].CreatedAt,
			SysAPI:     list[j].SysAPI,
			Children:   []models.SysMenu{}}

		if mi.MenuType != cModels.Button {
			ms := menuCall(menuList, mi)
			children = append(children, ms)
		} else {
			children = append(children, mi)
		}
	}
	menu.Children = children
	return menu
}

func menuDistinct(menuList []models.SysMenu) (result []models.SysMenu) {
	distinctMap := make(map[int]struct{}, len(menuList))
	for _, menu := range menuList {
		if _, ok := distinctMap[menu.MenuID]; !ok {
			distinctMap[menu.MenuID] = struct{}{}
			result = append(result, menu)
		}
	}
	return result
}

func recursiveSetMenu(orm *gorm.DB, mIDs []int, menus *[]models.SysMenu) error {
	if len(mIDs) == 0 || menus == nil {
		return nil
	}
	var subMenus []models.SysMenu
	err := orm.Where(fmt.Sprintf(" menu_type in ('%s', '%s', '%s') and menu_id in ?",
		cModels.Directory, cModels.Menu, cModels.Button), mIDs).Order("sort").Find(&subMenus).Error
	if err != nil {
		return err
	}

	subIDs := make([]int, 0)
	for _, menu := range subMenus {
		if menu.ParentID != 0 {
			subIDs = append(subIDs, menu.ParentID)
		}
		if menu.MenuType != cModels.Button {
			*menus = append(*menus, menu)
		}
	}
	return recursiveSetMenu(orm, subIDs, menus)
}

// SetMenuRole 获取左侧菜单树使用
func (e *SysMenu) SetMenuRole(roleName string) (m []models.SysMenu, err error) {
	menus, err := e.getByRoleName(roleName)
	m = make([]models.SysMenu, 0)
	for i := range menus {
		if menus[i].ParentID != 0 {
			continue
		}
		menusInfo := menuCall(&menus, menus[i])
		m = append(m, menusInfo)
	}
	return
}

// getPage 菜单分页列表
func (e *SysMenu) getPage(c *dto.SysMenuGetPageReq, list *[]models.SysMenu) error {
	var err error
	var data models.SysMenu

	err = e.DB.Model(&data).
		Scopes(
			cDto.OrderDest("sort", false),
			cDto.MakeCondition(c.GetNeedSearch()),
		).Preload("SysAPI").
		Find(list).Error
	if err != nil {
		e.Logger.Errorf("getSysMenuPage error:%s", err)
		return err
	}
	return nil
}

func (e *SysMenu) initPaths(tx *gorm.DB, menu *models.SysMenu) error {
	var err error
	var data models.SysMenu
	parentMenu := new(models.SysMenu)
	if menu.ParentID != 0 {
		if menu.ParentID == menu.MenuID {
			return errors.New("菜单不能设置自身为父级")
		}
		err = tx.Model(&data).First(parentMenu, menu.ParentID).Error
		if err != nil {
			return err
		}
		if parentMenu.Paths == "" {
			err = errors.New("父级paths异常，请尝试对当前节点父级菜单进行更新操作！")
			return err
		}
		if strings.Contains(parentMenu.Paths+"/", "/"+pkg.IntToString(menu.MenuID)+"/") {
			return errors.New("菜单不能移动到自己的子菜单")
		}
		menu.Paths = parentMenu.Paths + "/" + pkg.IntToString(menu.MenuID)
	} else {
		menu.Paths = "/0/" + pkg.IntToString(menu.MenuID)
	}
	err = tx.Model(&data).Where("menu_id = ?", menu.MenuID).Update("paths", menu.Paths).Error
	return err
}

func (e *SysMenu) getByRoleName(roleName string) ([]models.SysMenu, error) {
	var role models.SysRole
	var err error
	data := make([]models.SysMenu, 0)

	if roleName == "admin" {
		// The soft-delete condition is GORM's to add: it appends one for the
		// model's DeletedAt field on every query. Writing it by hand duplicates
		// that and hard-codes what "deleted" looks like — a column that stops
		// being nullable turns this clause into one that matches nothing.
		err = e.DB.Where("menu_type in ('M','C')").
			Order("sort").
			Find(&data).
			Error
	} else {
		role.RoleKey = roleName
		err = e.DB.Model(&role).Where("role_key = ? ", roleName).Preload("SysMenu").First(&role).Error

		if role.SysMenu != nil {
			mIDs := make([]int, 0)
			for _, menu := range *role.SysMenu {
				mIDs = append(mIDs, menu.MenuID)
			}
			if err := recursiveSetMenu(e.DB, mIDs, &data); err != nil {
				return nil, err
			}

			data = menuDistinct(data)
		}
	}

	sort.Sort(models.SysMenuSlice(data))
	return data, err
}
