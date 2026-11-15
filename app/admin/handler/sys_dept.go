package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger annotations require the response schema. // Registers the documentation or route assets used by this package.
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/pkg"
	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/common/request"

	"github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/admin/service/dto"
)

// Department handles HTTP requests for its administration resource.
type Department struct {
	api.API
}

// List lists records matching the request and data permissions.
// @Summary 分页部门列表数据
// @Description 分页列表
// @Tags 部门
// @Param deptName query string false "deptName"
// @Param deptId query string false "deptId"
// @Param position query string false "position"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/dept [get]
// @Security Bearer
func (e Department) List(c *gin.Context) {
	s := service.Department{}
	req := dto.DepartmentSearch{}
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
	list, err := s.List(c.Request.Context(), &req)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}
	e.OK(list, "查询成功")
}

// Get loads the requested record within its data permissions.
// @Summary 获取部门数据
// @Description 获取JSON
// @Tags 部门
// @Param deptId path string false "deptId"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/dept/{deptId} [get]
// @Security Bearer
func (e Department) Get(c *gin.Context) {
	s := service.Department{}
	req := dto.DepartmentByID{}
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
	var object models.SysDept

	err = s.Get(c.Request.Context(), &req, &object)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}

	e.OK(object, "查询成功")
}

// Create 添加部门
// @Summary 添加部门
// @Description 获取JSON
// @Tags 部门
// @Accept  application/json
// @Product application/json
// @Param data body dto.DepartmentCreate true "data"
// @Success 200 {string} string	"{"code": 200, "message": "添加成功"}"
// @Success 200 {string} string	"{"code": -1, "message": "添加失败"}"
// @Router /api/v1/dept [post]
// @Security Bearer
func (e Department) Create(c *gin.Context) {
	s := service.Department{}
	req := dto.DepartmentCreate{}
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
	err = s.Create(c.Request.Context(), &req)
	if err != nil {
		e.Error(500, err, "创建失败")
		return
	}
	e.OK(req.GetID(), "创建成功")
}

// Update updates an existing record within its data permissions.
// @Summary 修改部门
// @Description 获取JSON
// @Tags 部门
// @Accept  application/json
// @Product application/json
// @Param id path int true "id"
// @Param data body dto.DepartmentUpdate true "body"
// @Success 200 {string} string	"{"code": 200, "message": "添加成功"}"
// @Success 200 {string} string	"{"code": -1, "message": "添加失败"}"
// @Router /api/v1/dept/{deptId} [put]
// @Security Bearer
func (e Department) Update(c *gin.Context) {
	s := service.Department{}
	req := dto.DepartmentUpdate{}
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
	err = s.Update(c.Request.Context(), &req)
	if err != nil {
		e.Error(500, err, err.Error())
		return
	}
	e.OK(req.GetID(), "更新成功")
}

// Delete removes selected records within their data permissions.
// @Summary 删除部门
// @Description 删除数据
// @Tags 部门
// @Param data body dto.DepartmentDelete true "body"
// @Success 200 {string} string	"{"code": 200, "message": "删除成功"}"
// @Success 200 {string} string	"{"code": -1, "message": "删除失败"}"
// @Router /api/v1/dept [delete]
// @Security Bearer
func (e Department) Delete(c *gin.Context) {
	s := service.Department{}
	req := dto.DepartmentDelete{}
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

	err = s.Delete(c.Request.Context(), &req)
	if err != nil {
		e.Error(500, err, "删除失败")
		return
	}
	e.OK(req.GetID(), "删除成功")
}

// Tree 用户管理 左侧部门树
func (e Department) Tree(c *gin.Context) {
	s := service.Department{}
	req := dto.DepartmentSearch{}
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
	list, err := s.Tree(c.Request.Context(), &req)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}
	e.OK(list, "")
}

// RoleTree TODO: 此接口需要调整不应该将list和选中放在一起
func (e Department) RoleTree(c *gin.Context) {
	s := service.Department{}
	var err error
	e.API, err = request.New(c)
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	id, err := pkg.StringToInt(c.Param("roleId"))
	if err != nil {
		e.Error(400, err, "invalid role id")
		return
	}
	result, err := s.Labels(c.Request.Context())
	if err != nil {
		e.Error(500, err, err.Error())
		return
	}
	menuIDs := make([]int, 0)
	if id != 0 {
		menuIDs, err = s.IDsForRole(c.Request.Context(), id)
		if err != nil {
			e.Error(500, err, err.Error())
			return
		}
	}
	e.OK(gin.H{
		"depts":       result,
		"checkedKeys": menuIDs,
	}, "")
}
