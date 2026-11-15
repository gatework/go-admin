package handler

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/gatework/motor/common/request"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger response schemas are resolved from this import.
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/contract/actions"
	dto "github.com/gatework/motor/app/net/service/dto/fabric"
	service "github.com/gatework/motor/app/net/service/fabric"
)

// Fabric handles the fabric HTTP operations for each request.
type Fabric struct{ api.API }

// List returns scoped Fabric summaries.
// @Summary Fabric 列表
// @Tags Fabric
// @Produce json
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics [get]
// @Security Bearer
func (e Fabric) List(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	r, ok := e.page(c)
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

// Get returns analysis fixed to the current visible membership and evidence versions.
// @Summary Fabric 详情
// @Tags Fabric
// @Produce json
// @Param id path int true "Fabric ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/{id} [get]
// @Security Bearer
func (e Fabric) Get(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	row, err := s.Detail(c.Request.Context(), id, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "查询成功")
}

// Rules returns the versioned membership policy.
// @Summary Fabric 归属规则
// @Tags Fabric
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/rules [get]
// @Security Bearer
func (e Fabric) Rules(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	row, err := s.Rules(c.Request.Context())
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "查询成功")
}

// PreviewRules evaluates rules without saving.
// @Summary 预览 Fabric 规则
// @Tags Fabric
// @Accept json
// @Param data body dto.RulesReq true "规则"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/rules/preview [post]
// @Security Bearer
func (e Fabric) PreviewRules(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	var r dto.RulesReq
	if !decodeFabricRequest(c, &r) {
		e.respond(service.InvalidErr)
		return
	}
	rows, err := s.PreviewRules(c.Request.Context(), &r, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(rows, "预览成功")
}
func decodeFabricRequest(c *gin.Context, target any) bool {
	return decodeStrictJSON(c, target, 128<<10)
}

// SaveRules updates rules with an optimistic version predicate.
// @Summary 保存 Fabric 规则
// @Tags Fabric
// @Accept json
// @Param data body dto.RulesReq true "规则和版本"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/rules [put]
// @Security Bearer
func (e Fabric) SaveRules(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	var r dto.RulesReq
	if !decodeFabricRequest(c, &r) {
		e.respond(service.InvalidErr)
		return
	}
	row, err := s.UpdateRules(c.Request.Context(), &r, user.ID(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "保存成功")
}

// Members returns assigned, conflicting and unmatched scoped inventory.
// @Summary Fabric 成员归属
// @Tags Fabric
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/members [get]
// @Security Bearer
func (e Fabric) Members(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	r, ok := e.page(c)
	if !ok {
		return
	}
	rows, count, err := s.Members(c.Request.Context(), r, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(count), r.PageIndex, r.PageSize, "查询成功")
}

// SetMember sets manual membership or returns it to automatic policy.
// @Summary 调整 Fabric 归属
// @Tags Fabric
// @Accept json
// @Param deviceId path int true "设备 ID"
// @Param data body dto.MemberReq true "归属"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/members/{deviceId} [put]
// @Security Bearer
func (e Fabric) SetMember(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "deviceId")
	if !ok {
		return
	}
	var r dto.MemberReq
	if !decodeFabricRequest(c, &r) {
		e.respond(service.InvalidErr)
		return
	}
	row, err := s.UpdateMember(c.Request.Context(), id, &r, user.ID(c), actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "保存成功")
}

// Settings updates collection intervals; schedules start disabled.
// @Summary Fabric 采集设置
// @Tags Fabric
// @Accept json
// @Param id path int true "Fabric ID"
// @Param data body dto.SettingsReq true "设置和版本"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/{id}/settings [put]
// @Security Bearer
func (e Fabric) Settings(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	var r dto.SettingsReq
	if !decodeFabricRequest(c, &r) {
		e.respond(service.InvalidErr)
		return
	}
	row, err := s.Settings(c.Request.Context(), id, &r, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "保存成功")
}

// Endpoints performs scoped normalized IP/MAC/CIDR lookup.
// @Summary Fabric 终端定位
// @Tags Fabric
// @Param keyword query string true "IP/MAC/CIDR"
// @Param vrf query string false "VRF"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/endpoints [get]
// @Security Bearer
func (e Fabric) Endpoints(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	r, ok := e.page(c)
	if !ok {
		return
	}
	rows, count, err := s.Endpoints(c.Request.Context(), r, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(count), r.PageIndex, r.PageSize, "查询成功")
}

// Nodes lists scoped Fabric devices and dimension evidence.
// @Summary Fabric 设备
// @Tags Fabric
// @Param id path int true "Fabric ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/{id}/nodes [get]
// @Security Bearer
func (e Fabric) Nodes(c *gin.Context) { e.detailPage(c, "nodes") }

// Segments lists the all-Leaf service matrix.
// @Summary Fabric 网段矩阵
// @Tags Fabric
// @Param id path int true "Fabric ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/{id}/segments [get]
// @Security Bearer
func (e Fabric) Segments(c *gin.Context) { e.detailPage(c, "segments") }

// VPCGroups lists peer-confirmed pairs.
// @Summary Fabric vPC 组
// @Tags Fabric
// @Param id path int true "Fabric ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/{id}/vpc-groups [get]
// @Security Bearer
func (e Fabric) VPCGroups(c *gin.Context) { e.detailPage(c, "vpc") }

// Findings lists faults separately from missing evidence.
// @Summary Fabric 检查结果
// @Tags Fabric
// @Param id path int true "Fabric ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/{id}/findings [get]
// @Security Bearer
func (e Fabric) Findings(c *gin.Context) { e.detailPage(c, "findings") }

// Assessments lists persisted scoped analyses.
// @Summary Fabric 分析历史
// @Tags Fabric
// @Param id path int true "Fabric ID"
// @Success 200 {object} response.Response
// @Router /api/v1/network-fabrics/{id}/assessments [get]
// @Security Bearer
func (e Fabric) Assessments(c *gin.Context) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	r, ok := e.page(c)
	if !ok {
		return
	}
	rows, count, err := s.Assessments(c.Request.Context(), id, r, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(count), r.PageIndex, r.PageSize, "查询成功")
}

