package handler

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger response schemas are resolved from this import.
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/contract/actions"
	dto "github.com/gatework/motor/app/net/service/dto/wireless"
	service "github.com/gatework/motor/app/net/service/wireless"
	"github.com/gatework/motor/common/request"
)

// Wireless handles the wireless HTTP operations for each request.
type Wireless struct{ api.API }

// Controllers returns scoped wireless data.
// @Summary Wireless Controllers
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers [get]
// @Security Bearer
func (e Wireless) Controllers(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, _, ok := e.ids(c)
	if !ok {
		return
	}
	_ = id
	r := dto.PageReq{}
	if c.ShouldBindQuery(&r) != nil {
		e.respond(service.InvalidErr)
		return
	}
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	rows, n, err := s.Controllers(ctx, &r, p)
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(n), r.PageIndex, r.PageSize, "OK")
}

// Resources returns scoped wireless data.
// @Summary Wireless Resources
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/access-points [get]
// @Security Bearer
func (e Wireless) Resources(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, _, ok := e.ids(c)
	if !ok {
		return
	}
	_ = id
	r := dto.PageReq{}
	if c.ShouldBindQuery(&r) != nil {
		e.respond(service.InvalidErr)
		return
	}
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	rows, n, err := s.ResourcePage(ctx, id, c.GetString("wirelessKind"), &r, p)
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(n), r.PageIndex, r.PageSize, "OK")
}

// Clients returns scoped wireless data.
// @Summary Wireless Clients
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/clients [get]
// @Security Bearer
func (e Wireless) Clients(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, _, ok := e.ids(c)
	if !ok {
		return
	}
	_ = id
	r := dto.PageReq{}
	if c.ShouldBindQuery(&r) != nil {
		e.respond(service.InvalidErr)
		return
	}
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	rows, n, err := s.Clients(ctx, &r, p)
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(n), r.PageIndex, r.PageSize, "OK")
}
func decodeWirelessRequest(c *gin.Context, v any) bool {
	limit := int64(128 << 10)
	if strings.Contains(c.FullPath(), "/imports/") {
		limit = 64 << 20
	}
	return decodeStrictJSON(c, v, limit)
}

// Switches returns scoped wireless data.
// @Summary Wireless Switches
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/switches [get]
// @Security Bearer
func (e Wireless) Switches(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, _, ok := e.ids(c)
	if !ok {
		return
	}
	_ = id
	r := dto.PageReq{}
	if c.ShouldBindQuery(&r) != nil {
		e.respond(service.InvalidErr)
		return
	}
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	rows, n, err := s.Switches(ctx, &r, p)
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(n), r.PageIndex, r.PageSize, "OK")
}

// Collections returns scoped wireless data.
// @Summary Wireless Collections
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/collections [get]
// @Security Bearer
func (e Wireless) Collections(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, _, ok := e.ids(c)
	if !ok {
		return
	}
	_ = id
	r := dto.PageReq{}
	if c.ShouldBindQuery(&r) != nil {
		e.respond(service.InvalidErr)
		return
	}
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	rows, n, err := s.Collections(ctx, id, &r, p)
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(n), r.PageIndex, r.PageSize, "OK")
}

// Controller reads an authorized wireless resource.
// @Summary Wireless Controller
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId} [get]
// @Security Bearer
func (e Wireless) Controller(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, sub, ok := e.ids(c)
	if !ok {
		return
	}
	_, _ = id, sub
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	row, err := s.Controller(ctx, id, p)
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "OK")
}

// Analysis reads an authorized wireless resource.
// @Summary Wireless Analysis
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/analysis [get]
// @Security Bearer
func (e Wireless) Analysis(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, sub, ok := e.ids(c)
	if !ok {
		return
	}
	_, _ = id, sub
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	row, err := s.Analysis(ctx, id, p)
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "OK")
}

// APDetail reads an authorized wireless resource.
// @Summary Wireless APDetail
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/access-points/{id} [get]
// @Security Bearer
func (e Wireless) APDetail(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, sub, ok := e.ids(c)
	if !ok {
		return
	}
	_, _ = id, sub
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	row, err := s.APDetail(ctx, id, sub, p)
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "OK")
}

// Collection reads an authorized wireless resource.
// @Summary Wireless Collection
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/collections/{id} [get]
// @Security Bearer
func (e Wireless) Collection(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, sub, ok := e.ids(c)
	if !ok {
		return
	}
	_, _ = id, sub
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	row, err := s.Collection(ctx, id, sub, p)
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "OK")
}

// Groups reads an authorized wireless resource.
// @Summary Wireless Groups
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/groups [get]
// @Security Bearer
func (e Wireless) Groups(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, sub, ok := e.ids(c)
	if !ok {
		return
	}
	_, _ = id, sub
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	row, err := s.Groups(ctx, p)
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "OK")
}

// Collect validates scope and versions before mutation.
// @Summary Wireless Collect
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/collections [post]
// @Security Bearer
func (e Wireless) Collect(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, sub, ok := e.ids(c)
	if !ok {
		return
	}
	_, _ = id, sub
	var r dto.CollectionReq
	if !decodeWirelessRequest(c, &r) {
		e.respond(service.InvalidErr)
		return
	}
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	row, err := s.CreateCollection(ctx, id, r, user.ID(c), p)
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "OK")
}

