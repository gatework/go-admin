package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/logger"
)

func TestAuditPasswordOperationLogRedaction(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/user/pwd/set", strings.NewReader(`{"oldPassword":"audit-old-secret","newPassword":"audit-new-secret"}`))
	body := readOperParam(c, logger.NewHelper(logger.Default()))
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
			c.Request = httptest.NewRequest(http.MethodPut, "/?token=secret-value&name=visible", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", tc.contentType)
			got := readOperParam(c, logger.NewHelper(logger.Default()))
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

func TestRecoverRedactsURLCredentials(t *testing.T) {
	var output bytes.Buffer
	original := logger.Default()
	logger.SetDefault(logger.NewLogger(logger.WithOutput(&output)))
	defer logger.SetDefault(original)
	r := gin.New()
	r.Use(Recover)
	r.GET("/broken", func(c *gin.Context) { panic("invalid request") })
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/broken?token=panic-secret&name=visible", nil))
	logged := output.Bytes()
	if strings.Contains(string(logged), "panic-secret") || !strings.Contains(string(logged), "name=visible") {
		t.Fatalf("unsafe or empty panic log: %s", logged)
	}
}
