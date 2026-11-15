package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

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
