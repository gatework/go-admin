package dto

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-admin-team/go-admin-core/v2/sdk/api"
	"github.com/robfig/cron/v3"

	"go-admin/app/jobs/models"
	"go-admin/common/dto"
)

type SysJobSearch struct {
	dto.Pagination `search:"-"`
	JobId          int    `form:"jobId" search:"type:exact;column:job_id;table:sys_job"`
	JobName        string `form:"jobName" search:"type:icontains;column:job_name;table:sys_job"`
	JobGroup       string `form:"jobGroup" search:"type:exact;column:job_group;table:sys_job"`
	CronExpression string `form:"cronExpression" search:"type:exact;column:cron_expression;table:sys_job"`
	InvokeTarget   string `form:"invokeTarget" search:"type:exact;column:invoke_target;table:sys_job"`
	Status         int    `form:"status" search:"type:exact;column:status;table:sys_job"`
}

func (m *SysJobSearch) GetNeedSearch() interface{} {
	return *m
}

func (m *SysJobSearch) Bind(ctx *gin.Context) error {
	log := api.GetRequestLogger(ctx)
	err := ctx.ShouldBind(m)
	if err != nil {
		log.Errorf("Bind error: %s", err)
	}
	return err
}

type SysJobControl struct {
	present        map[string]json.RawMessage
	JobId          int    `json:"jobId" uri:"id"`
	JobName        string `json:"jobName"`        // 名称
	JobGroup       string `json:"jobGroup"`       // 任务分组
	JobType        int    `json:"jobType"`        // 任务类型
	CronExpression string `json:"cronExpression"` // cron表达式
	InvokeTarget   string `json:"invokeTarget"`   // 调用目标
	Args           string `json:"args"`           // 目标参数
	MisfirePolicy  int    `json:"misfirePolicy"`  // 执行策略
	Concurrent     int    `json:"concurrent"`     // 是否并发
	Status         int    `json:"status"`         // 状态
	EntryId        int    `json:"entryId"`        // job启动时返回的id
}

func (s *SysJobControl) Bind(ctx *gin.Context) error {
	if err := ctx.ShouldBind(s); err != nil {
		return err
	}
	if s.present == nil {
		// Gin's legacy form binding accepts Go field names. Also accept the
		// JSON names and normalize presence so both formats update the same fields.
		values := make(map[string][]string)
		for key, value := range ctx.Request.Form {
			values[strings.ToLower(key)] = value
		}
		if ctx.Request.MultipartForm != nil {
			for key, value := range ctx.Request.MultipartForm.Value {
				values[strings.ToLower(key)] = value
			}
		}
		s.present = make(map[string]json.RawMessage, len(values))
		form := make(map[string][]string, len(values))
		for _, name := range []string{"jobId", "jobName", "jobGroup", "jobType", "cronExpression", "invokeTarget", "args", "misfirePolicy", "concurrent", "status", "entryId"} {
			if value, ok := values[strings.ToLower(name)]; ok {
				form[name] = value
				s.present[strings.ToLower(name)] = nil
			}
		}
		if err := binding.MapFormWithTag(s, form, "json"); err != nil {
			return err
		}
	}
	if ctx.Param("id") != "" {
		if err := ctx.ShouldBindUri(s); err != nil {
			return err
		}
	}
	if ctx.Request.Method != http.MethodPost && s.JobId <= 0 {
		return errors.New("jobId must be positive")
	}
	if _, ok := s.present["jobname"]; ok || ctx.Request.Method == http.MethodPost {
		if s.JobName == "" {
			return errors.New("jobName is required")
		}
	}
	if _, ok := s.present["cronexpression"]; ok || ctx.Request.Method == http.MethodPost {
		parser := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.DowOptional | cron.Descriptor)
		if _, err := parser.Parse(s.CronExpression); err != nil {
			return err
		}
	}
	if _, ok := s.present["jobtype"]; ok || ctx.Request.Method == http.MethodPost {
		if s.JobType != 1 && s.JobType != 2 {
			return errors.New("jobType must be 1 or 2")
		}
	}
	if _, ok := s.present["status"]; ok || ctx.Request.Method == http.MethodPost {
		if s.Status != 1 && s.Status != 2 {
			return errors.New("status must be 1 or 2")
		}
	}
	return nil
}

// UnmarshalJSON records supplied fields independently from their zero values.
func (s *SysJobControl) UnmarshalJSON(data []byte) error {
	type plain SysJobControl
	if err := json.Unmarshal(data, (*plain)(s)); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	s.present = make(map[string]json.RawMessage, len(fields))
	for key, value := range fields {
		s.present[strings.ToLower(key)] = value
	}
	return nil
}

// UpdateFields excludes process-local entry IDs and all server-owned metadata.
func (s *SysJobControl) UpdateFields() []string {
	names := map[string]string{"jobName": "JobName", "jobGroup": "JobGroup", "jobType": "JobType", "cronExpression": "CronExpression", "invokeTarget": "InvokeTarget", "args": "Args", "misfirePolicy": "MisfirePolicy", "concurrent": "Concurrent", "status": "Status"}
	fields := make([]string, 0, len(s.present))
	for name, field := range names {
		if _, ok := s.present[strings.ToLower(name)]; ok {
			fields = append(fields, field)
		}
	}
	return fields
}

func (s *SysJobControl) ToModel() (*models.SysJob, error) {
	return &models.SysJob{
		JobId:          s.JobId,
		JobName:        s.JobName,
		JobGroup:       s.JobGroup,
		JobType:        s.JobType,
		CronExpression: s.CronExpression,
		InvokeTarget:   s.InvokeTarget,
		Args:           s.Args,
		MisfirePolicy:  s.MisfirePolicy,
		Concurrent:     s.Concurrent,
		Status:         s.Status,
	}, nil
}

type SysJobById struct {
	dto.ObjectById
}

type SysJobItem struct {
	JobId          int    `json:"jobId"`
	JobName        string `json:"jobName" validate:"required"` // 名称
	JobGroup       string `json:"jobGroup"`                    // 任务分组
	JobType        int    `json:"jobType"`                     // 任务类型
	CronExpression string `json:"cronExpression"`              // cron表达式
	InvokeTarget   string `json:"invokeTarget"`                // 调用目标
	Args           string `json:"args"`                        // 目标参数
	MisfirePolicy  int    `json:"misfirePolicy"`               // 执行策略
	Concurrent     int    `json:"concurrent"`                  // 是否并发
	Status         int    `json:"status"`                      // 状态
	EntryId        int    `json:"entryId"`                     // job启动时返回的id
}
