package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gatework/motor/common/request"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/gatework/firemon/config"
	"github.com/gatework/firemon/snapshot"
	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger response annotations require this package.
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/firewall/storage"
	"github.com/gatework/motor/app/net/models/firewall/types"
	service "github.com/gatework/motor/app/net/service/firewall"
	"github.com/gatework/motor/app/net/service/firewall/engine"
)

// Firewall adapts firewall HTTP routes. Its methods take a value receiver: gin
// registers each route once and serves it concurrently, so the request-scoped
// API assigned in run must live on a per-call copy, never on a shared pointer.
type Firewall struct{ api.API }
type pageResult struct {
	List  any
	Count int64
	Page  service.Page
}
type operation func(*service.Firewall) (any, error)

// Options returns firewall workbench capabilities.
// @Summary Firewall options
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/options [get]
// @Security Bearer
func (e Firewall) Options(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		domains, err := s.Domains(c.Request.Context())
		return gin.H{"domains": domains, "vendors": []string{"paloalto", "srx", "huawei", "hillstone", "fortinet", "netscreen", "asa"}, "limits": gin.H{"maxRows": engine.MaxRows, "maxDesignRows": engine.MaxDesignRows, "maxFileBytes": config.DefaultMaxBytes, "maxCollectionTargets": service.MaxCollectionTargets, "collectionConcurrency": service.CollectionConcurrency}, "defaultProtocol": "tcp"}, err
	})
}

// Normalize previews structured input without modifying a saved request.
// @Summary Normalize firewall request
// @Tags Firewall
// @Param body body types.Input true "Input"
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/normalize [post]
// @Security Bearer
func (e Firewall) Normalize(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		var in types.Input
		if err := decodeFirewallRequest(c, &in); err != nil {
			return nil, err
		}
		return engine.Normalize(in)
	})
}
func id(c *gin.Context, key string) (int, error) {
	value := c.Param(key)
	if value == "" {
		return 0, nil
	}
	v, e := strconv.Atoi(value)
	if e != nil || v <= 0 {
		return 0, service.InvalidErr
	}
	return v, nil
}
func decodeFirewallRequest(c *gin.Context, v any) error {
	limit := int64(16 << 20)
	if strings.HasSuffix(c.FullPath(), "/imports") {
		limit = 2*config.DefaultMaxBytes + 65536
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return service.InvalidErr
	}
	if d.Decode(new(any)) != io.EOF {
		return service.InvalidErr
	}
	return nil
}
func page(c *gin.Context) (service.Page, error) {
	p := service.Page{}
	if c.ShouldBindQuery(&p) != nil {
		return p, service.InvalidErr
	}
	return p, p.Validate()
}
func permission(c *gin.Context) *actions.DataPermission { return actions.GetPermissionFromContext(c) }

func (e Firewall) respond(err error) {
	code, msg := 500, "firewall.operation_failed"
	switch {
	case errors.Is(err, service.InvalidErr), errors.Is(err, engine.InputErr), errors.Is(err, engine.BudgetErr), errors.Is(err, storage.InvalidErr):
		code, msg = 400, "firewall.invalid_input"
	case errors.Is(err, service.NotFoundErr), errors.Is(err, gorm.ErrRecordNotFound):
		code, msg = 404, "firewall.not_found"
	case errors.Is(err, service.ConflictErr), errors.Is(err, storage.VersionErr), errors.Is(err, snapshot.ErrConflict):
		code, msg = 409, "firewall.version_conflict"
	case errors.Is(err, service.UnavailableErr):
		code, msg = 503, "firewall.evidence_unavailable"
	}
	e.Error(code, nil, msg)
}

func (e Firewall) run(c *gin.Context, fn operation) {
	s := &service.Firewall{Actor: user.ID(c)}
	var err error
	e.API, err = request.New(c)
	s.Service = e.Service()
	if err != nil {
		e.respond(err)
		return
	}
	c.Header("Cache-Control", "no-store")
	if s.Actor <= 0 {
		e.Error(403, nil, "firewall.not_found")
		return
	}
	out, err := fn(s)
	if err != nil {
		e.respond(err)
		return
	}
	if page, ok := out.(pageResult); ok {
		e.PageOK(page.List, int(page.Count), page.Page.PageIndex, page.Page.PageSize, "OK")
		return
	}
	e.OK(out, "OK")
}
