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
	s := service.Role{}
	req := dto.RoleSearch{}
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

	err = s.List(c.Request.Context(), &req, &list, &count)
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
	s := service.Role{}
	req := dto.RoleByID{}
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

	err = s.Get(c.Request.Context(), &req, &object)
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
// @Param data body dto.RoleCreate true "data"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/role [post]
// @Security Bearer
func (e Role) Create(c *gin.Context) {
	s := service.Role{}
	req := dto.RoleCreate{}
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
	cb := sdk.Runtime.Casbin(c.Request.Host)
	err = s.Create(c.Request.Context(), &req, cb)
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
// @Param data body dto.RoleUpdate true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/role/{id} [put]
// @Security Bearer
func (e Role) Update(c *gin.Context) {
	s := service.Role{}
	req := dto.RoleUpdate{}
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

	err = s.Update(c.Request.Context(), &req, cb)
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
// @Param data body dto.RoleDelete true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/role [delete]
// @Security Bearer
func (e Role) Delete(c *gin.Context) {
	s := new(service.Role)
	req := dto.RoleDelete{}
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
	err = s.Delete(c.Request.Context(), &req, cb)
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
// @Param data body dto.RoleStatusUpdate true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/role-status/{id} [put]
// @Security Bearer
func (e Role) UpdateStatus(c *gin.Context) {
	s := service.Role{}
	req := dto.RoleStatusUpdate{}
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
	err = s.UpdateStatus(c.Request.Context(), &req)
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
// @Param data body dto.RoleDataScopeUpdate true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/role-status/{id} [put]
// @Security Bearer
func (e Role) UpdateDataScope(c *gin.Context) {
	s := service.Role{}
	req := dto.RoleDataScopeUpdate{}
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
	err = s.UpdateDataScope(c.Request.Context(), &req)
	if err != nil {
		e.Error(500, err, fmt.Sprintf("更新角色数据权限失败！错误详情：%s", err.Error()))
		return
	}
	e.OK(nil, "操作成功")
}
