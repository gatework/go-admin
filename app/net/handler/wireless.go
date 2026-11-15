package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

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

// wirelessCall is what every wireless operation needs from its request: the
// scoped service, the caller's data permission, and the controller and item
// IDs from the path (zero when the route has none).
type wirelessCall struct {
	s          *service.Wireless
	p          *actions.DataPermission
	controller int
	item       int
	c          *gin.Context
}

func (w wirelessCall) ctx() context.Context { return w.c.Request.Context() }

// withWirelessBody decodes the request body into T before running op.
func withWirelessBody[T any](op func(wirelessCall, T) (any, error)) func(wirelessCall) (any, error) {
	return func(call wirelessCall) (any, error) {
		var r T
		if !decodeWirelessRequest(call.c, &r) {
			return nil, service.InvalidErr
		}
		return op(call, r)
	}
}

func decodeWirelessRequest(c *gin.Context, v any) bool {
	limit := int64(128 << 10)
	if strings.Contains(c.FullPath(), "/imports/") {
		limit = 64 << 20
	}
	return decodeStrictJSON(c, v, limit)
}

// Controllers returns scoped wireless data.
// @Summary Wireless Controllers
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers [get]
// @Security Bearer
func (e Wireless) Controllers(c *gin.Context) {
	e.servePage(c, func(w wirelessCall, r *dto.PageReq) (any, int64, error) { return w.s.Controllers(w.ctx(), r, w.p) })
}

// Clients returns scoped wireless data.
// @Summary Wireless Clients
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/clients [get]
// @Security Bearer
func (e Wireless) Clients(c *gin.Context) {
	e.servePage(c, func(w wirelessCall, r *dto.PageReq) (any, int64, error) { return w.s.Clients(w.ctx(), r, w.p) })
}

// Switches returns scoped wireless data.
// @Summary Wireless Switches
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/switches [get]
// @Security Bearer
func (e Wireless) Switches(c *gin.Context) {
	e.servePage(c, func(w wirelessCall, r *dto.PageReq) (any, int64, error) { return w.s.Switches(w.ctx(), r, w.p) })
}

// Collections returns scoped wireless data.
// @Summary Wireless Collections
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/collections [get]
// @Security Bearer
func (e Wireless) Collections(c *gin.Context) {
	e.servePage(c, func(w wirelessCall, r *dto.PageReq) (any, int64, error) {
		return w.s.Collections(w.ctx(), w.controller, r, w.p)
	})
}

// AccessPoints lists a controller's access points.
// @Summary Wireless access points
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/access-points [get]
// @Security Bearer
func (e Wireless) AccessPoints(c *gin.Context) { e.resources(c, "access_points") }

// Radios lists a controller's radios.
// @Summary Wireless radios
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/radios [get]
// @Security Bearer
func (e Wireless) Radios(c *gin.Context) { e.resources(c, "radios") }

// BSSes lists a controller's BSSes.
// @Summary Wireless BSSes
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/bsses [get]
// @Security Bearer
func (e Wireless) BSSes(c *gin.Context) { e.resources(c, "bsses") }

// ClientResources lists a controller's clients.
// @Summary Wireless controller clients
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/clients [get]
// @Security Bearer
func (e Wireless) ClientResources(c *gin.Context) { e.resources(c, "clients") }

// Controller reads an authorized wireless resource.
// @Summary Wireless Controller
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId} [get]
// @Security Bearer
func (e Wireless) Controller(c *gin.Context) {
	e.serve(c, func(w wirelessCall) (any, error) { return w.s.Controller(w.ctx(), w.controller, w.p) })
}

// Analysis reads an authorized wireless resource.
// @Summary Wireless Analysis
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/analysis [get]
// @Security Bearer
func (e Wireless) Analysis(c *gin.Context) {
	e.serve(c, func(w wirelessCall) (any, error) { return w.s.Analysis(w.ctx(), w.controller, w.p) })
}

// APDetail reads an authorized wireless resource.
// @Summary Wireless APDetail
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/access-points/{id} [get]
// @Security Bearer
func (e Wireless) APDetail(c *gin.Context) {
	e.serve(c, func(w wirelessCall) (any, error) { return w.s.APDetail(w.ctx(), w.controller, w.item, w.p) })
}

// Client reads one of a controller's clients.
// @Summary Wireless client
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/clients/{id} [get]
// @Security Bearer
func (e Wireless) Client(c *gin.Context) {
	e.serve(c, func(w wirelessCall) (any, error) { return w.s.Client(w.ctx(), w.controller, w.item, w.p) })
}

// Collection reads an authorized wireless resource.
// @Summary Wireless Collection
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/collections/{id} [get]
// @Security Bearer
func (e Wireless) Collection(c *gin.Context) {
	e.serve(c, func(w wirelessCall) (any, error) { return w.s.Collection(w.ctx(), w.controller, w.item, w.p) })
}

// Groups reads an authorized wireless resource.
// @Summary Wireless Groups
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/groups [get]
// @Security Bearer
func (e Wireless) Groups(c *gin.Context) {
	e.serve(c, func(w wirelessCall) (any, error) { return w.s.Groups(w.ctx(), w.p) })
}

