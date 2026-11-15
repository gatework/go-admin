package dto

import (
	"github.com/gatework/motor/app/admin/models"
	common "github.com/gatework/motor/common/models"

	"github.com/gatework/motor/common/dto"
)

// SysMenuGetPageReq 列表或者搜索使用结构体
type SysMenuGetPageReq struct {
	dto.Pagination `search:"-"`
	Title          string `form:"title" search:"type:contains;column:title;table:sys_menu" comment:"菜单名称"`  // 菜单名称
	Visible        int    `form:"visible" search:"type:exact;column:visible;table:sys_menu" comment:"显示状态"` // 显示状态
}

// GetNeedSearch returns the tagged query parameters.
func (m *SysMenuGetPageReq) GetNeedSearch() any {
	return *m
}

// SysMenuInsertReq carries validated administration request parameters.
type SysMenuInsertReq struct {
	common.ControlBy
	MenuID     int             `uri:"id" comment:"编码"`            // 编码
	MenuName   string          `form:"menuName" comment:"菜单name"` //菜单name
	Title      string          `form:"title" comment:"显示名称"`      //显示名称
	Icon       string          `form:"icon" comment:"图标"`         //图标
	Path       string          `form:"path" comment:"路径"`         //路径
	Paths      string          `form:"paths" comment:"id路径"`      //id路径
	MenuType   string          `form:"menuType" comment:"菜单类型"`   //菜单类型
	SysAPI     []models.SysAPI `form:"sysApi"`
	APIs       []int           `form:"apis"`
	Action     string          `form:"action" comment:"请求方式"`      //请求方式
	Permission string          `form:"permission" comment:"权限编码"`  //权限编码
	ParentID   int             `form:"parentId" comment:"上级菜单"`    //上级菜单
	NoCache    bool            `form:"noCache" comment:"是否缓存"`     //是否缓存
	Breadcrumb string          `form:"breadcrumb" comment:"是否面包屑"` //是否面包屑
	Component  string          `form:"component" comment:"组件"`     //组件
	Sort       int             `form:"sort" comment:"排序"`          //排序
	Visible    string          `form:"visible" comment:"是否显示"`     //是否显示
	IsFrame    string          `form:"isFrame" comment:"是否frame"`  //是否frame
}

// Generate copies request fields into a new request or supplied model.
func (s *SysMenuInsertReq) Generate(model *models.SysMenu) {
	if s.MenuID != 0 {
		model.MenuID = s.MenuID
	}
	model.MenuName = s.MenuName
	model.Title = s.Title
	model.Icon = s.Icon
	model.Path = s.Path
	model.Paths = s.Paths
	model.MenuType = s.MenuType
	model.Action = s.Action
	model.SysAPI = s.SysAPI
	model.Permission = s.Permission
	model.ParentID = s.ParentID
	model.NoCache = s.NoCache
	model.Breadcrumb = s.Breadcrumb
	model.Component = s.Component
	model.Sort = s.Sort
	model.Visible = s.Visible
	model.IsFrame = s.IsFrame
	if s.CreateBy != 0 {
		model.CreateBy = s.CreateBy
	}
	if s.UpdateBy != 0 {
		model.UpdateBy = s.UpdateBy
	}
}

// GetID returns the selected resource identity.
func (s *SysMenuInsertReq) GetID() any {
	return s.MenuID
}

// SysMenuUpdateReq carries validated administration request parameters.
type SysMenuUpdateReq struct {
	common.ControlBy
	MenuID     int             `uri:"id" comment:"编码"`            // 编码
	MenuName   string          `form:"menuName" comment:"菜单name"` //菜单name
	Title      string          `form:"title" comment:"显示名称"`      //显示名称
	Icon       string          `form:"icon" comment:"图标"`         //图标
	Path       string          `form:"path" comment:"路径"`         //路径
	Paths      string          `form:"paths" comment:"id路径"`      //id路径
	MenuType   string          `form:"menuType" comment:"菜单类型"`   //菜单类型
	SysAPI     []models.SysAPI `form:"sysApi"`
	APIs       []int           `form:"apis"`
	Action     string          `form:"action" comment:"请求方式"`      //请求方式
	Permission string          `form:"permission" comment:"权限编码"`  //权限编码
	ParentID   int             `form:"parentId" comment:"上级菜单"`    //上级菜单
	NoCache    bool            `form:"noCache" comment:"是否缓存"`     //是否缓存
	Breadcrumb string          `form:"breadcrumb" comment:"是否面包屑"` //是否面包屑
	Component  string          `form:"component" comment:"组件"`     //组件
	Sort       int             `form:"sort" comment:"排序"`          //排序
	Visible    string          `form:"visible" comment:"是否显示"`     //是否显示
	IsFrame    string          `form:"isFrame" comment:"是否frame"`  //是否frame
}

// Generate copies request fields into a new request or supplied model.
func (s *SysMenuUpdateReq) Generate(model *models.SysMenu) {
	if s.MenuID != 0 {
		model.MenuID = s.MenuID
	}
	model.MenuName = s.MenuName
	model.Title = s.Title
	model.Icon = s.Icon
	model.Path = s.Path
	model.Paths = s.Paths
	model.MenuType = s.MenuType
	model.Action = s.Action
	model.SysAPI = s.SysAPI
	model.Permission = s.Permission
	model.ParentID = s.ParentID
	model.NoCache = s.NoCache
	model.Breadcrumb = s.Breadcrumb
	model.Component = s.Component
	model.Sort = s.Sort
	model.Visible = s.Visible
	model.IsFrame = s.IsFrame
	if s.CreateBy != 0 {
		model.CreateBy = s.CreateBy
	}
	if s.UpdateBy != 0 {
		model.UpdateBy = s.UpdateBy
	}
}

// GetID returns the selected resource identity.
func (s *SysMenuUpdateReq) GetID() any {
	return s.MenuID
}

// SysMenuGetReq carries validated administration request parameters.
type SysMenuGetReq struct {
	ID int `uri:"id"`
}

// GetID returns the selected resource identity.
func (s *SysMenuGetReq) GetID() any {
	return s.ID
}

// SysMenuDeleteReq carries validated administration request parameters.
type SysMenuDeleteReq struct {
	common.ControlBy
	IDs []int `json:"ids"`
}

// GetID returns the selected resource identity.
func (s *SysMenuDeleteReq) GetID() any {
	return s.IDs
}

// MenuLabel carries validated administration request parameters.
type MenuLabel struct {
	ID       int         `json:"id,omitempty" gorm:"-"`
	Label    string      `json:"label,omitempty" gorm:"-"`
	Children []MenuLabel `json:"children,omitempty" gorm:"-"`
}

// MenuRole carries validated administration request parameters.
type MenuRole struct {
	models.SysMenu
	IsSelect bool `json:"is_select" gorm:"-"`
}

// SelectRole carries validated administration request parameters.
type SelectRole struct {
	RoleID int `uri:"roleId"`
}
