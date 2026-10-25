package dto

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSysJobControlBindPreservesPresence(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
		uri                     bool
	}{
		{"canonical JSON", "application/json", `{"jobId":42,"args":"","concurrent":0,"entryId":999}`, false},
		{"case insensitive JSON", "application/json", `{"JOBID":42,"ARGS":"","CONCURRENT":0,"ENTRYID":999}`, false},
		{"legacy form fields", "application/x-www-form-urlencoded", "JobId=42&Args=&Concurrent=0&EntryId=999", false},
		{"canonical form fields", "application/x-www-form-urlencoded", "jobId=42&args=&concurrent=0&entryId=999", false},
		{"URI authoritative", "application/json", `{"jobId":99,"args":"","concurrent":0,"entryId":999}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPut, "/sysjob", strings.NewReader(tc.body))
			ctx.Request.Header.Set("Content-Type", tc.contentType)
			if tc.uri {
				ctx.Params = gin.Params{{Key: "id", Value: "42"}}
			}
			var req SysJobControl
			if err := req.Bind(ctx); err != nil {
				t.Fatal(err)
			}
			fields := req.UpdateFields()
			sort.Strings(fields)
			if req.JobId != 42 || req.Args != "" || req.Concurrent != 0 || !reflect.DeepEqual(fields, []string{"Args", "Concurrent"}) {
				t.Fatalf("request=%+v fields=%v", req, fields)
			}
			model, err := req.ToModel()
			if err != nil {
				t.Fatal(err)
			}
			if model.EntryId != 0 {
				t.Fatal("client supplied process-local entry id persisted")
			}
		})
	}
}

func TestSysJobControlValidationMatchesScheduler(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		wantError  bool
	}{
		{"legacy optional weekday", `{"jobId":42,"cronExpression":"0 * * * *"}`, false},
		{"six fields", `{"jobId":42,"cronExpression":"0 * * * * *"}`, false},
		{"uppercase cron validation", `{"jobId":42,"CRONEXPRESSION":"invalid"}`, true},
		{"uppercase type validation", `{"jobId":42,"JOBTYPE":9}`, true},
		{"uppercase status validation", `{"jobId":42,"STATUS":9}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPut, "/sysjob", strings.NewReader(tc.body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			var req SysJobControl
			if err := req.Bind(ctx); (err != nil) != tc.wantError {
				t.Fatalf("Bind error=%v want error=%v", err, tc.wantError)
			}
		})
	}
}

func TestSysJobControlRequiresJobName(t *testing.T) {
	for _, tc := range []struct {
		name, method, body string
		wantError          bool
	}{
		{"create missing name", http.MethodPost, `{"jobType":2,"status":2,"cronExpression":"0 * * * * *"}`, true},
		{"create empty name", http.MethodPost, `{"jobName":"","jobType":2,"status":2,"cronExpression":"0 * * * * *"}`, true},
		{"create named job", http.MethodPost, `{"jobName":"nightly","jobType":2,"status":2,"cronExpression":"0 * * * * *"}`, false},
		{"update explicit empty name", http.MethodPut, `{"jobId":42,"jobName":""}`, true},
		{"update uppercase empty name", http.MethodPut, `{"jobId":42,"JOBNAME":""}`, true},
		{"update omitted name", http.MethodPut, `{"jobId":42,"args":"new"}`, false},
		{"update named job", http.MethodPut, `{"jobId":42,"jobName":"renamed"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(tc.method, "/sysjob", strings.NewReader(tc.body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			var req SysJobControl
			err := req.Bind(ctx)
			if (err != nil) != tc.wantError {
				t.Fatalf("Bind error=%v want error=%v", err, tc.wantError)
			}
			if tc.name == "update omitted name" {
				for _, field := range req.UpdateFields() {
					if field == "JobName" {
						t.Fatal("omitted name included in update")
					}
				}
			}
		})
	}
}
