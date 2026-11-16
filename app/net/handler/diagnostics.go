package handler

import (
	"net"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger response annotations require this package.
	"github.com/gatework/motor-core/sdk/api"
	service "github.com/gatework/motor/app/net/service/diagnostics"
	"github.com/gatework/motor/common/request"
)

// Diagnostics adapts network diagnostic HTTP routes. It holds only the shared,
// concurrency-safe manager; the request-scoped API is created per call so that
// concurrent requests never share a gin context.
type Diagnostics struct {
	Manager *service.Manager
}

func owner(c *gin.Context) service.Owner {
	return service.Owner{Tenant: c.Request.Host, UserID: user.ID(c)}
}

// Server reports actual backend identity and executable availability.
// @Summary 网络检查服务器信息
// @Tags 网络检查
// @Produce json
// @Success 200 {object} response.Response
// @Router /api/v1/network-diagnostics/server [get]
// @Security Bearer
func (e Diagnostics) Server(c *gin.Context) {
	a, ok := e.prepare(c)
	if !ok {
		return
	}
	host := c.Request.Host
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	a.OK(e.Manager.Server(host), "OK")
}

// Start launches a bounded diagnostic on the backend server.
// @Summary 开始网络检查
// @Tags 网络检查
// @Accept json
// @Produce json
// @Param request body service.Request true "Diagnostic parameters"
// @Success 200 {object} response.Response
// @Failure 400,429,503 {object} response.Response
// @Router /api/v1/network-diagnostics/jobs [post]
// @Security Bearer
func (e Diagnostics) Start(c *gin.Context) {
	a, ok := e.prepare(c)
	if !ok {
		return
	}
	var req service.Request
	if !decodeStrictJSON(c, &req, 4096) {
		respondDiagnostics(a, service.InvalidErr)
		return
	}
	ctx, who := c.Request.Context(), owner(c)
	runs := service.Runs{DB: a.DB}
	if err := runs.Admit(ctx, who); err != nil {
		respondDiagnostics(a, err)
		return
	}
	job, err := e.Manager.Start(ctx, who, req)
	if err != nil {
		respondDiagnostics(a, err)
		return
	}
	// A run that cannot be audited does not run.
	if err := runs.Record(ctx, who, job); err != nil {
		_, _ = e.Manager.Stop(who, job.ID)
		respondDiagnostics(a, err)
		return
	}
	a.OK(job, "OK")
}

// Get returns output only for the initiating user and tenant.
// @Summary 查询网络检查输出
// @Tags 网络检查
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} response.Response
// @Failure 404 {object} response.Response
// @Router /api/v1/network-diagnostics/jobs/{id} [get]
// @Security Bearer
func (e Diagnostics) Get(c *gin.Context) {
	a, ok := e.prepare(c)
	if !ok {
		return
	}
	job, err := e.Manager.Get(owner(c), c.Param("id"))
	if err != nil {
		respondDiagnostics(a, err)
		return
	}
	a.OK(job, "OK")
}

// Stop cancels the owned job and preserves its captured output.
// @Summary 停止网络检查
// @Tags 网络检查
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} response.Response
// @Failure 404 {object} response.Response
// @Router /api/v1/network-diagnostics/jobs/{id} [delete]
// @Security Bearer
func (e Diagnostics) Stop(c *gin.Context) {
	a, ok := e.prepare(c)
	if !ok {
		return
	}
	job, err := e.Manager.Stop(owner(c), c.Param("id"))
	if err != nil {
		respondDiagnostics(a, err)
		return
	}
	a.OK(job, "OK")
}

func (e Diagnostics) prepare(c *gin.Context) (*api.API, bool) {
	a, err := request.New(c)
	if err != nil {
		request.Fail(&a, err)
		return nil, false
	}
	c.Header("Cache-Control", "no-store")
	return &a, true
}

func respondDiagnostics(a *api.API, err error) { request.Fail(a, err) }
