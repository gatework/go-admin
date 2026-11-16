package handler

import (
	"github.com/gin-gonic/gin"

	_ "github.com/gatework/motor-core/response" // Swagger response annotations require this package.
	service "github.com/gatework/motor/app/net/service/firewall"
	"github.com/gatework/motor/app/net/service/firewall/dto"
)

// EnqueueImport handles an authorized firewall workbench operation.
// @Summary Firewall EnqueueImport
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/instances/{id}/imports [post]
// @Security Bearer
func (e Firewall) EnqueueImport(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		var in dto.ImportInput
		if err := decodeFirewallRequest(c, &in); err != nil {
			return nil, err
		}
		return s.EnqueueImport(ctx, i, in, permission(c))
	})
}

// Imports handles an authorized firewall workbench operation.
// @Summary Firewall Imports
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/instances/{id}/imports [get]
// @Security Bearer
func (e Firewall) Imports(c *gin.Context) {
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
		rows, n, err := s.Imports(ctx, i, &p, permission(c))
		return pageResult{rows, n, p}, err
	})
}

// Import handles an authorized firewall workbench operation.
// @Summary Firewall Import
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/imports/{id} [get]
// @Security Bearer
func (e Firewall) Import(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		return s.Import(ctx, i, permission(c))
	})
}

// CancelImport handles an authorized firewall workbench operation.
// @Summary Firewall CancelImport
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/imports/{id}/cancel [post]
// @Security Bearer
func (e Firewall) CancelImport(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		return s.CancelImport(ctx, i, permission(c))
	})
}

// Snapshots handles an authorized firewall workbench operation.
// @Summary Firewall Snapshots
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/instances/{id}/snapshots [get]
// @Security Bearer
func (e Firewall) Snapshots(c *gin.Context) {
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
		rows, n, err := s.Snapshots(ctx, i, &p, permission(c))
		return pageResult{rows, n, p}, err
	})
}

// Objects handles an authorized firewall workbench operation.
// @Summary Firewall Objects
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/snapshots/{id}/objects/{kind} [get]
// @Security Bearer
func (e Firewall) Objects(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		var q dto.ObjectQuery
		if c.ShouldBindQuery(&q) != nil {
			return nil, service.InvalidErr
		}
		out, err := s.Objects(ctx, i, c.Param("kind"), q, permission(c))
		return pageResult{out.List, out.Count, q.Page}, err
	})
}

// Object handles an authorized firewall workbench operation.
// @Summary Firewall Object
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/snapshots/{id}/objects/{kind}/{objectId} [get]
// @Security Bearer
func (e Firewall) Object(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		oid, err := id(c, "objectId")
		if err != nil {
			return nil, err
		}
		return s.Object(ctx, i, c.Param("kind"), oid, permission(c))
	})
}

// References handles an authorized firewall workbench operation.
// @Summary Firewall References
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/snapshots/{id}/objects/{kind}/{objectId}/references [get]
// @Security Bearer
func (e Firewall) References(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		ctx := c.Request.Context()
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		oid, err := id(c, "objectId")
		if err != nil {
			return nil, err
		}
		return s.References(ctx, i, c.Param("kind"), oid, permission(c))
	})
}
