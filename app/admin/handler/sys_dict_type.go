package handler

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger annotations require the response schema. // Registers the documentation or route assets used by this package.
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/common/request"

	"github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/admin/service/dto"
)

// Dictionary handles HTTP requests for its administration resource.
type Dictionary struct {
	api.API
}

// List 字典类型列表数据
// @Summary 字典类型列表数据
// @Description 获取JSON
// @Tags 字典类型
// @Param dictName query string false "dictName"
// @Param dictId query string false "dictId"
// @Param dictType query string false "dictType"
// @Param pageSize query int false "页条数"
// @Param pageIndex query int false "页码"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/dict/type [get]
// @Security Bearer
func (e Dictionary) List(c *gin.Context) {
	s := service.Dictionary{}
	req := dto.SysDictTypeGetPageReq{}
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
	list := make([]models.SysDictType, 0)
	var count int64
	err = s.List(&req, &list, &count)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}
	e.PageOK(list, int(count), req.GetPageIndex(), req.GetPageSize(), "查询成功")
}

// Get 字典类型通过字典id获取
// @Summary 字典类型通过字典id获取
// @Description 获取JSON
// @Tags 字典类型
// @Param dictId path int true "字典类型编码"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/dict/type/{dictId} [get]
// @Security Bearer
func (e Dictionary) Get(c *gin.Context) {
	s := service.Dictionary{}
	req := dto.SysDictTypeGetReq{}
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
	var object models.SysDictType
	err = s.Get(&req, &object)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}
	e.OK(object, "查询成功")
}

// Create 字典类型创建
// @Summary 添加字典类型
// @Description 获取JSON
// @Tags 字典类型
// @Accept  application/json
// @Product application/json
// @Param data body dto.SysDictTypeInsertReq true "data"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/dict/type [post]
// @Security Bearer
func (e Dictionary) Create(c *gin.Context) {
	s := service.Dictionary{}
	req := dto.SysDictTypeInsertReq{}
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
	err = s.Create(&req)
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, fmt.Sprintf(" 创建字典类型失败，详情：%s", err.Error()))
		return
	}
	e.OK(req.GetID(), "创建成功")
}

// Update updates an existing record within its data permissions.
// @Summary 修改字典类型
// @Description 获取JSON
// @Tags 字典类型
// @Accept  application/json
// @Product application/json
// @Param data body dto.SysDictTypeUpdateReq true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/dict/type/{dictId} [put]
// @Security Bearer
func (e Dictionary) Update(c *gin.Context) {
	s := service.Dictionary{}
	req := dto.SysDictTypeUpdateReq{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Error(500, err, err.Error())
		e.Logger.Error(err)
		return
	}
	req.SetUpdateBy(user.ID(c))
	err = s.Update(&req)
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, "更新失败")
		return
	}
	e.OK(req.GetID(), "更新成功")
}

// Delete removes selected records within their data permissions.
// @Summary 删除字典类型
// @Description 删除数据
// @Tags 字典类型
// @Param dictCode body dto.SysDictTypeDeleteReq true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/dict/type [delete]
// @Security Bearer
func (e Dictionary) Delete(c *gin.Context) {
	s := service.Dictionary{}
	req := dto.SysDictTypeDeleteReq{}
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
	err = s.Delete(&req)
	if err != nil {
		e.Error(500, err, err.Error())
		return
	}
	e.OK(req.GetID(), "删除成功")
}

// All performs the get all administration operation.
// @Summary 字典类型全部数据 代码生成使用接口
// @Description 获取JSON
// @Tags 字典类型
// @Param dictName query string false "dictName"
// @Param dictId query string false "dictId"
// @Param dictType query string false "dictType"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/dict/type-option-select [get]
// @Security Bearer
func (e Dictionary) All(c *gin.Context) {
	s := service.Dictionary{}
	req := dto.SysDictTypeGetPageReq{}
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
	list := make([]models.SysDictType, 0)
	err = s.All(&req, &list)
	if err != nil {
		e.Error(500, err, err.Error())
		return
	}
	e.OK(list, "查询成功")
}