// Collect validates scope and versions before mutation.
// @Summary Wireless Collect
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/collections [post]
// @Security Bearer
func (e Wireless) Collect(c *gin.Context) {
	e.serve(c, withWirelessBody(func(w wirelessCall, r dto.CollectionReq) (any, error) {
		return w.s.CreateCollection(w.ctx(), w.controller, r, user.ID(w.c), w.p)
	}))
}

// Management validates scope and versions before mutation.
// @Summary Wireless Management
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/access-points/{id}/management [put]
// @Security Bearer
func (e Wireless) Management(c *gin.Context) {
	e.serve(c, withWirelessBody(func(w wirelessCall, r dto.ManagementReq) (any, error) {
		return w.s.Management(w.ctx(), w.controller, w.item, r, w.p)
	}))
}

// Associate validates scope and versions before mutation.
// @Summary Wireless Associate
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/controllers/{controllerId}/access-points/{id}/neighbor [put]
// @Security Bearer
func (e Wireless) Associate(c *gin.Context) {
	e.serve(c, withWirelessBody(func(w wirelessCall, r dto.NeighborReq) (any, error) {
		return w.s.Associate(w.ctx(), w.controller, w.item, r, w.p)
	}))
}

// PreviewImport validates scope and versions before mutation.
// @Summary Wireless PreviewImport
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/imports/preview [post]
// @Security Bearer
func (e Wireless) PreviewImport(c *gin.Context) {
	e.serve(c, withWirelessBody(func(w wirelessCall, r dto.ImportReq) (any, error) {
		return w.s.PreviewImport(w.ctx(), r.Content, w.p)
	}))
}

// CommitImport applies a digest-bound CSV atomically.
// @Summary Wireless import
// @Tags Wireless
// @Success 200 {object} response.Response
// @Router /api/v1/network-wireless/imports/commit [post]
// @Security Bearer
func (e Wireless) CommitImport(c *gin.Context) {
	e.serve(c, withWirelessBody(func(w wirelessCall, r dto.ImportReq) (any, error) {
		return nil, w.s.CommitImport(w.ctx(), r, w.p)
	}))
}

// Export returns scoped UTF-8 CSV.
// @Summary Wireless export
// @Tags Wireless
// @Produce text/csv
// @Success 200 {string} string
// @Router /api/v1/network-wireless/controllers/{controllerId}/exports/{kind} [get]
// @Security Bearer
func (e Wireless) Export(c *gin.Context) {
	w, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	r := dto.PageReq{}
	if c.ShouldBindQuery(&r) != nil {
		e.respond(service.InvalidErr)
		return
	}
	kind := strings.ReplaceAll(c.Param("kind"), "-", "_")
	body, err := w.s.Export(w.ctx(), w.controller, kind, &r, w.p)
	if err != nil {
		e.respond(err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=wireless.csv")
	c.Data(http.StatusOK, "text/csv; charset=utf-8", []byte(body))
}

// serve prepares the call, runs op and answers its result.
func (e Wireless) serve(c *gin.Context, op func(wirelessCall) (any, error)) {
	call, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	out, err := op(call)
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(out, "OK")
}

// servePage binds the page query and answers op's rows as a page.
func (e Wireless) servePage(c *gin.Context, op func(wirelessCall, *dto.PageReq) (any, int64, error)) {
	call, ok := prepareWireless(&e, c)
	if !ok {
		return
	}
	r := dto.PageReq{}
	if c.ShouldBindQuery(&r) != nil {
		e.respond(service.InvalidErr)
		return
	}
	rows, n, err := op(call, &r)
	if err != nil {
		e.respond(err)
		return
	}
	e.PageOK(rows, int(n), r.PageIndex, r.PageSize, "OK")
}

func (e Wireless) resources(c *gin.Context, kind string) {
	e.servePage(c, func(w wirelessCall, r *dto.PageReq) (any, int64, error) {
		return w.s.ResourcePage(w.ctx(), w.controller, kind, r, w.p)
	})
}

// prepareWireless binds the request to e and builds the call: the scoped service and
// the positive controller and item IDs the route carries.
func prepareWireless(e *Wireless, c *gin.Context) (wirelessCall, bool) {
	var err error
	e.API, err = request.New(c)
	if err != nil {
		e.respond(err)
		return wirelessCall{}, false
	}
	c.Header("Cache-Control", "no-store")
	call := wirelessCall{
		s: &service.Wireless{Service: e.Service(), Actor: user.ID(c)},
		p: actions.GetPermissionFromContext(c),
		c: c,
	}
	for key, dst := range map[string]*int{"controllerId": &call.controller, "id": &call.item} {
		v, ok := optionalPathID(c, key)
		if !ok {
			e.respond(service.InvalidErr)
			return wirelessCall{}, false
		}
		*dst = v
	}
	return call, true
}

// respond answers err by its kind. A missing row reads as ScopeErr: whether
// it does not exist or is out of scope is not the caller's to learn.
func (e Wireless) respond(err error) { fail(&e.API, err, service.ScopeErr) }
