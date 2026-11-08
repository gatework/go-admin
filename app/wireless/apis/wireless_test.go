package apis

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"go-admin/app/wireless/service/dto"
	"net/http/httptest"
	"testing"
)

func TestCollectionRejectsSpoofedActorAndTrailingJSON(t *testing.T) {
	for _, body := range []string{`{"requestKey":"test-test","createdBy":99}`, `{"requestKey":"test-test"} {}`} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", bytes.NewBufferString(body))
		var r dto.CollectionReq
		if decode(c, &r) {
			t.Fatal("accepted untrusted body")
		}
	}
}
