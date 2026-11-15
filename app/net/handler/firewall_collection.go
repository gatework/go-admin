package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor/app/net/models/connection"
	ndto "github.com/gatework/motor/app/net/service/dto/inventory"
	service "github.com/gatework/motor/app/net/service/firewall"
)

// Devices lists scoped firewall inventory with non-secret connection readiness.
// @Summary Firewall inventory
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/devices [get]
// @Security Bearer
func (e *Firewall) Devices(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		var p ndto.DevicePageReq
		if err := c.ShouldBindQuery(&p); err != nil {
			return nil, service.InvalidErr
		}
		if err := p.Validate(); err != nil {
			return nil, service.InvalidErr
		}
		rows, count, err := s.Devices(c.Request.Context(), &p, permission(c))
		return pageResult{rows, count, service.Page{PageIndex: p.GetPageIndex(), PageSize: p.GetPageSize()}}, err
	})
}

// Connection returns server-side credential readiness, never credentials.
// @Summary Firewall connection options
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/devices/{id}/connection [get]
// @Security Bearer
func (e *Firewall) Connection(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		return s.Connection(c.Request.Context(), i, permission(c))
	})
}

// SetConnection updates versioned transport options shared with device operations.
// @Summary Update firewall connection options
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/devices/{id}/connection [put]
// @Security Bearer
func (e *Firewall) SetConnection(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		var in connection.Input
		if err = decodeFirewallRequest(c, &in); err != nil {
			return nil, err
		}
		return s.SetConnection(c.Request.Context(), i, in, permission(c))
	})
}

// EnqueueCollection creates an idempotent manual read-only batch.
// @Summary Start firewall login test or capture
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/collection-batches [post]
// @Security Bearer
func (e *Firewall) EnqueueCollection(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		var in service.CollectionInput
		if err := decodeFirewallRequest(c, &in); err != nil {
			return nil, err
		}
		return s.EnqueueCollection(c.Request.Context(), in, permission(c))
	})
}

// Collections returns actor-owned, inventory-scoped batches.
// @Summary Firewall collection history
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/collection-batches [get]
// @Security Bearer
func (e *Firewall) Collections(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		p, err := page(c)
		if err != nil {
			return nil, err
		}
		rows, count, err := s.Collections(c.Request.Context(), &p, permission(c))
		return pageResult{rows, count, p}, err
	})
}

// Collection returns durable per-device progress.
// @Summary Firewall collection progress
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/collection-batches/{id} [get]
// @Security Bearer
func (e *Firewall) Collection(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		return s.Collection(c.Request.Context(), i, permission(c))
	})
}

// CancelCollection cancels pending targets and signals active readers to close.
// @Summary Cancel firewall collection
// @Tags Firewall
// @Success 200 {object} response.Response
// @Router /api/v1/network-firewall/collection-batches/{id}/cancel [post]
// @Security Bearer
func (e *Firewall) CancelCollection(c *gin.Context) {
	e.run(c, func(s *service.Firewall) (any, error) {
		i, err := id(c, "id")
		if err != nil {
			return nil, err
		}
		return s.CancelCollection(c.Request.Context(), i, permission(c))
	})
}
