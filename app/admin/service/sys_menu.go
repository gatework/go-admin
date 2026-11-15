package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gatework/motor/common/account"

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

// Menu coordinates administration queries and mutations.
type Menu struct {
	service.Service
}

// List 获取SysMenu列表
func (e *Menu) List(ctx context.Context, c *dto.MenuSearch, menus *[]models.SysMenu) error {
	var rows []models.SysMenu
	if err := e.getPage(ctx, c, &rows); err != nil {
		return err
	}
	*menus = append(*menus, forest(rows, menuID, menuParent, isNotButton, withMenuChildren)...)
	return nil
}

// Get 获取SysMenu对象
func (e *Menu) Get(ctx context.Context, d *dto.MenuByID, model *models.SysMenu) error {
	var err error
	var data models.SysMenu

	db := e.DB.WithContext(ctx).Model(&data).Preload("SysAPI").
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

// Create 创建SysMenu对象
func (e *Menu) Create(ctx context.Context, c *dto.MenuCreate) error {
	var data models.SysMenu
	err := e.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
func (e *Menu) Update(ctx context.Context, c *dto.MenuUpdate) error {
	err := e.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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

// Delete 删除SysMenu
func (e *Menu) Delete(ctx context.Context, d *dto.MenuDelete) error {
	var err error
	var data models.SysMenu

	db := e.DB.WithContext(ctx).Model(&data).Delete(&data, d.IDs)
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

// ListFlat 获取菜单数据
func (e *Menu) ListFlat(ctx context.Context, c *dto.MenuSearch, list *[]models.SysMenu) error {
	var err error
	var data models.SysMenu

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

// Labels 修改角色中 设置菜单基础数据
func (e *Menu) Labels(ctx context.Context) (m []dto.MenuLabel, err error) {
	var list []models.SysMenu
	if err = e.ListFlat(ctx, &dto.MenuSearch{}, &list); err != nil {
		return nil, err
	}
	return forest(list, menuID, menuParent, isNotButton, func(menu models.SysMenu, children []dto.MenuLabel) dto.MenuLabel {
		return dto.MenuLabel{ID: menu.MenuID, Label: menu.Title, Children: children}
	}), nil
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

// ForRole 获取左侧菜单树使用
func (e *Menu) ForRole(ctx context.Context, roleName string) (m []models.SysMenu, err error) {
	menus, err := e.getByRoleName(ctx, roleName)
	if err != nil {
		return nil, err
	}
	return forest(menus, menuID, menuParent, isNotButton, withMenuChildren), nil
}

// getPage 菜单分页列表
func (e *Menu) getPage(ctx context.Context, c *dto.MenuSearch, list *[]models.SysMenu) error {
	var err error
	var data models.SysMenu

	err = e.DB.WithContext(ctx).Model(&data).
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

func (e *Menu) initPaths(tx *gorm.DB, menu *models.SysMenu) error {
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

func (e *Menu) getByRoleName(ctx context.Context, roleName string) ([]models.SysMenu, error) {
	var role models.SysRole
	var err error
	data := make([]models.SysMenu, 0)

	if account.IsAdmin(roleName) {
		// The soft-delete condition is GORM's to add: it appends one for the
		// model's DeletedAt field on every query. Writing it by hand duplicates
		// that and hard-codes what "deleted" looks like — a column that stops
		// being nullable turns this clause into one that matches nothing.
		err = e.DB.WithContext(ctx).Where("menu_type in ('M','C')").
			Order("sort").
			Find(&data).
			Error
	} else {
		role.RoleKey = roleName
		err = e.DB.WithContext(ctx).Model(&role).Where("role_key = ? ", roleName).Preload("SysMenu").First(&role).Error

		if role.SysMenu != nil {
			mIDs := make([]int, 0)
			for _, menu := range *role.SysMenu {
				mIDs = append(mIDs, menu.MenuID)
			}
			if err := recursiveSetMenu(e.DB.WithContext(ctx), mIDs, &data); err != nil {
				return nil, err
			}

			data = menuDistinct(data)
		}
	}

	sort.Sort(models.SysMenuSlice(data))
	return data, err
}

func menuID(m models.SysMenu) int     { return m.MenuID }
func menuParent(m models.SysMenu) int { return m.ParentID }

// isNotButton keeps buttons as leaves: they carry permissions, not children.
func isNotButton(m models.SysMenu) bool { return m.MenuType != cModels.Button }

func withMenuChildren(m models.SysMenu, children []models.SysMenu) models.SysMenu {
	m.Children = children
	return m
}
