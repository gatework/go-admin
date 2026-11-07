package jobs

import (
	"fmt"
	log "github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"gorm.io/gorm"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg/cronjob"
)

var timeFormat = "2006-01-02 15:04:05"
var retryCount = 3

var jobList map[string]JobExec

//var lock sync.Mutex

type JobCore struct {
	InvokeTarget   string
	Name           string
	JobId          int
	EntryId        int
	CronExpression string
	Args           string
}

// HttpJob 任务类型 http
type HttpJob struct {
	JobCore
	allowed func() bool
}

type ExecJob struct {
	JobCore
	allowed func() bool
}

func (e *ExecJob) Run() {
	if e.allowed != nil && !e.allowed() {
		return
	}
	startTime := time.Now()
	var obj = jobList[e.InvokeTarget]
	if obj == nil {
		log.Warn("[Job] ExecJob Run job nil")
		return
	}
	err := CallExec(obj.(JobExec), e.Args)
	if err != nil {
		// 如果失败暂停一段时间重试
		fmt.Println(time.Now().Format(timeFormat), " [ERROR] mission failed! ", err)
	}
	// 结束时间
	endTime := time.Now()

	// 执行时间
	latencyTime := endTime.Sub(startTime)
	//TODO: 待完善部分
	//str := time.Now().Format(timeFormat) + " [INFO] JobCore " + string(e.EntryId) + "exec success , spend :" + latencyTime.String()
	//ws.SendAll(str)
	log.Infof("[Job] JobCore %s exec success , spend :%v", e.Name, latencyTime)
	return
}

// Run http 任务接口
func (h *HttpJob) Run() {
	if h.allowed != nil && !h.allowed() {
		return
	}

	startTime := time.Now()
	var count = 0
	var err error
	var str string
	/* 循环 */
LOOP:
	if count < retryCount {
		/* 跳过迭代 */
		str, err = pkg.Get(h.InvokeTarget)
		if err != nil {
			// 如果失败暂停一段时间重试
			log.Warnf("[Job] mission failed! %v", err)
			log.Warnf("[Job] Retry after the task fails %d seconds! %s \n", (count+1)*5, str)
			time.Sleep(time.Duration(count+1) * 5 * time.Second)
			count = count + 1
			goto LOOP
		}
	}
	// 结束时间
	endTime := time.Now()

	// 执行时间
	latencyTime := endTime.Sub(startTime)
	//TODO: 待完善部分

	log.Infof("[Job] JobCore %s exec success , spend :%v", h.Name, latencyTime)
	return
}

// Setup 初始化
// Setup gives every tenant a scheduler and a supervisor to decide whether
// this instance is the one that fills it.
//
// One owner id for the whole process, not one per tenant: the thing holding
// the leases is this process, and a log line naming it should name the same
// thing in every database it appears in.
func Setup(dbs map[string]*gorm.DB) {

	fmt.Println(time.Now().Format(timeFormat), " [INFO] JobCore Starting...")

	owner := newOwnerID()
	for k, db := range dbs {
		sdk.Runtime.SetCrontabByTenant(k, cronjob.NewWithSeconds())
		newSupervisor(k, db, owner).start()
	}
}

// AddJob 添加任务 AddJob(invokeTarget string, jobId int, jobName string, cronExpression string)
func AddJob(c *cron.Cron, job Job) (int, error) {
	if job == nil {
		fmt.Println("unknown")
		return 0, nil
	}
	return job.addJob(c)
}

func (h *HttpJob) addJob(c *cron.Cron) (int, error) {
	id, err := c.AddJob(h.CronExpression, h)
	if err != nil {
		fmt.Println(time.Now().Format(timeFormat), " [ERROR] JobCore AddJob error", err)
		return 0, err
	}
	EntryId := int(id)
	return EntryId, nil
}

func (e *ExecJob) addJob(c *cron.Cron) (int, error) {
	id, err := c.AddJob(e.CronExpression, e)
	if err != nil {
		fmt.Println(time.Now().Format(timeFormat), " [ERROR] JobCore AddJob error", err)
		return 0, err
	}
	EntryId := int(id)
	return EntryId, nil
}

// Remove 移除任务
func Remove(c *cron.Cron, entryID int) chan bool {
	ch := make(chan bool, 1)
	go func() {
		c.Remove(cron.EntryID(entryID))
		fmt.Println(time.Now().Format(timeFormat), " [INFO] JobCore Remove success ,info entryID :", entryID)
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
