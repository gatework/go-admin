package apis

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	jwt "github.com/go-admin-team/go-admin-core/v2/jwtauth"
	"github.com/go-admin-team/go-admin-core/v2/jwtauth/user"
	_ "github.com/go-admin-team/go-admin-core/v2/response"
	"github.com/go-admin-team/go-admin-core/v2/sdk/api"
	"github.com/go-admin-team/go-admin-core/v2/sdk/contract/actions"
	"go-admin/app/network"
	"go-admin/app/network/service"
	"go-admin/app/network/service/dto"
)

type Device struct{ api.Api }

func (e *Device) prepare(c *gin.Context) (*service.Device, bool) {
	s := &service.Device{}
	if err := e.MakeContext(c).MakeOrm().MakeService(&s.Service).Errors; err != nil {
		e.respond(err)
		return nil, false
	}
	return s, true
}
func (e *Device) respond(err error) {
	code, message := 500, "台账操作失败，请稍后重试"
	var safe *network.Error
	if errors.As(err, &safe) {
		code, message = safe.Code, safe.Message
	}
	// response.Error may log the supplied error. Give it only the public
	// message, never raw upstream/SQL text or a request containing credentials.
	e.Error(code, nil, message)
}
func (e *Device) pageRequest(c *gin.Context) (*dto.DevicePageReq, bool) {
	req := &dto.DevicePageReq{}
	if c.ShouldBindQuery(req) != nil || req.Validate() != nil {
		e.respond(network.ErrInvalidInput)
		return nil, false
	}
	return req, true
}
func (e *Device) id(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		e.respond(network.ErrInvalidInput)
		return 0, false
	}
	return id, true
}

// GetPage returns the caller's persisted inventory snapshot.
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
func (e Device) GetPage(c *gin.Context) {
	s, ok := e.prepare(c)
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
	s, ok := e.prepare(c)
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
	s, ok := e.prepare(c)
	if !ok {
		return
	}
	id, ok := e.id(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var req dto.DeviceUpdateReq
	if err := decoder.Decode(&req); err != nil {
		e.respond(network.ErrInvalidInput)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		e.respond(network.ErrInvalidInput)
		return
	}
	row, err := s.Update(c.Request.Context(), id, &req, user.GetUserId(c), actions.GetPermissionFromContext(c))
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
	s, ok := e.prepare(c)
	if !ok {
		return
	}
	var req dto.DeviceGroupAssignReq
	if !decodeDeviceBody(c, &req) {
		e.respond(network.ErrInvalidInput)
		return
	}
	updated, err := s.AssignGroup(c.Request.Context(), &req, user.GetUserId(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(struct {
		Updated int `json:"updated"`
	}{updated}, "分组已更新")
}

func decodeDeviceBody(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil && decoder.Decode(new(any)) == io.EOF
}

// Options returns distinct source/manual filter values inside the caller's scope.
// @Summary 设备台账筛选选项
// @Tags 设备台账
// @Produce json
// @Success 200 {object} response.Response
// @Router /api/v1/network-device/options [get]
// @Security Bearer
func (e Device) Options(c *gin.Context) {
	s, ok := e.prepare(c)
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
	s, ok := e.prepare(c)
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

// Sync refreshes facts without modifying manual metadata or deleting absent rows.
// @Summary 从 Netdisco 同步设备台账
// @Tags 设备台账
// @Produce json
// @Success 200 {object} response.Response
// @Failure 409 {object} response.Response
// @Router /api/v1/network-device/sync [post]
// @Security Bearer
func (e Device) Sync(c *gin.Context) {
	s, ok := e.prepare(c)
	if !ok {
		return
	}
	// The upstream is exclusively environment-configured; this operation has no body.
	if body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1)); err != nil || len(body) != 0 {
		e.respond(network.ErrInvalidInput)
		return
	}
	admin := jwt.ExtractClaims(c)[jwt.RoleKey] == "admin"
	result, err := s.Sync(c.Request.Context(), user.GetUserId(c), actions.GetPermissionFromContext(c), admin)
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(result, "同步完成")
}

// Export downloads every matching row the caller can view, without pagination.
// @Summary 导出设备台账 CSV
// @Tags 设备台账
// @Produce text/csv
// @Param keyword query string false "检索关键字"
// @Success 200 {file} binary
// @Router /api/v1/network-device/export [get]
// @Security Bearer
func (e Device) Export(c *gin.Context) {
	s, ok := e.prepare(c)
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
