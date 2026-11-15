package handler

import (
	"io"
	"net/http"
	"time"

	"github.com/gatework/motor/common/account"

	"github.com/gatework/motor/common/request"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger response schemas are resolved from this import.
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/inventory"
	dto "github.com/gatework/motor/app/net/service/dto/inventory"
	service "github.com/gatework/motor/app/net/service/inventory"
)

// Device handles the network HTTP operations for each request.
type Device struct{ api.API }

// List returns the caller's persisted inventory snapshot.
// @Summary 查询设备台账
// @Tags 设备台账
// @Produce json
// @Param keyword query string false "名称/IP/序列号/厂商/型号/位置"
// @Param vendor query string false "厂商"
// @Param model query string false "型号"
// @Param status query string false "台账状态"
// @Param deviceType query string false "设备类型"
// @Param deviceRole query string false "设备角色"
// @Param groupId query int false "分组子树，0 为未分组"
// @Param tag query string false "标签"
// @Param pageIndex query int false "页码"
// @Param pageSize query int false "每页数量，最多 200"
// @Success 200 {object} response.Response
// @Router /api/v1/network-device [get]
// @Security Bearer
func (e Device) List(c *gin.Context) {
	s, ok := prepareDevice(&e, c)
	if !ok {
		return
	}
	req, ok := e.pageRequest(c)
	if !ok {
		return
	}
	rows, count, err := s.Page(c.Request.Context(), req, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(count), req.PageIndex, req.PageSize, "查询成功")
}

// Get returns a device inside the caller's data scope.
// @Summary 查看设备台账
// @Tags 设备台账
// @Produce json
// @Param id path int true "设备 ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-device/{id} [get]
// @Security Bearer
func (e Device) Get(c *gin.Context) {
	s, ok := prepareDevice(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c)
	if !ok {
		return
	}
	row, err := s.Get(c.Request.Context(), id, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "查询成功")
}

// Update changes manual inventory fields using an optimistic version check.
// @Summary 编辑设备台账资料
// @Tags 设备台账
// @Accept json
// @Produce json
// @Param id path int true "设备 ID"
// @Param data body dto.DeviceUpdateReq true "人工资料与 version"
// @Success 200 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/network-device/{id} [put]
// @Security Bearer
func (e Device) Update(c *gin.Context) {
	s, ok := prepareDevice(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c)
	if !ok {
		return
	}
	var req dto.DeviceUpdateReq
	if !decodeStrictJSON(c, &req, 64<<10) {
		e.respond(inventory.InvalidErr)
		return
	}
	row, err := s.Update(c.Request.Context(), id, &req, user.ID(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "保存成功")
}

// AssignGroup changes all selected memberships in one scoped transaction.
// @Summary 批量调整设备分组
// @Tags 设备台账
// @Accept json
// @Produce json
// @Param data body dto.DeviceGroupAssignReq true "设备版本与分组来源"
// @Success 200 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/network-device/assign-group [put]
// @Security Bearer
func (e Device) AssignGroup(c *gin.Context) {
	s, ok := prepareDevice(&e, c)
	if !ok {
		return
	}
	var req dto.DeviceGroupAssignReq
	if !decodeDeviceBody(c, &req) {
		e.respond(inventory.InvalidErr)
		return
	}
	updated, err := s.AssignGroup(c.Request.Context(), &req, user.ID(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(struct {
		Updated int `json:"updated"`
	}{updated}, "分组已更新")
}

// Options returns distinct source/manual filter values inside the caller's scope.
// @Summary 设备台账筛选选项
// @Tags 设备台账
// @Produce json
// @Success 200 {object} response.Response
// @Router /api/v1/network-device/options [get]
// @Security Bearer
func (e Device) Options(c *gin.Context) {
	s, ok := prepareDevice(&e, c)
	if !ok {
		return
	}
	options, err := s.Options(c.Request.Context(), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(options, "查询成功")
}

// SyncStatus returns configuration and persistent synchronization state.
// @Summary 设备台账同步状态
// @Tags 设备台账
// @Produce json
// @Success 200 {object} response.Response
// @Router /api/v1/network-device/sync-status [get]
// @Security Bearer
func (e Device) SyncStatus(c *gin.Context) {
	s, ok := prepareDevice(&e, c)
	if !ok {
		return
	}
	status, err := s.SyncStatus(c.Request.Context())
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(status, "查询成功")
}

// Sync queues a Netdisco sync and answers with the queued state; poll
// sync-status for its outcome. A sync refreshes facts without modifying manual
// metadata or deleting absent rows.
// @Summary 排队从 Netdisco 同步设备台账
// @Tags 设备台账
// @Produce json
// @Success 200 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/network-device/sync [post]
// @Security Bearer
func (e Device) Sync(c *gin.Context) {
	s, ok := prepareDevice(&e, c)
	if !ok {
		return
	}
	// The upstream is exclusively environment-configured; this operation has no body.
	if body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1)); err != nil || len(body) != 0 {
		e.respond(inventory.InvalidErr)
		return
	}
	admin := account.IsAdmin(user.RoleName(c))
	status, err := s.RequestSync(c.Request.Context(), user.ID(c), actions.GetPermissionFromContext(c), admin)
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(status, "同步已排队")
}

// Export downloads every matching row the caller can view, without
// pagination, up to inventory.MaxExportRows; a larger match is refused.
// @Summary 导出设备台账 CSV
// @Tags 设备台账
// @Produce text/csv
// @Param keyword query string false "检索关键字"
// @Success 200 {file} binary
// @Router /api/v1/network-device/export [get]
// @Security Bearer
func (e Device) Export(c *gin.Context) {
	s, ok := prepareDevice(&e, c)
	if !ok {
		return
	}
	req, ok := e.pageRequest(c)
	if !ok {
		return
	}
	body, err := s.Export(c.Request.Context(), req, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=network-devices-"+time.Now().UTC().Format("20060102")+".csv")
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", body)
}

func decodeDeviceBody(c *gin.Context, target any) bool {
	return decodeStrictJSON(c, target, 64<<10)
}

func prepareDevice(e *Device, c *gin.Context) (*service.Device, bool) {
	s := &service.Device{}
	var err error
	e.API, err = request.New(c)
	s.Service = e.Service()
	if err != nil {
		e.respond(err)
		return nil, false
	}
	return s, true
}

// respond answers err by its kind and code. Inventory errors are declared
// safe to show; anything unclassified is logged and answered internal_error,
// never with upstream, SQL or credential text.
func (e Device) respond(err error) { request.Fail(&e.API, err) }

func (e Device) pageRequest(c *gin.Context) (*dto.DevicePageReq, bool) {
	req := &dto.DevicePageReq{}
	if c.ShouldBindQuery(req) != nil || req.Validate() != nil {
		e.respond(inventory.InvalidErr)
		return nil, false
	}
	return req, true
}

func (e Device) id(c *gin.Context) (int, bool) {
	id, ok := pathID(c, "id")
	if !ok {
		e.respond(inventory.InvalidErr)
	}
	return id, ok
}
