package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDecodeStrictJSONRefusesAnythingButOneKnownValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	type body struct {
		Name string `json:"name"`
	}
	for input, want := range map[string]bool{
		`{"name":"core-1"}`:                           true,
		`{"name":"core-1","extra":true}`:              false, // unknown field
		`{"name":"core-1"} {}`:                        false, // trailing value
		`{"name":"` + strings.Repeat("x", 100) + `"}`: false, // over the limit
		``: false,
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(input))
		var out body
		if got := decodeStrictJSON(c, &out, 64); got != want {
			t.Errorf("%q: %v, want %v", input, got, want)
		}
	}
}
