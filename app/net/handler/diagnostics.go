package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/jwtauth/user"
	_ "github.com/gatework/motor-core/response" // Swagger response annotations require this package.
	"github.com/gatework/motor-core/sdk/api"
	service "github.com/gatework/motor/app/net/service/diagnostics"
)

// Diagnostics carries diagnostics state for this module.
type Diagnostics struct {
	*api.API
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
func (e *Diagnostics) Server(c *gin.Context) {
	if !e.prepare(c) {
		return
	}
	host := c.Request.Host
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	e.OK(e.Manager.Server(host), "OK")
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
func (e *Diagnostics) Start(c *gin.Context) {
	if !e.prepare(c) {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var request service.Request
	if err := decoder.Decode(&request); err != nil {
		e.respond(service.InvalidErr)
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		e.respond(service.InvalidErr)
		return
	}
	job, err := e.Manager.Start(c.Request.Context(), owner(c), request)
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(job, "OK")
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
func (e *Diagnostics) Get(c *gin.Context) {
	if !e.prepare(c) {
		return
	}
	job, err := e.Manager.Get(owner(c), c.Param("id"))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(job, "OK")
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
func (e *Diagnostics) Stop(c *gin.Context) {
	if !e.prepare(c) {
		return
	}
	job, err := e.Manager.Stop(owner(c), c.Param("id"))
	if err != nil {
		e.respond(err)
		return
	}
	e.OK(job, "OK")
}

func (e *Diagnostics) prepare(c *gin.Context) bool {
	var err error
	e.API, err = api.New(c, api.Dependencies{})
	if err != nil {
		e.respond(err)
		return false
	}
	c.Header("Cache-Control", "no-store")
	return true
}

func (e *Diagnostics) respond(err error) {
	code, message := 500, "Network diagnostic request failed"
	switch {
	case errors.Is(err, service.InvalidErr):
		code, message = 400, "Invalid target or diagnostic parameters"
	case errors.Is(err, service.BusyErr):
		code, message = 429, "A diagnostic is already running or the server is busy"
	case errors.Is(err, service.UnavailableErr):
		code, message = 503, "The selected diagnostic tool is unavailable on this server"
	case errors.Is(err, service.NotFoundErr):
		code, message = 404, "The diagnostic job was not found or has expired"
	}
	e.Error(code, nil, message)
}
