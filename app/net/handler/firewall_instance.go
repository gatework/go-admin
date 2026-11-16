package handler

import (
	"github.com/gin-gonic/gin"

	_ "github.com/gatework/motor-core/response" // Swagger response annotations require this package.
	"github.com/gatework/motor/app/net/models/firewall/types"
	service "github.com/gatework/motor/app/net/service/firewall"
	"github.com/gatework/motor/app/net/service/firewall/dto"
)

// Domains handles an authorized firewall workbench operation.
// @Summary Firewall Domains
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/domains [get]
// @Security Bearer
func (e Firewall) Domains(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		return s.Domains(ctx)
	})
}

// SaveDomain handles an authorized firewall workbench operation.
// @Summary Firewall SaveDomain
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/domains [post]
// @Router /api/v1/network-firewall/domains/{id} [put]
// @Security Bearer
func (e Firewall) SaveDomain(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		var in dto.DomainInput
		if err := decodeFirewallRequest(c, &in); err != nil {
			return nil, err
		}
		return s.SaveDomain(ctx, i, in)
	})
}

// Instances handles an authorized firewall workbench operation.
// @Summary Firewall Instances
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/instances [get]
// @Security Bearer
func (e Firewall) Instances(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		p, err := page(c)
		if err != nil {
			return nil, err
		}
		rows, n, err := s.Instances(ctx, &p, permission(c))
		return pageResult{rows, n, p}, err
	})
}

// CreateInstance handles an authorized firewall workbench operation.
// @Summary Firewall CreateInstance
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/instances [post]
// @Security Bearer
func (e Firewall) CreateInstance(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		var in dto.InstanceInput
		if err := decodeFirewallRequest(c, &in); err != nil {
			return nil, err
		}
		return s.CreateInstance(ctx, in, permission(c))
	})
}

// Instance handles an authorized firewall workbench operation.
// @Summary Firewall Instance
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/instances/{id} [get]
// @Security Bearer
func (e Firewall) Instance(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		return s.Instance(ctx, i, permission(c))
	})
}

// Settings handles an authorized firewall workbench operation.
// @Summary Firewall Settings
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/instances/{id}/settings [get]
// @Security Bearer
func (e Firewall) Settings(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		return s.Settings(ctx, i, permission(c))
	})
}

// SaveSettings handles an authorized firewall workbench operation.
// @Summary Firewall SaveSettings
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/instances/{id}/settings [put]
// @Security Bearer
func (e Firewall) SaveSettings(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		var in types.Settings
		if err := decodeFirewallRequest(c, &in); err != nil {
			return nil, err
		}
		return s.SaveSettings(ctx, i, in, permission(c))
	})
}

// Coverage handles an authorized firewall workbench operation.
// @Summary Firewall Coverage
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/instances/{id}/coverage [get]
// @Security Bearer
func (e Firewall) Coverage(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		return s.Coverage(ctx, i, permission(c))
	})
}
