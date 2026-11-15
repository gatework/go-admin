package handler

import (
	"fmt"
	"net/http"

	"github.com/gatework/motor/common/request"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor/app/admin/models"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger annotations require the response schema. // Registers the documentation or route assets used by this package.
	"github.com/gatework/motor-core/sdk/api"

	"github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/admin/service/dto"
)

// Role handles HTTP requests for its administration resource.
type Role struct {
	api.API
}

// List lists records matching the request and data permissions.
// @Summary 角色列表数据
// @Description Get JSON
// @Tags 角色/Role
// @Param roleName query string false "roleName"
// @Param status query string false "status"
// @Param roleKey query string false "roleKey"
// @Param pageSize query int false "页条数"
// @Param pageIndex query int false "页码"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/role [get]
// @Security Bearer
func (e Role) List(c *gin.Context) {
	s := service.SysRole{}
	req := dto.SysRoleGetPageReq{}
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

	list := make([]models.SysRole, 0)
	var count int64

	err = s.GetPage(&req, &list, &count)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}

	e.PageOK(list, int(count), req.GetPageIndex(), req.GetPageSize(), "查询成功")
}

// Get loads the requested record within its data permissions.
// @Summary 获取Role数据
// @Description 获取JSON
// @Tags 角色/Role
// @Param roleId path string false "roleId"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/role/{id} [get]
// @Security Bearer
func (e Role) Get(c *gin.Context) {
	s := service.SysRole{}
	req := dto.SysRoleGetReq{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, fmt.Sprintf(" %s ", err.Error()))
		return
	}

	var object models.SysRole

	err = s.Get(&req, &object)
	if err != nil {
		e.Error(http.StatusUnprocessableEntity, err, "查询失败")
		return
	}

	e.OK(object, "查询成功")
}

// Create creates a record from the validated request.
// @Summary 创建角色
// @Description 获取JSON
// @Tags 角色/Role
// @Accept  application/json
// @Product application/json
// @Param data body dto.SysRoleInsertReq true "data"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/role [post]
// @Security Bearer
func (e Role) Create(c *gin.Context) {
	s := service.SysRole{}
	req := dto.SysRoleInsertReq{}
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
	req.CreateBy = user.ID(c)
	if req.Status == "" {
		req.Status = "2"
	}
	cb := sdk.Runtime.Casbin(c.Request.Host)
	err = s.Insert(&req, cb)
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, "创建失败,"+err.Error())
		return
	}

	e.OK(req.GetID(), "创建成功")
}

// Update 修改用户角色
// @Summary 修改用户角色
// @Description 获取JSON
// @Tags 角色/Role
// @Accept  application/json
// @Product application/json
// @Param data body dto.SysRoleUpdateReq true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/role/{id} [put]
// @Security Bearer
func (e Role) Update(c *gin.Context) {
	s := service.SysRole{}
	req := dto.SysRoleUpdateReq{}
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
	cb := sdk.Runtime.Casbin(c.Request.Host)

	req.SetUpdateBy(user.ID(c))

	err = s.Update(&req, cb)
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, "更新失败,"+err.Error())
		return
	}

	e.OK(req.GetID(), "更新成功")
}

// Delete removes selected records within their data permissions.
// @Summary 删除用户角色
// @Description 删除数据
// @Tags 角色/Role
// @Param data body dto.SysRoleDeleteReq true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/role [delete]
// @Security Bearer
func (e Role) Delete(c *gin.Context) {
	s := new(service.SysRole)
	req := dto.SysRoleDeleteReq{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, fmt.Sprintf("删除角色 %v 失败，\r\n失败信息 %s", req.IDs, err.Error()))
		return
	}

	cb := sdk.Runtime.Casbin(c.Request.Host)
	err = s.Remove(&req, cb)
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, "")
		return
	}

	e.OK(req.GetID(), fmt.Sprintf("删除角色角色 %v 状态成功！", req.GetID()))
}

// UpdateStatus 修改用户角色状态
// @Summary 修改用户角色
// @Description 获取JSON
// @Tags 角色/Role
// @Accept  application/json
// @Product application/json
// @Param data body dto.UpdateStatusReq true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/role-status/{id} [put]
// @Security Bearer
func (e Role) UpdateStatus(c *gin.Context) {
	s := service.SysRole{}
	req := dto.UpdateStatusReq{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, fmt.Sprintf("更新角色状态失败，失败原因：%s ", err.Error()))
		return
	}
	req.SetUpdateBy(user.ID(c))
	err = s.UpdateStatus(&req)
	if err != nil {
		e.Error(500, err, fmt.Sprintf("更新角色状态失败，失败原因：%s ", err.Error()))
		return
	}
	e.OK(req.GetID(), fmt.Sprintf("更新角色 %v 状态成功！", req.GetID()))
}

// UpdateDataScope 更新角色数据权限
// @Summary 更新角色数据权限
// @Description 获取JSON
// @Tags 角色/Role
// @Accept  application/json
// @Product application/json
// @Param data body dto.RoleDataScopeReq true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/role-status/{id} [put]
// @Security Bearer
func (e Role) UpdateDataScope(c *gin.Context) {
	s := service.SysRole{}
	req := dto.RoleDataScopeReq{}
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
	err = s.UpdateDataScope(&req)
	if err != nil {
		e.Error(500, err, fmt.Sprintf("更新角色数据权限失败！错误详情：%s", err.Error()))
		return
	}
	e.OK(nil, "操作成功")
}
