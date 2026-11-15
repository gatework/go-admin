package handler

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/admin/service/dto"
	"github.com/gatework/motor/common/request"
)

// OperationLog handles HTTP requests for its administration resource.
type OperationLog struct {
	api.API
}

// List 操作日志列表
// @Summary 操作日志列表
// @Description 获取JSON
// @Tags 操作日志
// @Param title query string false "title"
// @Param method query string false "method"
// @Param requestMethod  query string false "requestMethod"
// @Param operUrl query string false "operUrl"
// @Param operIp query string false "operIp"
// @Param status query string false "status"
// @Param beginTime query string false "beginTime"
// @Param endTime query string false "endTime"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/sys-opera-log [get]
// @Security Bearer
func (e OperationLog) List(c *gin.Context) {
	s := service.SysOperaLog{}
	req := new(dto.SysOperaLogGetPageReq)
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(req)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	list := make([]models.SysOperaLog, 0)
	var count int64

	err = s.GetPage(req, &list, &count)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}

	e.PageOK(list, int(count), req.GetPageIndex(), req.GetPageSize(), "查询成功")
}

// Get 操作日志通过id获取
// @Summary 操作日志通过id获取
// @Description 获取JSON
// @Tags 操作日志
// @Param id path string false "id"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/sys-opera-log/{id} [get]
// @Security Bearer
func (e OperationLog) Get(c *gin.Context) {
	s := new(service.SysOperaLog)
	req := dto.SysOperaLogGetReq{}
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
	var object models.SysOperaLog
	err = s.Get(&req, &object)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}
	e.OK(object, "查询成功")
}

// Delete 操作日志删除
// DeleteSysMenu 操作日志删除
// @Summary 删除操作日志
// @Description 删除数据
// @Tags 操作日志
// @Param data body dto.SysOperaLogDeleteReq true "body"
// @Success 200 {object} response.Response "{"code": 200, "data": [...]}"
// @Router /api/v1/sys-opera-log [delete]
// @Security Bearer
func (e OperationLog) Delete(c *gin.Context) {
	s := new(service.SysOperaLog)
	req := dto.SysOperaLogDeleteReq{}
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

	err = s.Remove(&req)
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, fmt.Sprintf("删除失败！错误详情：%s", err.Error()))
		return
	}
	e.OK(req.GetID(), "删除成功")
}
