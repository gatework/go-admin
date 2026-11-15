package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/common/request"

	"github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/admin/service/dto"
)

// Menu handles HTTP requests for its administration resource.
type Menu struct {
	api.API
}

// List Menu列表数据
// @Summary Menu列表数据
// @Description 获取JSON
// @Tags 菜单
// @Param menuName query string false "menuName"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/menu [get]
// @Security Bearer
func (e Menu) List(c *gin.Context) {
	s := service.SysMenu{}
	req := dto.SysMenuGetPageReq{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}
	var list = make([]models.SysMenu, 0)
	err = s.GetPage(&req, &list)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}
	e.OK(list, "查询成功")
}

// Get 获取菜单详情
// @Summary Menu详情数据
// @Description 获取JSON
// @Tags 菜单
// @Param id path string false "id"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/menu/{id} [get]
// @Security Bearer
func (e Menu) Get(c *gin.Context) {
	req := dto.SysMenuGetReq{}
	s := new(service.SysMenu)
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}
	var object = models.SysMenu{}

	err = s.Get(&req, &object)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}
	e.OK(object, "查询成功")
}

// Create 创建菜单
// @Summary 创建菜单
// @Description 获取JSON
// @Tags 菜单
// @Accept  application/json
// @Product application/json
// @Param data body dto.SysMenuInsertReq true "data"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/menu [post]
// @Security Bearer
func (e Menu) Create(c *gin.Context) {
	req := dto.SysMenuInsertReq{}
	s := new(service.SysMenu)
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}
	// 设置创建人
	req.SetCreateBy(user.ID(c))
	err = s.Insert(&req)
	if err != nil {
		e.Error(500, err, "创建失败")
		return
	}
	e.OK(req.GetID(), "创建成功")
}

// Update 修改菜单
// @Summary 修改菜单
// @Description 获取JSON
// @Tags 菜单
// @Accept  application/json
// @Product application/json
// @Param id path int true "id"
// @Param data body dto.SysMenuUpdateReq true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/menu/{id} [put]
// @Security Bearer
func (e Menu) Update(c *gin.Context) {
	req := dto.SysMenuUpdateReq{}
	s := new(service.SysMenu)
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	req.SetUpdateBy(user.ID(c))
	err = s.Update(&req)
	if err != nil {
		e.Error(500, err, "更新失败")
		return
	}
	e.OK(req.GetID(), "更新成功")
}

// Delete 删除菜单
// @Summary 删除菜单
// @Description 删除数据
// @Tags 菜单
// @Param data body dto.SysMenuDeleteReq true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/menu [delete]
// @Security Bearer
func (e Menu) Delete(c *gin.Context) {
	control := new(dto.SysMenuDeleteReq)
	s := new(service.SysMenu)
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(control)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}
	err = s.Remove(control)
	if err != nil {
		e.Logger.Errorf("RemoveSysMenu error, %s", err)
		e.Error(500, err, "删除失败")
		return
	}
	e.OK(control.GetID(), "删除成功")
}

// RoleTree 根据登录角色名称获取菜单列表数据（左菜单使用）
// @Summary 根据登录角色名称获取菜单列表数据（左菜单使用）
// @Description 获取JSON
// @Tags 菜单
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/menurole [get]
// @Security Bearer
func (e Menu) RoleTree(c *gin.Context) {
	s := new(service.SysMenu)
	var err error
	e.API, err = request.New(c)
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	result, err := s.SetMenuRole(user.RoleName(c))

	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}

	e.OK(result, "")
}

// Tree 根据角色ID查询菜单下拉树结构
// @Summary 角色修改使用的菜单列表
// @Description 获取JSON
// @Tags 菜单
// @Accept  application/json
// @Product application/json
// @Param roleId path int true "roleId"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/menuTreeselect/{roleId} [get]
// @Security Bearer
func (e Menu) Tree(c *gin.Context) {
	m := service.SysMenu{}
	r := service.SysRole{}
	req := dto.SelectRole{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	m.Service = e.Service()
	r.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	result, err := m.SetLabel()
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}

	menuIDs := make([]int, 0)
	if req.RoleID != 0 {
		menuIDs, err = r.GetRoleMenuID(req.RoleID)
		if err != nil {
			e.Error(500, err, "")
			return
		}
	}
	e.OK(gin.H{
		"menus":       result,
		"checkedKeys": menuIDs,
	}, "获取成功")
}
