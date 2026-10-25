package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/go-admin-team/go-admin-core/v2/logger"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestAuditPasswordOperationLogRedaction(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("PUT", "/api/v1/user/pwd/set", strings.NewReader(`{"oldPassword":"audit-old-secret","newPassword":"audit-new-secret"}`))
	body := readOperParam(c, logger.NewHelper(logger.DefaultLogger))
	fields := operaLogFields(c, "127.0.0.1", 200, c.Request.RequestURI, "PUT", 0, body, "", 200)
	logged := fields["operParam"].(string)
	if strings.Contains(logged, "audit-old-secret") || strings.Contains(logged, "audit-new-secret") {
		t.Fatal("operation log payload contains plaintext oldPassword and newPassword")
	}
}

func TestOperationLogScrubsNestedFieldsAndPreservesRequest(t *testing.T) {
	for _, tc := range []struct{ name, contentType, body string }{
		{"json", "application/json", `{"items":[{"password":"secret-value","name":"visible"}],"access_token":"secret-value"}`},
		{"form", "application/x-www-form-urlencoded", "password=secret-value&name=visible"},
		{"malformed", "application/json", `{"password":"secret-value"`},
		{"multipart", "multipart/form-data; boundary=abc", "--abc\r\nsecret-value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("PUT", "/?token=secret-value&name=visible", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", tc.contentType)
			got := readOperParam(c, logger.NewHelper(logger.DefaultLogger))
			if strings.Contains(got, "secret-value") {
				t.Fatalf("credential retained: %s", got)
			}
			if tc.name == "json" || tc.name == "form" {
				if !strings.Contains(got, "visible") {
					t.Fatal("ordinary field lost")
				}
			}
			body, err := io.ReadAll(c.Request.Body)
			if err != nil || string(body) != tc.body {
				t.Fatal("request changed")
			}
			if strings.Contains(redactLogURL(c.Request.URL), "secret-value") {
				t.Fatal("query token retained")
			}
		})
	}
}

func TestCustomErrorRedactsURLCredentials(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original; reader.Close(); writer.Close() }()
	r := gin.New()
	r.Use(CustomError)
	r.GET("/broken", func(c *gin.Context) { panic("CustomError#422#invalid request") })
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", "/broken?token=panic-secret&name=visible", nil))
	writer.Close()
	os.Stdout = original
	logged, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logged), "panic-secret") || !strings.Contains(string(logged), "name=visible") {
		t.Fatalf("unsafe or empty custom-error log: %s", logged)
	}
}
