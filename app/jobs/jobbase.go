package jobs

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	log "github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"

	"github.com/robfig/cron/v3"

	"github.com/gatework/motor-core/sdk/pkg"
	"github.com/gatework/motor-core/sdk/pkg/cronjob"
)

const retryCount = 3

var jobList map[string]JobExec

// JobCore carries job core state for this module.
type JobCore struct {
	InvokeTarget   string
	Name           string
	JobID          int
	EntryID        int
	CronExpression string
	Args           string
}

// HTTPJob 任务类型 http
type HTTPJob struct {
	JobCore
	allowed   func() bool
	execution context.Context //nolint:containedctx // Scheduler-owned lifecycle context; request contexts are never retained.
}

// ExecJob carries exec job state for this module.
type ExecJob struct {
	JobCore
	allowed func() bool
}

// Run provides run for this module.
func (e *ExecJob) Run() {
	if e.allowed != nil && !e.allowed() {
		return
	}
	logger := log.NewHelper(log.Default())
	started := time.Now()
	job := jobList[e.InvokeTarget]
	if job == nil {
		logger.Warn("job: execution target is not registered")
		return
	}
	if err := CallExec(job, e.Args); err != nil {
		logger.Errorf("job %s: %v", e.Name, err)
		return
	}
	logger.Infof("job %s completed in %v", e.Name, time.Since(started))
}

// Run satisfies cron.Job. Its execution lifetime belongs to the scheduler,
// independently of requests that create or remove this registration.
func (h *HTTPJob) Run() {
	if h.allowed != nil && !h.allowed() {
		return
	}
	ctx := h.execution
	if ctx == nil {
		ctx = context.Background()
	}
	started := time.Now()
	err := runHTTPJob(
		ctx,
		h.InvokeTarget,
		pkg.Get,
		func(attempt int) time.Duration { return time.Duration(attempt) * 5 * time.Second },
	)
	if err != nil {
		log.NewHelper(log.Default()).Errorf("job %s failed: %v", h.Name, err)
		return
	}
	log.NewHelper(log.Default()).Infof("job %s completed in %v", h.Name, time.Since(started))
}

func runHTTPJob(
	ctx context.Context,
	target string,
	fetch func(context.Context, string) (string, error),
	delay func(int) time.Duration,
) error {
	var last error
	for attempt := 1; attempt <= retryCount; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := fetch(ctx, target)
		if err == nil {
			return nil
		}
		last = err
		if attempt == retryCount {
			break
		}
		timer := time.NewTimer(delay(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("http job exhausted %d attempts: %w", retryCount, last)
}

// Setup 初始化
// Setup gives every tenant a scheduler and a supervisor to decide whether
// this instance is the one that fills it.
//
// One owner id for the whole process, not one per tenant: the thing holding
// the leases is this process, and a log line naming it should name the same
// thing in every database it appears in.
func Setup(dbs map[string]*gorm.DB) {

	log.NewHelper(log.Default()).Info("starting tenant schedulers")

	owner := newOwnerID()
	for k, db := range dbs {
		sdk.Runtime.SetCrontab(k, cronjob.NewWithSeconds())
		newSupervisor(k, db, owner).start()
	}
}

// AddJob 添加任务 AddJob(invokeTarget string, jobId int, jobName string, cronExpression string)
func AddJob(c *cron.Cron, job Job) (int, error) {
	if job == nil {
		log.NewHelper(log.Default()).Warn("job implementation is missing")
		return 0, nil
	}
	return job.addJob(c)
}

func (h *HTTPJob) addJob(c *cron.Cron) (int, error) {
	id, err := c.AddJob(h.CronExpression, h)
	if err != nil {

		return 0, err
	}
	return int(id), nil
}

func (e *ExecJob) addJob(c *cron.Cron) (int, error) {
	id, err := c.AddJob(e.CronExpression, e)
	if err != nil {

		return 0, err
	}
	return int(id), nil
}

// Remove 移除任务
func Remove(c *cron.Cron, entryID int) chan bool {
	ch := make(chan bool, 1)
	go func() {
		c.Remove(cron.EntryID(entryID))
		log.NewHelper(log.Default()).Infof("removed scheduler entry %d", entryID)
		ch <- true
	}()
	return ch
}

// 任务停止
//func Stop() chan bool {
//	ch := make(chan bool)
//	go func() {
//		global.GADMCron.Stop()
//		ch <- true
//	}()
//	return ch
//}
