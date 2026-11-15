package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger response schemas are resolved from this import.
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/contract/actions"
	service "github.com/gatework/motor/app/net/service/delivery"
	dto "github.com/gatework/motor/app/net/service/dto/delivery"
	ndto "github.com/gatework/motor/app/net/service/dto/inventory"
	"github.com/gatework/motor/common/request"
)

// Delivery handles the delivery HTTP operations for each request.
type Delivery struct{ api.API }

// Options returns safe server configuration.
// @Summary 配置下发选项
// @Tags 配置下发
// @Produce json
// @Success 200 {object} response.Response
// @Router /api/v1/network-delivery/options [get]
// @Security Bearer
func (e Delivery) Options(c *gin.Context) {
	s, ok := prepareDelivery(&e, c)
	if !ok {
		return
	}
	out, err := s.Options(c.Request.Context(), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(out, "查询成功")
}

// Devices returns scoped inventory and connection readiness.
// @Summary 选择下发设备
// @Tags 配置下发
// @Produce json
// @Success 200 {object} response.Response
// @Router /api/v1/network-delivery/devices [get]
// @Security Bearer
func (e Delivery) Devices(c *gin.Context) {
	s, ok := prepareDelivery(&e, c)
	if !ok {
		return
	}
	r := &ndto.DevicePageReq{}
	if c.ShouldBindQuery(r) != nil || r.Validate() != nil {
		e.respond(service.InvalidErr)
		return
	}
	rows, count, err := s.Devices(c.Request.Context(), r, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(count), r.PageIndex, r.PageSize, "查询成功")
}

// Connection returns effective settings without credentials.
// @Summary 查看设备登录配置
// @Tags 配置下发
// @Produce json
// @Param id path int true "设备 ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-delivery/devices/{id}/connection [get]
// @Security Bearer
func (e Delivery) Connection(c *gin.Context) {
	s, ok := prepareDelivery(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	out, err := s.Connection(c.Request.Context(), id, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(out, "查询成功")
}

// SetConnection updates settings using optimistic locking.
// @Summary 保存设备登录配置
// @Tags 配置下发
// @Accept json
// @Produce json
// @Param id path int true "设备 ID"
// @Param data body dto.Connection true "登录配置"
// @Success 200 {object} response.Response
// @Router /api/v1/network-delivery/devices/{id}/connection [put]
// @Security Bearer
func (e Delivery) SetConnection(c *gin.Context) {
	s, ok := prepareDelivery(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	var r dto.Connection
	if !e.body(c, &r) {
		return
	}
	out, err := s.SetConnection(c.Request.Context(), id, r, user.ID(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(out, "保存成功")
}

// List returns scoped order history.
// @Summary 下发工单列表
// @Tags 配置下发
// @Produce json
// @Success 200 {object} response.Response
// @Router /api/v1/network-delivery/orders [get]
// @Security Bearer
func (e Delivery) List(c *gin.Context) {
	s, ok := prepareDelivery(&e, c)
	if !ok {
		return
	}
	r := dto.Page{PageIndex: 1, PageSize: 20}
	if c.ShouldBindQuery(&r) != nil {
		e.respond(service.InvalidErr)
		return
	}
	rows, count, err := s.Page(c.Request.Context(), r, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(count), r.PageIndex, r.PageSize, "查询成功")
}

// Get returns scripts and statuses, without output bodies.
// @Summary 下发工单详情
// @Tags 配置下发
// @Produce json
// @Param id path int true "工单 ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-delivery/orders/{id} [get]
// @Security Bearer
func (e Delivery) Get(c *gin.Context) {
	s, ok := prepareDelivery(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	out, err := s.Get(c.Request.Context(), id, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(out, "查询成功")
}

// Target returns the bounded retained output.
// @Summary 下发目标输出
// @Tags 配置下发
// @Produce json
// @Param id path int true "工单 ID"
// @Param targetId path int true "目标 ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-delivery/orders/{id}/targets/{targetId} [get]
// @Security Bearer
func (e Delivery) Target(c *gin.Context) {
	s, ok := prepareDelivery(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	tid, ok := e.id(c, "targetId")
	if !ok {
		return
	}
	out, err := s.Target(c.Request.Context(), id, tid, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(out, "查询成功")
}

// Create validates the entire order before scheduling.
// @Summary 创建下发工单
// @Tags 配置下发
// @Accept json
// @Produce json
// @Param data body dto.Create true "工单"
// @Success 200 {object} response.Response
// @Router /api/v1/network-delivery/orders [post]
// @Security Bearer
func (e Delivery) Create(c *gin.Context) {
	s, ok := prepareDelivery(&e, c)
	if !ok {
		return
	}
	var r dto.Create
	if !e.body(c, &r) {
		return
	}
	out, err := s.Create(c.Request.Context(), r, user.ID(c), user.Name(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(out, "创建成功")
}

// Edit replaces a scheduled draft and restarts the grace period.
// @Summary 编辑下发工单
// @Tags 配置下发
// @Accept json
// @Produce json
// @Param id path int true "工单 ID"
// @Param data body dto.Edit true "工单"
// @Success 200 {object} response.Response
// @Router /api/v1/network-delivery/orders/{id} [put]
// @Security Bearer
func (e Delivery) Edit(c *gin.Context) {
	s, ok := prepareDelivery(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	var r dto.Edit
	if !e.body(c, &r) {
		return
	}
	out, err := s.Edit(c.Request.Context(), id, r, user.ID(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(out, "保存成功")
}

// Execute skips the grace period.
// @Summary 立即执行下发工单
// @Tags 配置下发
// @Accept json
// @Produce json
// @Param id path int true "工单 ID"
// @Param data body dto.Action true "版本"
// @Success 200 {object} response.Response
// @Router /api/v1/network-delivery/orders/{id}/execute [post]
// @Security Bearer
func (e Delivery) Execute(c *gin.Context) { e.action(c, "execute") }

// Cancel prevents further sends, without claiming rollback.
// @Summary 取消下发工单
// @Tags 配置下发
// @Accept json
// @Produce json
// @Param id path int true "工单 ID"
// @Param data body dto.Action true "版本"
// @Success 200 {object} response.Response
// @Router /api/v1/network-delivery/orders/{id}/cancel [post]
// @Security Bearer
func (e Delivery) Cancel(c *gin.Context) { e.action(c, "cancel") }

// Retry creates a related order containing only pre-write failures.
// @Summary 重试安全失败目标
// @Tags 配置下发
// @Accept json
// @Produce json
// @Param id path int true "工单 ID"
// @Param data body dto.Action true "版本和请求键"
// @Success 200 {object} response.Response
// @Router /api/v1/network-delivery/orders/{id}/retry [post]
// @Security Bearer
func (e Delivery) Retry(c *gin.Context) { e.action(c, "retry") }

func prepareDelivery(e *Delivery, c *gin.Context) (*service.Workflow, bool) {
	s := &service.Workflow{Config: service.FromEnvironment()}
	var err error
	e.API, err = request.New(c)
	s.Service = e.Service()
	if err != nil {
		e.respond(err)
		return nil, false
	}
	c.Header("Cache-Control", "no-store")
	return s, true
}

// respond answers err by its kind; a missing row is NotFoundErr.
func (e Delivery) respond(err error) { fail(&e.API, err, service.NotFoundErr) }

func (e Delivery) id(c *gin.Context, key string) (int, bool) {
	id, ok := pathID(c, key)
	if !ok {
		e.respond(service.InvalidErr)
	}
	return id, ok
}

func (e Delivery) body(c *gin.Context, v any) bool {
	if !decodeStrictJSON(c, v, (service.MaxTotalBytes*6)+(64<<10)) {
		e.respond(service.InvalidErr)
		return false
	}
	return true
}
func (e Delivery) action(c *gin.Context, action string) {
	s, ok := prepareDelivery(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	var r dto.Action
	if !e.body(c, &r) {
		return
	}
	if action == "retry" {
		out, err := s.Retry(c.Request.Context(), id, r, user.ID(c), user.Name(c), actions.GetPermissionFromContext(c))
		if err != nil {
			e.respond(err)
			return
		}
		e.OK(out, "创建成功")
		return
	}
	out, err := s.Action(c.Request.Context(), id, r.Version, action, user.ID(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(out, "操作成功")
}
