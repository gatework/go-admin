package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/jwtauth/user"
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor/common/middleware"
	"github.com/gatework/motor/common/request"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service"
	"github.com/gatework/motor/app/admin/service/dto"
)

// Config handles HTTP requests for its administration resource.
type Config struct {
	api.API
}

// List 获取配置管理列表
// @Summary 获取配置管理列表
// @Description 获取配置管理列表
// @Tags 配置管理
// @Param configName query string false "名称"
// @Param configKey query string false "key"
// @Param configType query string false "类型"
// @Param isFrontend query int false "是否前端"
// @Param pageSize query int false "页条数"
// @Param pageIndex query int false "页码"
// @Success 200 {object} response.Response{data=response.Page{list=[]models.SysAPI}} "{"code": 200, "data": [...]}"
// @Router /api/v1/sys-config [get]
// @Security Bearer
func (e Config) List(c *gin.Context) {
	s := service.SysConfig{}
	req := dto.SysConfigGetPageReq{}
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

	list := make([]models.SysConfig, 0)
	var count int64
	err = s.GetPage(&req, &list, &count)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}
	e.PageOK(list, int(count), req.GetPageIndex(), req.GetPageSize(), "查询成功")
}

// Get 获取配置管理
// @Summary 获取配置管理
// @Description 获取配置管理
// @Tags 配置管理
// @Param id path string false "id"
// @Success 200 {object} response.Response{data=models.SysConfig} "{"code": 200, "data": [...]}"
// @Router /api/v1/sys-config/{id} [get]
// @Security Bearer
func (e Config) Get(c *gin.Context) {
	req := dto.SysConfigGetReq{}
	s := service.SysConfig{}
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
	var object models.SysConfig

	err = s.Get(&req, &object)
	if err != nil {
		e.Error(500, err, err.Error())
		return
	}

	e.OK(object, "查询成功")
}

// Create 创建配置管理
// @Summary 创建配置管理
// @Description 创建配置管理
// @Tags 配置管理
// @Accept application/json
// @Product application/json
// @Param data body dto.SysConfigControl true "body"
// @Success 200 {object} response.Response	"{"code": 200, "message": "创建成功"}"
// @Router /api/v1/sys-config [post]
// @Security Bearer
func (e Config) Create(c *gin.Context) {
	s := service.SysConfig{}
	req := dto.SysConfigControl{}
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
		e.Error(500, err, "创建失败")
		return
	}
	e.OK(req.GetID(), "创建成功")
}

// Update 修改配置管理
// @Summary 修改配置管理
// @Description 修改配置管理
// @Tags 配置管理
// @Accept application/json
// @Product application/json
// @Param data body dto.SysConfigControl true "body"
// @Success 200 {object} response.Response	"{"code": 200, "message": "修改成功"}"
// @Router /api/v1/sys-config/{id} [put]
// @Security Bearer
func (e Config) Update(c *gin.Context) {
	s := service.SysConfig{}
	req := dto.SysConfigControl{}
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

// Delete 删除配置管理
// @Summary 删除配置管理
// @Description 删除配置管理
// @Tags 配置管理
// @Param ids body []int false "ids"
// @Success 200 {object} response.Response	"{"code": 200, "message": "删除成功"}"
// @Router /api/v1/sys-config [delete]
// @Security Bearer
func (e Config) Delete(c *gin.Context) {
	s := service.SysConfig{}
	req := dto.SysConfigDeleteReq{}
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
		e.Error(500, err, "删除失败")
		return
	}
	e.OK(req.GetID(), "删除成功")
}

// AppConfig 获取系统配置信息
// @Summary 获取系统前台配置信息，主要注意这里不在验证权限
// @Description 获取系统配置信息，主要注意这里不在验证权限
// @Tags 配置管理
// @Success 200 {object} response.Response{data=map[string]string} "{"code": 200, "data": [...]}"
// @Router /api/v1/app-config [get]
func (e Config) AppConfig(c *gin.Context) {
	req := dto.SysConfigGetToSysAppReq{}
	s := service.SysConfig{}
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
	// 控制只读前台的数据
	req.IsFrontend = "1"
	list := make([]models.SysConfig, 0)
	err = s.GetWithKeyList(&req, &list)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}
	mp := make(map[string]string)
	for i := range list {
		key := list[i].ConfigKey
		if key != "" {
			mp[key] = list[i].ConfigValue
		}
	}
	e.OK(mp, "查询成功")
}

// Settings 获取配置
// @Summary 获取配置
// @Description 界面操作设置配置值的获取
// @Tags 配置管理
// @Accept application/json
// @Product application/json
// @Success 200 {object} response.Response{data=map[string]any}	"{"code": 200, "message": "修改成功"}"
// @Router /api/v1/set-config [get]
// @Security Bearer
func (e Config) Settings(c *gin.Context) {
	s := service.SysConfig{}
	req := make([]dto.GetSetSysConfigReq, 0)
	var err error
	e.API, err = request.New(c)
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}
	err = s.GetForSet(&req)
	if err != nil {
		e.Error(500, err, "查询失败")
		return
	}
	m := make(map[string]any, 0)
	for _, v := range req {
		m[v.ConfigKey] = v.ConfigValue
	}
	e.OK(m, "查询成功")
}

// UpdateSettings 设置配置
// @Summary 设置配置
// @Description 界面操作设置配置值
// @Tags 配置管理
// @Accept application/json
// @Product application/json
// @Param data body []dto.GetSetSysConfigReq true "body"
// @Success 200 {object} response.Response	"{"code": 200, "message": "修改成功"}"
// @Router /api/v1/set-config [put]
// @Security Bearer
func (e Config) UpdateSettings(c *gin.Context) {
	s := service.SysConfig{}
	req := make([]dto.GetSetSysConfigReq, 0)
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

	err = s.UpdateForSet(&req)
	if err != nil {
		e.Error(500, err, err.Error())
		return
	}

	e.OK("", "更新成功")
}

// GetByKey 根据Key获取SysConfig的Service
// @Summary 根据Key获取SysConfig的Service
// @Description 根据Key获取SysConfig的Service
// @Tags 配置管理
// @Param configKey path string false "configKey"
// @Success 200 {object} response.Response{data=dto.SysConfigByKeyReq} "{"code": 200, "data": [...]}"
// @Router /api/v1/sys-config/{id} [get]
// @Security Bearer
func (e Config) GetByKey(c *gin.Context) {
	var s = new(service.SysConfig)
	var req = new(dto.SysConfigByKeyReq)
	var resp = new(dto.GetSysConfigByKEYForServiceResp)
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

	err = s.GetWithKey(req, resp)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		e.Error(404, nil, "配置不存在")
		return
	}
	if err != nil {
		e.Error(500, nil, "查询失败")
		return
	}
	// Any signed-in user may read a frontend key. Every other key (the initial
	// password among them) needs the permission to read configuration; an
	// unknown key and a forbidden one answer alike so keys cannot be probed.
	if resp.IsFrontend != "1" {
		allowed, err := middleware.EnforceRoleFor(c, "/api/v1/config", http.MethodGet)
		if err != nil || !allowed {
			e.Error(404, nil, "配置不存在")
			return
		}
	}
	e.OK(resp, "查询成功")
}
