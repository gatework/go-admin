package handler

import (
	"net/http"

	"github.com/gatework/motor/common/request"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/api"

	"github.com/gatework/motor/app/jobs"
	"github.com/gatework/motor/app/jobs/service"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/dto"
)

// Job carries sys job state for this module.
type Job struct {
	api.API
}

// Stop 调用service实现
func (e Job) Stop(c *gin.Context) {
	v := dto.DeleteRequest{}
	s := service.SysJob{}
	var err error
	e.API, err = request.New(c)
	if err == nil {
		err = e.Bind(&v)
	}
	s.Service = e.Service()
	if err != nil {
		e.Logger.Error(err)
		e.Error(500, err, err.Error())
		return
	}

	s.Cron = sdk.Runtime.Crontab(c.Request.Host)
	s.Tenant = c.Request.Host
	s.Permission = actions.GetPermissionFromContext(c)
	s.DB = s.DB.WithContext(c.Request.Context())
	err = s.RemoveJob(&v)
	if err != nil {
		e.Logger.Errorf("RemoveJob error, %s", err.Error())
		e.Error(500, err, "")
		return
	}
	e.OK(nil, "")
}

// Start 启动job service实现
func (e Job) Start(c *gin.Context) {
	var err error
	e.API, err = request.New(c)
	log := e.Logger
	db := e.DB
	if err != nil {
		log.Error(err)
		e.Error(500, err, err.Error())
		return
	}
	var v dto.GetRequest
	err = c.BindUri(&v)
	if err != nil {
		log.Warnf("参数验证错误, error: %s", err)
		e.Error(http.StatusUnprocessableEntity, err, "参数验证失败")
		return
	}
	s := service.SysJob{
		DB:         db,
		Logger:     log,
		Cron:       sdk.Runtime.Crontab(c.Request.Host),
		Tenant:     c.Request.Host,
		Permission: actions.GetPermissionFromContext(c)}
	s.DB = s.DB.WithContext(c.Request.Context())
	err = s.StartJob(&v)
	if err != nil {
		log.Errorf("GetCrontabKey error, %s", err.Error())
		e.Error(500, err, err.Error())
		return
	}
	e.OK(nil, "")
}

// ReconcileWrites observes committed state after generic CRUD. The supervisor
// also polls it, so a request routed to a standby is applied by the leader.
func ReconcileWrites(c *gin.Context) {
	c.Next()
	if err := jobs.Reconcile(c.Request.Context(), c.Request.Host); err != nil {
		api.GetRequestLogger(c).Errorf("[Job] reconciling committed job changes: %v", err)
	}
}
