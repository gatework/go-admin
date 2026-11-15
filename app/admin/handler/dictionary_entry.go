package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger annotations require the response schema. // Registers the documentation or route assets used by this package.
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/common/request"

	"github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/admin/service/dto"
)

// DictionaryEntry handles HTTP requests for its administration resource.
type DictionaryEntry struct {
	api.API
}

// List lists records matching the request and data permissions.
// @Summary 字典数据列表
// @Description 获取JSON
// @Tags 字典数据
// @Param status query string false "status"
// @Param dictCode query string false "dictCode"
// @Param dictType query string false "dictType"
// @Param pageSize query int false "页条数"
// @Param pageIndex query int false "页码"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/dict/data [get]
// @Security Bearer
func (e DictionaryEntry) List(c *gin.Context) {
	s := service.DictionaryEntry{}
	req := dto.DictionaryEntrySearch{}
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

	list := make([]models.SysDictData, 0)
	var count int64
	err = s.List(c.Request.Context(), &req, &list, &count)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}

	e.PageOK(list, int(count), req.GetPageIndex(), req.GetPageSize(), "查询成功")
}

// Get loads the requested record within its data permissions.
// @Summary 通过编码获取字典数据
// @Description 获取JSON
// @Tags 字典数据
// @Param dictCode path int true "字典编码"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/dict/data/{dictCode} [get]
// @Security Bearer
func (e DictionaryEntry) Get(c *gin.Context) {
	s := service.DictionaryEntry{}
	req := dto.DictionaryEntryByID{}
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

	var object models.SysDictData

	err = s.Get(c.Request.Context(), &req, &object)
	if err != nil {
		e.Logger.Warnf("Get error: %s", err.Error())
		e.Error(500, err, "查询失败")
		return
	}

	e.OK(object, "查询成功")
}

// Create creates a record from the validated request.
// @Summary 添加字典数据
// @Description 获取JSON
// @Tags 字典数据
// @Accept  application/json
// @Product application/json
// @Param data body dto.DictionaryEntryCreate true "data"
// @Success 200 {object} response.Response	"{"code": 200, "message": "添加成功"}"
// @Router /api/v1/dict/data [post]
// @Security Bearer
func (e DictionaryEntry) Create(c *gin.Context) {
	s := service.DictionaryEntry{}
	req := dto.DictionaryEntryCreate{}
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
	err = s.Create(c.Request.Context(), &req)
	if err != nil {
		e.Error(500, err, "创建失败")
		return
	}

	e.OK(req.GetID(), "创建成功")
}

// Update updates an existing record within its data permissions.
// @Summary 修改字典数据
// @Description 获取JSON
// @Tags 字典数据
// @Accept  application/json
// @Product application/json
// @Param data body dto.DictionaryEntryUpdate true "body"
// @Success 200 {object} response.Response	"{"code": 200, "message": "修改成功"}"
// @Router /api/v1/dict/data/{dictCode} [put]
// @Security Bearer
func (e DictionaryEntry) Update(c *gin.Context) {
	s := service.DictionaryEntry{}
	req := dto.DictionaryEntryUpdate{}
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
		e.Error(500, err, "更新失败")
		return
	}
	e.OK(req.GetID(), "更新成功")
}

// Delete removes selected records within their data permissions.
// @Summary 删除字典数据
// @Description 删除数据
// @Tags 字典数据
// @Param dictCode body dto.DictionaryEntryDelete true "body"
// @Success 200 {object} response.Response	"{"code": 200, "message": "删除成功"}"
// @Router /api/v1/dict/data [delete]
// @Security Bearer
func (e DictionaryEntry) Delete(c *gin.Context) {
	s := service.DictionaryEntry{}
	req := dto.DictionaryEntryDelete{}
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
	err = s.Delete(c.Request.Context(), &req)
	if err != nil {
		e.Error(500, err, "删除失败")
		return
	}
	e.OK(req.GetID(), "删除成功")
}

// All 数据字典根据key获取 业务页面使用
// @Summary 数据字典根据key获取
// @Description 数据字典根据key获取
// @Tags 字典数据
// @Param dictType query int true "dictType"
// @Success 200 {object} response.Response{data=[]dto.DictionaryOption}  "{"code": 200, "data": [...]}"
// @Router /api/v1/dict-data/option-select [get]
// @Security Bearer
func (e DictionaryEntry) All(c *gin.Context) {
	s := service.DictionaryEntry{}
	req := dto.DictionaryEntrySearch{}
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
	list := make([]models.SysDictData, 0)
	err = s.All(c.Request.Context(), &req, &list)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}
	l := make([]dto.DictionaryOption, 0)
	for _, i := range list {
		d := dto.DictionaryOption{DictLabel: i.DictLabel, DictValue: i.DictValue}
		l = append(l, d)
	}

	e.OK(l, "查询成功")
}
