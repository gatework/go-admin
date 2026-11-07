package actions_test

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"

	"go-admin/common/actions"
	"go-admin/common/models"
)

type boolRow struct {
	models.Model
	models.ControlBy
	Enabled bool
}

func (boolRow) TableName() string { return "bool_row" }
func (s *boolRow) GetId() any     { return s.Id }

type boolControl struct {
	Id      int  `json:"id"`
	Enabled bool `json:"enabled"`
}

func (s *boolControl) Bind(c *gin.Context) error { return c.ShouldBindJSON(s) }
func (s *boolControl) ToModel() (*boolRow, error) {
	return &boolRow{Model: models.Model{Id: s.Id}, ControlBy: models.ControlBy{CreateBy: 999}, Enabled: s.Enabled}, nil
}
func (s *boolControl) UpdateFields() []string { return []string{"Enabled", "Id", "CreateBy", "*"} }
func TestGenericUpdateIncludesExplicitFalseAndProtectsCreator(t *testing.T) {
	old := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = false
	t.Cleanup(func() { config.ApplicationConfig.EnableDP = old })
	db := probeDB(t, t.Name(), nil)
	if err := db.AutoMigrate(&boolRow{}); err != nil {
		t.Fatal(err)
	}
	row := boolRow{Enabled: true, ControlBy: models.ControlBy{CreateBy: 42}}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	r := probeEngine(db, func(r gin.IRoutes) { r.PUT("/row", actions.Update[boolRow, boolControl]()) })
	_, body := serve(r, http.MethodPut, "/row", `{"id":1,"enabled":false}`)
	var after boolRow
	if err := db.First(&after, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if after.Enabled || after.CreateBy != 42 || after.UpdateBy != 7 {
		t.Fatalf("update response=%s row=%+v", body, after)
	}
}
