package dto

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor/app/demo/models"
)

// ToModel builds the row to write; the key has to reach it, or an update
// matches no row.
func TestToModelCarriesID(t *testing.T) {
	c := &DemoProductControl{ID: 42, Name: "示例", Code: "P-42", Price: 9.9}
	p, err := c.ToModel()
	if err != nil {
		t.Fatalf("ToModel: %v", err)
	}
	if p.ID != 42 {
		t.Errorf("key not carried: got %d, want 42", p.ID)
	}
	if p.Name != "示例" || p.Code != "P-42" || p.Price != 9.9 {
		t.Errorf("fields mapped wrongly: %+v", p)
	}
}

func TestTableName(t *testing.T) {
	if got := (&models.DemoProduct{}).TableName(); got != "demo_product" {
		t.Errorf("TableName() = %q, want %q", got, "demo_product")
	}
}

func TestBindPreservesPresenceForJSONAndForms(t *testing.T) {
	for _, tc := range []struct{ name, contentType, body string }{
		{"JSON", "application/json", `{"price":0,"remark":""}`},
		{"JSON case insensitive", "application/json", `{"PRICE":0,"REMARK":""}`},
		{"form", "application/x-www-form-urlencoded", "price=0&remark="},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPut, "/product/42", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", tc.contentType)
			c.Params = gin.Params{{Key: "id", Value: "42"}}
			var req DemoProductControl
			if err := req.Bind(c); err != nil {
				t.Fatal(err)
			}
			if req.ID != 42 || req.Price != 0 || req.Remark != "" || !reflect.DeepEqual(req.UpdateFields(), []string{"price", "remark"}) {
				t.Fatalf("bound request=%+v fields=%v", req, req.UpdateFields())
			}
		})
	}
}
