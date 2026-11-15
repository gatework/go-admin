package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	jwt "github.com/gatework/motor-core/jwtauth"

	dto "github.com/gatework/motor/app/net/service/dto/wireless"
)

func TestCollectionRejectsSpoofedActorAndTrailingJSON(t *testing.T) {
	for _, body := range []string{`{"requestKey":"test-test","createdBy":99}`, `{"requestKey":"test-test"} {}`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		var r dto.CollectionReq
		if decodeWirelessRequest(c, &r) {
			t.Fatal("accepted untrusted body")
		}
	}
}

// Every wireless route takes its controller and item IDs from the path; one
// that is not a positive integer is refused before any query runs.
func TestWirelessRejectsMalformedPathIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", (*gorm.DB)(nil))
		c.Set(jwt.JwtPayloadKey, jwt.MapClaims{"identity": float64(7), "rolekey": "operator"})
		c.Next()
	})
	a := Wireless{}
	r.GET("/controllers/:controllerId", a.Controller)
	r.GET("/controllers/:controllerId/access-points/:id", a.APDetail)
	for _, path := range []string{"/controllers/x", "/controllers/0", "/controllers/-1", "/controllers/3/access-points/abc"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if !strings.Contains(w.Body.String(), `"code":400`) {
			t.Errorf("%s answered %s, want code 400", path, w.Body.String())
		}
	}
}
