package handler

import (
	"fmt"

	"github.com/gatework/motor/common/request"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger annotations require the response schema. // Registers the documentation or route assets used by this package.
	"github.com/gatework/motor-core/sdk/api"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/admin/service/dto"
)

// Post handles HTTP requests for its administration resource.
type Post struct {
	api.API
}

// List lists records matching the request and data permissions.
// @Summary 岗位列表数据
// @Description 获取JSON
// @Tags 岗位
// @Param postName query string false "postName"
// @Param postCode query string false "postCode"
// @Param postId query string false "postId"
// @Param status query string false "status"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/post [get]
// @Security Bearer
func (e Post) List(c *gin.Context) {
	s := service.SysPost{}
	req := dto.SysPostPageReq{}
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

	list := make([]models.SysPost, 0)
	var count int64

	err = s.GetPage(&req, &list, &count)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}

	e.PageOK(list, int(count), req.GetPageIndex(), req.GetPageSize(), "查询成功")
}

// Get loads the requested record within its data permissions.
// @Summary 获取岗位信息
// @Description 获取JSON
// @Tags 岗位
// @Param id path int true "编码"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/post/{postId} [get]
// @Security Bearer
func (e Post) Get(c *gin.Context) {
	s := service.SysPost{}
	req := dto.SysPostGetReq{}
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
	var object models.SysPost

	err = s.Get(&req, &object)
	if err != nil {
		e.Error(500, err, fmt.Sprintf("岗位信息获取失败！错误详情：%s", err.Error()))
		return
	}

	e.OK(object, "查询成功")
}

// Create creates a record from the validated request.
// @Summary 添加岗位
// @Description 获取JSON
// @Tags 岗位
// @Accept  application/json
// @Product application/json
// @Param data body dto.SysPostInsertReq true "data"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/post [post]
// @Security Bearer
func (e Post) Create(c *gin.Context) {
	s := service.SysPost{}
	req := dto.SysPostInsertReq{}
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
	req.SetCreateBy(user.ID(c))
	err = s.Insert(&req)
	if err != nil {
		e.Error(500, err, fmt.Sprintf("新建岗位失败！错误详情：%s", err.Error()))
		return
	}
	e.OK(req.GetID(), "创建成功")
}

// Update updates an existing record within its data permissions.
// @Summary 修改岗位
// @Description 获取JSON
// @Tags 岗位
// @Accept  application/json
// @Product application/json
// @Param data body dto.SysPostUpdateReq true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/post/{id} [put]
// @Security Bearer
func (e Post) Update(c *gin.Context) {
	s := service.SysPost{}
	req := dto.SysPostUpdateReq{}
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
		e.Error(500, err, fmt.Sprintf("岗位更新失败！错误详情：%s", err.Error()))
		return
	}
	e.OK(req.GetID(), "更新成功")
}

// Delete removes selected records within their data permissions.
// @Summary 删除岗位
// @Description 删除数据
// @Tags 岗位
// @Param id body dto.SysPostDeleteReq true "请求参数"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/post [delete]
// @Security Bearer
func (e Post) Delete(c *gin.Context) {
	s := service.SysPost{}
	req := dto.SysPostDeleteReq{}
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
	err = s.Remove(&req)
	if err != nil {
		e.Error(500, err, fmt.Sprintf("岗位删除失败！错误详情：%s", err.Error()))
		return
	}
	e.OK(req.GetID(), "删除成功")
}
