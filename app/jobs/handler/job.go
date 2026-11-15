package handler

import (
	"context"
	"errors"
	"net/http"

	"gorm.io/gorm"

	"github.com/gatework/motor/common/request"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/api"

	"github.com/gatework/motor/app/jobs"
	"github.com/gatework/motor/app/jobs/service"
	"github.com/gatework/motor/common/actions"
)

// Job carries sys job state for this module.
type Job struct {
	api.API
}

// jobByID names the job in the path.
type jobByID struct {
	ID int `uri:"id" binding:"required,min=1"`
}

// Stop disables a scheduled job.
// @Summary 停止任务
// @Tags 定时任务
// @Param id path int true "任务编号"
// @Success 200 {object} response.Response
// @Router /api/v1/job/remove/{id} [post]
// @Security Bearer
func (e Job) Stop(c *gin.Context) { e.setState(c, (*service.Job).Stop) }

// Start enables a scheduled job.
// @Summary 启动任务
// @Tags 定时任务
// @Param id path int true "任务编号"
// @Success 200 {object} response.Response
// @Router /api/v1/job/start/{id} [post]
// @Security Bearer
func (e Job) Start(c *gin.Context) { e.setState(c, (*service.Job).Start) }

func (e Job) setState(c *gin.Context, change func(*service.Job, context.Context, int) error) {
	var err error
	e.API, err = request.New(c)
	if err != nil {
		e.Logger.Error(err)
		e.Error(http.StatusInternalServerError, nil, "数据库连接获取失败")
		return
	}
	var target jobByID
	if err := c.ShouldBindUri(&target); err != nil {
		e.Error(http.StatusUnprocessableEntity, nil, "参数验证失败")
		return
	}
	s := service.Job{
		Service:    e.Service(),
		Cron:       sdk.Runtime.Crontab(c.Request.Host),
		Tenant:     c.Request.Host,
		Permission: actions.GetPermissionFromContext(c),
	}
	err = change(&s, c.Request.Context(), target.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		e.Error(http.StatusNotFound, nil, "任务不存在或无权操作")
		return
	}
	if err != nil {
		e.Logger.Errorf("changing job %d state: %v", target.ID, err)
		e.Error(http.StatusInternalServerError, nil, "操作失败")
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
