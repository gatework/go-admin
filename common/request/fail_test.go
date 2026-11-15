package request

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/api"

	"github.com/gatework/motor/common/errkind"
)

func TestFailAnswersWithTheKindAndNeverLeaksDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	conflict := errkind.New(errkind.Conflict, "firewall.version_conflict")
	for name, tc := range map[string]struct {
		err  error
		code int
		msg  string
	}{
		"classified":   {fmt.Errorf("draft 7: %w", conflict), http.StatusConflict, "firewall.version_conflict"},
		"missing row":  {fmt.Errorf("loading: %w", gorm.ErrRecordNotFound), http.StatusNotFound, "not_found"},
		"duplicate":    {fmt.Errorf("saving: %w", gorm.ErrDuplicatedKey), http.StatusConflict, "duplicate"},
		"unclassified": {errors.New("pq: password authentication failed for user motor"), http.StatusInternalServerError, "internal_error"},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
			a, err := api.New(c, api.Dependencies{})
			if err != nil {
				t.Fatal(err)
			}
			Fail(a, tc.err)
			var body struct {
				Code int
				Msg  string
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != tc.code || body.Msg != tc.msg {
				t.Fatalf("got %d %q, want %d %q", body.Code, body.Msg, tc.code, tc.msg)
			}
			if strings.Contains(recorder.Body.String(), "password") {
				t.Fatal("error detail reached the client")
			}
		})
	}
}
