package actions_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/gatework/motor-core/sdk/config"

	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/models"
	"github.com/gatework/motor/internal/testconfig"
)

type boolRow struct {
	models.Model
	models.ControlBy
	Enabled bool
}

func (*boolRow) TableName() string { return "bool_row" }
func (s *boolRow) GetID() any      { return s.ID }

type boolControl struct {
	ID      int  `json:"id"`
	Enabled bool `json:"enabled"`
}

func (s *boolControl) Bind(c *gin.Context) error { return c.ShouldBindJSON(s) }
func (s *boolControl) ToModel() (*boolRow, error) {
	return &boolRow{ID: s.ID, CreateBy: 999, Enabled: s.Enabled}, nil
}
func (s *boolControl) UpdateFields() []string { return []string{"Enabled", "Id", "CreateBy", "*"} }
func TestGenericUpdateIncludesExplicitFalseAndProtectsCreator(t *testing.T) {

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = false
	})

	db := probeDB(t, t.Name(), nil)
	if err := db.AutoMigrate(&boolRow{}); err != nil {
		t.Fatal(err)
	}
	row := boolRow{Enabled: true, CreateBy: 42}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	r := probeEngine(db, func(r gin.IRoutes) { r.PUT("/row", actions.Update[boolRow, boolControl]()) })
	_, body := serve(r, http.MethodPut, "/row", `{"id":1,"enabled":false}`)
	var after boolRow
	if err := db.First(&after, row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Enabled || after.CreateBy != 42 || after.UpdateBy != 7 {
		t.Fatalf("update response=%s row=%+v", body, after)
	}
}
