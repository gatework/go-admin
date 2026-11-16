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

func TestPageOf(t *testing.T) {
	rows := []int{1, 2, 3, 4, 5}
	for _, tc := range []struct {
		index, size int
		want        []int
	}{{1, 2, []int{1, 2}}, {3, 2, []int{5}}, {4, 2, []int{}}, {1, 10, rows}} {
		got, n := pageOf(rows, tc.index, tc.size)
		if n != 5 || len(got) != len(tc.want) || len(got) > 0 && got[0] != tc.want[0] {
			t.Errorf("pageOf(%d, %d) = %v, %d; want %v, 5", tc.index, tc.size, got, n, tc.want)
		}
	}
}
