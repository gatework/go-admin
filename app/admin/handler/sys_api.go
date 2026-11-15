package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger annotations require the response schema. // Registers the documentation or route assets used by this package.
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor/common/request"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/admin/service/dto"
	"github.com/gatework/motor/common/actions"
)

// Route handles HTTP requests for its administration resource.
type Route struct {
	api.API
}

// List 获取接口管理列表
// @Summary 获取接口管理列表
// @Description 获取接口管理列表
// @Tags 接口管理
// @Param name query string false "名称"
// @Param title query string false "标题"
// @Param path query string false "地址"
// @Param action query string false "类型"
// @Param pageSize query int false "页条数"
// @Param pageIndex query int false "页码"
// @Success 200 {object} response.Response{data=response.Page{list=[]models.SysAPI}} "{"code": 200, "data": [...]}"
// @Router /api/v1/sys-api [get]
// @Security Bearer
func (e Route) List(c *gin.Context) {
	s := service.Route{}
	req := dto.SysAPIGetPageReq{}
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
	//数据权限检查
	p := actions.GetPermissionFromContext(c)
	list := make([]models.SysAPI, 0)
	var count int64
	err = s.List(&req, p, &list, &count)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}
	e.PageOK(list, int(count), req.GetPageIndex(), req.GetPageSize(), "查询成功")
}

// Get 获取接口管理
// @Summary 获取接口管理
// @Description 获取接口管理
// @Tags 接口管理
// @Param id path string false "id"
// @Success 200 {object} response.Response{data=models.SysAPI} "{"code": 200, "data": [...]}"
// @Router /api/v1/sys-api/{id} [get]
// @Security Bearer
func (e Route) Get(c *gin.Context) {
	req := dto.SysAPIGetReq{}
	s := service.Route{}
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
	p := actions.GetPermissionFromContext(c)
	var object models.SysAPI
	err = s.Get(&req, p, &object)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}
	e.OK(object, "查询成功")
}

// Update 修改接口管理
// @Summary 修改接口管理
// @Description 修改接口管理
// @Tags 接口管理
// @Accept application/json
// @Product application/json
// @Param data body dto.SysAPIUpdateReq true "body"
// @Success 200 {object} response.Response	"{"code": 200, "message": "修改成功"}"
// @Router /api/v1/sys-api/{id} [put]
// @Security Bearer
func (e Route) Update(c *gin.Context) {
	req := dto.SysAPIUpdateReq{}
	s := service.Route{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		return
	}
	req.SetUpdateBy(user.ID(c))
	p := actions.GetPermissionFromContext(c)
	err = s.Update(&req, p)
	if err != nil {
		e.Error(500, err, "更新失败")
		return
	}
	e.OK(req.GetID(), "更新成功")
}
