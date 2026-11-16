package handler

import (
	"github.com/gin-gonic/gin"

	_ "github.com/gatework/motor-core/response" // Swagger response annotations require this package.
	service "github.com/gatework/motor/app/net/service/firewall"
	"github.com/gatework/motor/app/net/service/firewall/dto"
)

// Requests handles an authorized firewall workbench operation.
// @Summary Firewall Requests
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/requests [get]
// @Security Bearer
func (e Firewall) Requests(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		p, err := page(c)
		if err != nil {
			return nil, err
		}
		rows, n, err := s.Requests(ctx, &p, permission(c))
		return pageResult{rows, n, p}, err
	})
}

// SaveRequest handles an authorized firewall workbench operation.
// @Summary Firewall SaveRequest
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/requests [post]
// @Router /api/v1/network-firewall/requests/{id} [put]
// @Security Bearer
func (e Firewall) SaveRequest(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		var in dto.RequestInput
		if err := decodeFirewallRequest(c, &in); err != nil {
			return nil, err
		}
		return s.SaveRequest(ctx, i, in, permission(c))
	})
}

// Request handles an authorized firewall workbench operation.
// @Summary Firewall Request
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/requests/{id} [get]
// @Security Bearer
func (e Firewall) Request(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		return s.Request(ctx, i, permission(c))
	})
}

// EnqueueAnalysis handles an authorized firewall workbench operation.
// @Summary Firewall EnqueueAnalysis
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/requests/{id}/analyses [post]
// @Security Bearer
func (e Firewall) EnqueueAnalysis(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		var in dto.JobInput
		if err := decodeFirewallRequest(c, &in); err != nil {
			return nil, err
		}
		return s.EnqueueAnalysis(ctx, i, in, permission(c))
	})
}

// Analyses handles an authorized firewall workbench operation.
// @Summary Firewall Analyses
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/requests/{id}/analyses [get]
// @Security Bearer
func (e Firewall) Analyses(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		p, err := page(c)
		if err != nil {
			return nil, err
		}
		rows, n, err := s.Analyses(ctx, i, &p, permission(c))
		return pageResult{rows, n, p}, err
	})
}

// Analysis handles an authorized firewall workbench operation.
// @Summary Firewall Analysis
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/analyses/{id} [get]
// @Security Bearer
func (e Firewall) Analysis(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		return s.Analysis(ctx, i, permission(c))
	})
}

// CancelAnalysis handles an authorized firewall workbench operation.
// @Summary Firewall CancelAnalysis
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/analyses/{id}/cancel [post]
// @Security Bearer
func (e Firewall) CancelAnalysis(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		return s.CancelAnalysis(ctx, i, permission(c))
	})
}

// EnqueueDraft handles an authorized firewall workbench operation.
// @Summary Firewall EnqueueDraft
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/drafts [post]
// @Security Bearer
func (e Firewall) EnqueueDraft(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		var in dto.DraftInput
		if err := decodeFirewallRequest(c, &in); err != nil {
			return nil, err
		}
		return s.EnqueueDraft(ctx, in, permission(c))
	})
}

// Drafts handles an authorized firewall workbench operation.
// @Summary Firewall Drafts
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/drafts [get]
// @Security Bearer
func (e Firewall) Drafts(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		p, err := page(c)
		if err != nil {
			return nil, err
		}
		rows, n, err := s.Drafts(ctx, &p, permission(c))
		return pageResult{rows, n, p}, err
	})
}

// Draft handles an authorized firewall workbench operation.
// @Summary Firewall Draft
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/drafts/{id} [get]
// @Security Bearer
func (e Firewall) Draft(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		return s.Draft(ctx, i, permission(c))
	})
}

// EditDraft handles an authorized firewall workbench operation.
// @Summary Firewall EditDraft
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/drafts/{id} [put]
// @Security Bearer
func (e Firewall) EditDraft(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		var in dto.DraftEdit
		if err := decodeFirewallRequest(c, &in); err != nil {
			return nil, err
		}
		return s.EditDraft(ctx, i, in, permission(c))
	})
}

// ReanalyzeDraft handles an authorized firewall workbench operation.
// @Summary Firewall ReanalyzeDraft
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/drafts/{id}/reanalyze [post]
// @Security Bearer
func (e Firewall) ReanalyzeDraft(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		var in dto.JobInput
		if err := decodeFirewallRequest(c, &in); err != nil {
			return nil, err
		}
		return s.ReanalyzeDraft(ctx, i, in, permission(c))
	})
}
