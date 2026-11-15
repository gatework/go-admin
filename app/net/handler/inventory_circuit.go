package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

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

// Circuit handles the network HTTP operations for each request.
type Circuit struct{ api.API }

// List returns the caller's circuit inventory.
// @Summary 查询专线台账
// @Tags 专线管理
// @Produce json
// @Param pageIndex query int false "页码"
// @Param pageSize query int false "每页数量，最多 200"
// @Param keyword query string false "编号、名称、合同、运营商、地区或组织"
// @Param status query string false "状态"
// @Param category query string false "类型"
// @Param provider query string false "运营商"
// @Param region query string false "地区"
// @Param organization query string false "组织"
// @Param monitored query string false "true 或 false"
// @Success 200 {object} response.Response
// @Router /api/v1/network-circuit [get]
// @Security Bearer
func (e Circuit) List(c *gin.Context) {
	s, ok := prepareCircuit(&e, c)
	if !ok {
		return
	}
	r, ok := e.pageRequest(c)
	if !ok {
		return
	}
	rows, count, err := s.Page(c.Request.Context(), r, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(count), r.PageIndex, r.PageSize, "查询成功")
}

// Get returns a circuit inside the caller's data scope.
// @Summary 查看专线详情
// @Tags 专线管理
// @Produce json
// @Param id path int true "专线 ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-circuit/{id} [get]
// @Security Bearer
func (e Circuit) Get(c *gin.Context) {
	s, ok := prepareCircuit(&e, c)
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

// Create validates local fields, readable devices and changed Zabbix bindings.
// @Summary 新增专线
// @Tags 专线管理
// @Accept json
// @Produce json
// @Param data body dto.CircuitControlReq true "专线台账"
// @Success 200 {object} response.Response
// @Failure 422 {object} response.Response "线路编号已存在"
// @Router /api/v1/network-circuit [post]
// @Security Bearer
func (e Circuit) Create(c *gin.Context) {
	s, ok := prepareCircuit(&e, c)
	if !ok {
		return
	}
	r := &dto.CircuitControlReq{}
	if !e.body(c, r) {
		return
	}
	row, err := s.Create(c.Request.Context(), r, user.ID(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "创建成功")
}

// Update uses a compare-and-swap version predicate for all editable fields.
// @Summary 编辑专线
// @Tags 专线管理
// @Accept json
// @Produce json
// @Param id path int true "专线 ID"
// @Param data body dto.CircuitControlReq true "专线台账与 version"
// @Success 200 {object} response.Response
// @Failure 409 {object} response.Response "版本冲突"
// @Failure 422 {object} response.Response "线路编号已存在"
// @Router /api/v1/network-circuit/{id} [put]
// @Security Bearer
func (e Circuit) Update(c *gin.Context) {
	s, ok := prepareCircuit(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c)
	if !ok {
		return
	}
	r := &dto.CircuitControlReq{}
	if !e.body(c, r) {
		return
	}
	row, err := s.Update(c.Request.Context(), id, r, user.ID(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "保存成功")
}

// Delete only removes a planned circuit at the submitted version.
// @Summary 删除规划中专线
// @Tags 专线管理
// @Accept json
// @Produce json
// @Param id path int true "专线 ID"
// @Param data body dto.CircuitDeleteReq true "version"
// @Success 200 {object} response.Response
// @Failure 409 {object} response.Response "版本冲突"
// @Router /api/v1/network-circuit/{id} [delete]
// @Security Bearer
func (e Circuit) Delete(c *gin.Context) {
	s, ok := prepareCircuit(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c)
	if !ok {
		return
	}
	r := &dto.CircuitDeleteReq{}
	if !e.body(c, r) {
		return
	}
	if err := s.Delete(c.Request.Context(), id, r.Version, user.ID(c), actions.GetPermissionFromContext(c)); err != nil {
		e.respond(err)
		return
	}
	e.OK(nil, "删除成功")
}

// Options returns values from visible records and a credential-free config flag.
// @Summary 专线筛选选项
// @Tags 专线管理
// @Produce json
// @Success 200 {object} response.Response
// @Router /api/v1/network-circuit/options [get]
// @Security Bearer
func (e Circuit) Options(c *gin.Context) {
	s, ok := prepareCircuit(&e, c)
	if !ok {
		return
	}
	result, err := s.Options(c.Request.Context(), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(result, "查询成功")
}

// Export downloads all current-filter matches, up to the explicit 10000 cap.
// @Summary 导出筛选专线台账
// @Tags 专线管理
// @Produce text/csv
// @Success 200 {file} file
// @Router /api/v1/network-circuit/export [get]
// @Security Bearer
func (e Circuit) Export(c *gin.Context) {
	s, ok := prepareCircuit(&e, c)
	if !ok {
		return
	}
	r, ok := e.pageRequest(c)
	if !ok {
		return
	}
	data, err := s.Export(c.Request.Context(), r, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="network-circuits.csv"`)
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", data)
}

// Hosts searches existing Zabbix hosts; callers cannot supply URL or credentials.
// @Summary 搜索现有 Zabbix 主机
// @Tags 专线管理
// @Produce json
// @Param keyword query string true "1 至 128 个字符"
// @Success 200 {object} response.Response
// @Router /api/v1/network-circuit/zabbix/hosts [get]
// @Security Bearer
func (e Circuit) Hosts(c *gin.Context) {
	s, ok := prepareCircuit(&e, c)
	if !ok {
		return
	}
	result, err := s.Hosts(c.Request.Context(), c.Query("keyword"))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(result, "查询成功")
}

// Items lists enabled numeric bps/Bps items belonging to the selected host.
// @Summary 查询现有 Zabbix 速率监控项
// @Tags 专线管理
// @Produce json
// @Param hostId query string true "Zabbix 主机 ID"
// @Param keyword query string false "名称或键值，最多 128 个字符"
// @Success 200 {object} response.Response
// @Router /api/v1/network-circuit/zabbix/items [get]
// @Security Bearer
func (e Circuit) Items(c *gin.Context) {
	s, ok := prepareCircuit(&e, c)
	if !ok {
		return
	}
	result, err := s.Items(c.Request.Context(), c.Query("hostId"), c.Query("keyword"))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(result, "查询成功")
}

// Traffic reads only the stored bindings after the scoped circuit lookup.
// @Summary 查看专线双向流量
// @Tags 专线管理
// @Produce json
// @Param id path int true "专线 ID"
// @Param period query string false "1h、6h、24h 或 7d"
// @Success 200 {object} response.Response
// @Router /api/v1/network-circuit/{id}/traffic [get]
// @Security Bearer
func (e Circuit) Traffic(c *gin.Context) {
	s, ok := prepareCircuit(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c)
	if !ok {
		return
	}
	result, err := s.Traffic(c.Request.Context(), id, c.DefaultQuery("period", "1h"), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(result, "查询成功")
}

func prepareCircuit(e *Circuit, c *gin.Context) (*service.Circuit, bool) {
	s := &service.Circuit{}
	var err error
	e.API, err = request.New(c)
	s.Service = e.Service()
	if err != nil {
		e.respond(err)
		return nil, false
	}
	return s, true
}

func (e Circuit) respond(err error) {
	code, message := 500, "专线操作失败，请稍后重试"
	if safe, ok := errors.AsType[*inventory.Error](err); ok {
		code, message = safe.Code, safe.Message
	}
	e.Error(code, nil, message)
}

func (e Circuit) id(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		e.respond(inventory.InvalidInputErr)
		return 0, false
	}
	return id, true
}

func (e Circuit) pageRequest(c *gin.Context) (*dto.CircuitPageReq, bool) {
	r := &dto.CircuitPageReq{}
	if c.ShouldBindQuery(r) != nil || r.Validate() != nil {
		e.respond(inventory.InvalidInputErr)
		return nil, false
	}
	return r, true
}

func (e Circuit) body(c *gin.Context, out any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil || decoder.Decode(new(any)) != io.EOF {
		e.respond(inventory.InvalidInputErr)
		return false
	}
	return true
}
