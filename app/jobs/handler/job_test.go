package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk"

	"github.com/gatework/motor/app/jobs"
	"github.com/gatework/motor/app/jobs/models"
	jobdto "github.com/gatework/motor/app/jobs/service/dto"
	"github.com/gatework/motor/common/actions"
)

func TestJobWritesReconcileScheduler(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	if err = db.AutoMigrate(&models.SysJob{}, &models.SysJobLease{}); err != nil {
		t.Fatal(err)
	}
	if err = db.Create(&models.SysJobLease{Name: models.SchedulerLeaseName}).Error; err != nil {
		t.Fatal(err)
	}
	jobs.Setup(map[string]*gorm.DB{"*": db})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		sdk.Runtime.RunShutdown(ctx)
		sqlDB.Close()
	})
	c := sdk.Runtime.Crontab("*")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("db", db); c.Next() })
	router.POST("/sysjob", ReconcileWrites, actions.Create[models.SysJob, jobdto.JobControl]())
	router.PUT("/sysjob", ReconcileWrites, actions.Update[models.SysJob, jobdto.JobControl]())
	router.DELETE("/sysjob", ReconcileWrites, actions.Delete[models.SysJob, jobdto.JobByID]())
	request := func(method, body string) {
		t.Helper()
		req := httptest.NewRequest(method, "/sysjob", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		router.ServeHTTP(out, req)
		var response struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(out.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		if response.Code != 200 {
			t.Fatalf("%s: %s", method, out.Body.String())
		}
	}
	request("POST", `{"jobName":"initial","jobType":2,"status":2,"cronExpression":"0 * * * * *","args":"old","concurrent":1,"entryId":999}`)
	var saved models.SysJob
	if err := db.First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if len(c.Entries()) != 1 || saved.EntryID == 999 {
		t.Fatal("create did not reconcile or accepted entryId")
	}
	request("PUT", fmt.Sprintf(`{"jobId":%d,"args":"","concurrent":0,"entryId":123}`, saved.JobID))
	if err := db.First(&saved, saved.JobID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Args != "" || saved.Concurrent != 0 || saved.JobName != "initial" || saved.EntryID == 123 {
		t.Fatalf("presence update lost data: %+v", saved)
	}
	entries := c.Entries()
	if len(entries) != 1 || entries[0].Job.(*jobs.TaskJob).Args != "" {
		t.Fatal("update left stale schedule")
	}
	invalid := httptest.NewRequest(http.MethodPut, "/sysjob", strings.NewReader(fmt.Sprintf(`{"jobId":%d,"cronExpression":"invalid"}`, saved.JobID)))
	invalid.Header.Set("Content-Type", "application/json")
	rejected := httptest.NewRecorder()
	router.ServeHTTP(rejected, invalid)
	var invalidResponse struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(rejected.Body.Bytes(), &invalidResponse); err != nil {
		t.Fatal(err)
	}
	if invalidResponse.Code != 422 {
		t.Fatalf("invalid cron accepted: %s", rejected.Body.String())
	}
	if err := db.First(&saved, saved.JobID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.CronExpression != "0 * * * * *" || len(c.Entries()) != 1 {
		t.Fatal("invalid cron changed persisted or scheduled state")
	}
	request("PUT", fmt.Sprintf(`{"jobId":%d,"status":1}`, saved.JobID))
	if len(c.Entries()) != 0 {
		t.Fatal("disable left runnable job")
	}
	request("PUT", fmt.Sprintf(`{"jobId":%d,"status":2}`, saved.JobID))
	if len(c.Entries()) != 1 {
		t.Fatal("enable did not schedule")
	}
	request("DELETE", fmt.Sprintf(`{"ids":[%d]}`, saved.JobID))
	if len(c.Entries()) != 0 {
		t.Fatal("delete left runnable job")
	}
}