// Management validates scope and versions before mutation.
// @Summary Wireless Management
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/access-points/{id}/management [put]
// @Security Bearer
func (e Wireless) Management(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, sub, ok := e.ids(c)
	if !ok {
		return
	}
	_, _ = id, sub
	var r dto.ManagementReq
	if !decodeWirelessRequest(c, &r) {
		e.respond(service.InvalidErr)
		return
	}
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	row, err := s.Management(ctx, id, sub, r, p)
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "OK")
}

// Associate validates scope and versions before mutation.
// @Summary Wireless Associate
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/access-points/{id}/neighbor [put]
// @Security Bearer
func (e Wireless) Associate(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, sub, ok := e.ids(c)
	if !ok {
		return
	}
	_, _ = id, sub
	var r dto.NeighborReq
	if !decodeWirelessRequest(c, &r) {
		e.respond(service.InvalidErr)
		return
	}
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	row, err := s.Associate(ctx, id, sub, r, p)
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "OK")
}

// PreviewImport validates scope and versions before mutation.
// @Summary Wireless PreviewImport
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/imports/preview [post]
// @Security Bearer
func (e Wireless) PreviewImport(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, sub, ok := e.ids(c)
	if !ok {
		return
	}
	_, _ = id, sub
	var r dto.ImportReq
	if !decodeWirelessRequest(c, &r) {
		e.respond(service.InvalidErr)
		return
	}
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	row, err := s.PreviewImport(ctx, r.Content, p)
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "OK")
}

// CommitImport applies a digest-bound CSV atomically.
// @Summary Wireless import
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/imports/commit [post]
// @Security Bearer
func (e Wireless) CommitImport(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	var r dto.ImportReq
	if !decodeWirelessRequest(c, &r) {
		e.respond(service.InvalidErr)
		return
	}
	if err := s.CommitImport(c.Request.Context(), r, actions.GetPermissionFromContext(c)); err != nil {
		e.respond(err)
		return
	}
	e.OK(nil, "OK")
}

// Export returns scoped UTF-8 CSV.
// @Summary Wireless export
// @Tags Wireless
// @Produce text/csv
// @Success 200 {string} string
// @Router /api/v1/network-wireless/controllers/{controllerId}/exports/{kind} [get]
// @Security Bearer
func (e Wireless) Export(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, _, ok := e.ids(c)
	if !ok {
		return
	}
	r := dto.PageReq{}
	if c.ShouldBindQuery(&r) != nil {
		e.respond(service.InvalidErr)
		return
	}
	body, err := s.Export(c.Request.Context(), id, strings.ReplaceAll(c.Param("kind"), "-", "_"), &r, actions.GetPermissionFromContext(c))
	if err != nil {
		e.respond(err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=wireless.csv")
	c.Data(200, "text/csv; charset=utf-8", []byte(body))
}

// AccessPoints provides the access points operation within the caller's authorized wireless scope.
func (e Wireless) AccessPoints(c *gin.Context) {
	c.Set("wirelessKind", "access_points")
	e.Resources(c)
}

// Radios provides the radios operation within the caller's authorized wireless scope.
func (e Wireless) Radios(c *gin.Context) { c.Set("wirelessKind", "radios"); e.Resources(c) }

// BSSes provides the b s ses operation within the caller's authorized wireless scope.
func (e Wireless) BSSes(c *gin.Context) { c.Set("wirelessKind", "bsses"); e.Resources(c) }

// ClientResources provides the client resources operation within the caller's authorized wireless scope.
func (e Wireless) ClientResources(c *gin.Context) { c.Set("wirelessKind", "clients"); e.Resources(c) }

// Client provides the client operation within the caller's authorized wireless scope.
func (e Wireless) Client(c *gin.Context) {
	s, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	id, sub, ok := e.ids(c)
	if !ok {
		return
	}
	_, _ = id, sub
	ctx, p := c.Request.Context(), actions.GetPermissionFromContext(c)
	row, err := s.Client(ctx, id, sub, p)
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(row, "OK")
}
func prepareWireless(e *Wireless, c *gin.Context) (*service.Wireless, bool) {
	s := &service.Wireless{Actor: user.ID(c)}
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
func (e Wireless) respond(err error) {
	code, msg := 500, "wireless.operation_failed"
	switch {
	case errors.Is(err, service.InvalidErr):
		code, msg = 400, service.InvalidErr.Error()
	case errors.Is(err, service.ConflictErr):
		code, msg = 409, service.ConflictErr.Error()
	case errors.Is(err, service.ScopeErr), errors.Is(err, gorm.ErrRecordNotFound):
		code, msg = 404, service.ScopeErr.Error()
	}
	e.Error(code, nil, msg)
}

func (e Wireless) ids(c *gin.Context) (int, int, bool) {
	id, sub := 0, 0
	for _, k := range []string{"controllerId", "id"} {
		if c.Param(k) == "" {
			continue
		}
		v, err := strconv.Atoi(c.Param(k))
		if err != nil || v <= 0 {
			e.respond(service.InvalidErr)
			return 0, 0, false
		}
		if k == "id" {
			sub = v
		} else {
			id = v
		}
	}
	return id, sub, true
}
