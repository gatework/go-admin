package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/gatework/motor/app/net/models/firewall/types"
	service "github.com/gatework/motor/app/net/service/firewall"
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
		var in service.DomainInput
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
		var in service.InstanceInput
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
		var in service.ImportInput
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
		var q service.ObjectQuery
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
		var in service.RequestInput
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
		var in service.JobInput
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
		var in service.DraftInput
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
		var in service.DraftEdit
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
		var in service.JobInput
		if err := decodeFirewallRequest(c, &in); err != nil {
			return nil, err
		}
		return s.ReanalyzeDraft(ctx, i, in, permission(c))
	})
}

// Handlers is the compile-time method map used by the declared route contract.
func (e Firewall) Handlers() map[string]gin.HandlerFunc {
	return map[string]gin.HandlerFunc{"Devices": e.Devices, "Connection": e.Connection, "SetConnection": e.SetConnection, "EnqueueCollection": e.EnqueueCollection, "Collections": e.Collections, "Collection": e.Collection, "CancelCollection": e.CancelCollection, "Options": e.Options, "Normalize": e.Normalize, "Domains": e.Domains, "SaveDomain": e.SaveDomain, "Instances": e.Instances, "CreateInstance": e.CreateInstance, "Instance": e.Instance, "Settings": e.Settings, "SaveSettings": e.SaveSettings, "Coverage": e.Coverage, "EnqueueImport": e.EnqueueImport, "Imports": e.Imports, "Import": e.Import, "CancelImport": e.CancelImport, "Snapshots": e.Snapshots, "Objects": e.Objects, "Object": e.Object, "References": e.References, "Requests": e.Requests, "SaveRequest": e.SaveRequest, "Request": e.Request, "EnqueueAnalysis": e.EnqueueAnalysis, "Analyses": e.Analyses, "Analysis": e.Analysis, "CancelAnalysis": e.CancelAnalysis, "EnqueueDraft": e.EnqueueDraft, "Drafts": e.Drafts, "Draft": e.Draft, "EditDraft": e.EditDraft, "ReanalyzeDraft": e.ReanalyzeDraft}
}
