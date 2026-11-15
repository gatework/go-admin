package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestServerInfoAnswersTheFieldsTheUIReads(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/server-monitor", ServerMonitor{}.ServerInfo)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/server-monitor", nil))
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", response.Body.String(), err)
	}
	for _, field := range []string{"os", "mem", "cpu", "disk", "net", "swap", "bootTime"} {
		if _, ok := body[field]; !ok {
			t.Errorf("missing %q in %s", field, response.Body.String())
		}
	}
	if _, ok := body["location"]; ok {
		t.Error("hard-coded location still reported")
	}
}
