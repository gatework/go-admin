package handler

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gatework/motor/common/request"

	"github.com/gin-gonic/gin"

	"github.com/gatework/firemon/config"
	"github.com/gatework/firemon/snapshot"
	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger response annotations require this package.
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor-core/sdk/contract/actions"
	"github.com/gatework/motor/app/net/models/firewall/types"
	"github.com/gatework/motor/app/net/service/deviceaccess"
	service "github.com/gatework/motor/app/net/service/firewall"
	"github.com/gatework/motor/app/net/service/firewall/dto"
)

// Firewall adapts firewall HTTP routes. Its methods take a value receiver: gin
// registers each route once and serves it concurrently, so the request-scoped
// API assigned in run must live on a per-call copy, never on a shared pointer.
type Firewall struct {
	api.API
	Access deviceaccess.Config
}
type pageResult struct {
	List  any
	Count int64
	Page  dto.Page
}
type operation func(*service.Firewall) (any, error)

// Options returns firewall workbench capabilities.
// @Summary Firewall options
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/options [get]
// @Security Bearer
func (e Firewall) Options(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) { return s.Options(c.Request.Context()) })
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
		return s.Normalize(in)
	})
}
func id(c *gin.Context, key string) (int, error) {
	v, ok := optionalPathID(c, key)
	if !ok {
		return 0, service.InvalidErr
	}
	return v, nil
}
func decodeFirewallRequest(c *gin.Context, v any) error {
	limit := int64(16 << 20)
	if strings.HasSuffix(c.FullPath(), "/imports") {
		limit = 2*config.DefaultMaxBytes + 65536
	}
	if !decodeStrictJSON(c, v, limit) {
		return service.InvalidErr
	}
	return nil
}
func page(c *gin.Context) (dto.Page, error) {
	p := dto.Page{}
	if c.ShouldBindQuery(&p) != nil {
		return p, service.InvalidErr
	}
	return p, p.Validate()
}
func permission(c *gin.Context) *actions.DataPermission { return actions.GetPermissionFromContext(c) }

// Handlers is the compile-time method map used by the declared route contract.
func (e Firewall) Handlers() map[string]gin.HandlerFunc {
	return map[string]gin.HandlerFunc{"Devices": e.Devices, "Connection": e.Connection, "SetConnection": e.SetConnection, "EnqueueCollection": e.EnqueueCollection, "Collections": e.Collections, "Collection": e.Collection, "CancelCollection": e.CancelCollection, "Options": e.Options, "Normalize": e.Normalize, "Domains": e.Domains, "SaveDomain": e.SaveDomain, "Instances": e.Instances, "CreateInstance": e.CreateInstance, "Instance": e.Instance, "Settings": e.Settings, "SaveSettings": e.SaveSettings, "Coverage": e.Coverage, "EnqueueImport": e.EnqueueImport, "Imports": e.Imports, "Import": e.Import, "CancelImport": e.CancelImport, "Snapshots": e.Snapshots, "Objects": e.Objects, "Object": e.Object, "References": e.References, "Requests": e.Requests, "SaveRequest": e.SaveRequest, "Request": e.Request, "EnqueueAnalysis": e.EnqueueAnalysis, "Analyses": e.Analyses, "Analysis": e.Analysis, "CancelAnalysis": e.CancelAnalysis, "EnqueueDraft": e.EnqueueDraft, "Drafts": e.Drafts, "Draft": e.Draft, "EditDraft": e.EditDraft, "ReanalyzeDraft": e.ReanalyzeDraft}
}

// respond translates the two failures that arrive unclassified - a dependency's
// conflict and a missing row - into this module's codes, then lets the shared
// adapter answer by kind. Anything else unclassified is logged and reported
// as an internal error without its detail.
func (e Firewall) respond(err error) {
	if errors.Is(err, snapshot.ErrConflict) {
		err = fmt.Errorf("%w: %w", service.ConflictErr, err)
	}
	fail(&e.API, err, service.NotFoundErr)
}

func (e Firewall) run(c *gin.Context, fn operation) {
	s := &service.Firewall{Actor: user.ID(c), Access: e.Access}
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