func prepareFabric(e *Fabric, c *gin.Context) (*service.Fabric, bool) {
	s := &service.Fabric{}
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
func (e Fabric) respond(err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = fmt.Errorf("%w: %w", service.NotFoundErr, err)
	}
	request.Fail(&e.API, err)
}

func (e Fabric) id(c *gin.Context, key string) (int, bool) {
	id, err := strconv.Atoi(c.Param(key))
	if err != nil || id <= 0 {
		e.respond(service.InvalidErr)
		return 0, false
	}
	return id, true
}

func (e Fabric) page(c *gin.Context) (*dto.PageReq, bool) {
	r := &dto.PageReq{}
	if c.ShouldBindQuery(r) != nil || r.Validate() != nil {
		e.respond(service.InvalidErr)
		return nil, false
	}
	return r, true
}

func (e Fabric) detailPage(c *gin.Context, kind string) {
	s, ok := prepareFabric(&e, c)
	if !ok {
		return
	}
	id, ok := e.id(c, "id")
	if !ok {
		return
	}
	r, ok := e.page(c)
	if !ok {
		return
	}
	d, err := s.Detail(c.Request.Context(), id, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	var rows any
	count := 0
	start, end := 0, 0
	bounds := func(n int) {
		count = n
		start = min((r.PageIndex-1)*r.PageSize, n)
		end = min(start+r.PageSize, n)
	}
	switch kind {
	case "nodes":
		bounds(len(d.Nodes))
		rows = d.Nodes[start:end]
	case "segments":
		bounds(len(d.Segments))
		rows = d.Segments[start:end]
	case "vpc":
		bounds(len(d.VPCGroups))
		rows = d.VPCGroups[start:end]
	case "findings":
		bounds(len(d.Findings))
		rows = d.Findings[start:end]
	}
	e.PageOK(rows, count, r.PageIndex, r.PageSize, "查询成功")
}
