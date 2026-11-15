package dto_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/sdk/config"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/internal/testconfig"
)

// Post CRUD runs on the generic actions. What is post-specific: name and code
// are required, an update replaces every writable column (clearing a remark
// clears it), and posts are reference data that data permission does not hide.
func TestPostRoutes(t *testing.T) {
	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SysPost{}); err != nil {
		t.Fatal(err)
	}
	seeded := models.SysPost{CreateBy: 99, PostName: "Engineer", PostCode: "eng", Remark: "seeded"}
	if err := db.Create(&seeded).Error; err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set(jwtauth.JwtPayloadKey, jwtauth.MapClaims{"identity": float64(7)})
	}, actions.Unscoped)
	engine.GET("/post", actions.Index[models.SysPost, dto.PostSearch]())
	engine.POST("/post", actions.Create[models.SysPost, dto.PostControl]())
	engine.PUT("/post/:id", actions.Update[models.SysPost, dto.PostControl]())
	call := func(method, path, body string) (int, string) {
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		var envelope struct{ Code int }
		_ = json.Unmarshal(response.Body.Bytes(), &envelope)
		return envelope.Code, response.Body.String()
	}

	if code, body := call("GET", "/post?pageIndex=1&pageSize=10", ""); code != 200 || !strings.Contains(body, `"count":1`) {
		t.Fatalf("a post created by someone else is hidden by data permission: %s", body)
	}
	if code, _ := call("POST", "/post", `{"postName":"","postCode":"x"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("post without a name accepted: %d", code)
	}
	if code, body := call("PUT", "/post/1", `{"postName":"Senior engineer","postCode":"eng","sort":2,"status":2,"remark":""}`); code != 200 {
		t.Fatalf("update refused: %s", body)
	}
	var stored models.SysPost
	if err := db.First(&stored, seeded.PostID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PostName != "Senior engineer" || stored.Remark != "" || stored.Sort != 2 || stored.CreateBy != 99 || stored.UpdateBy != 7 {
		t.Fatalf("update not a full replace that keeps the creator: %+v", stored)
	}
	if code, _ := call("PUT", "/post/999", `{"postName":"ghost","postCode":"ghost"}`); code != http.StatusNotFound {
		t.Fatalf("update of a missing post answered %d", code)
	}
	var count int64
	db.Model(&models.SysPost{}).Count(&count)
	if count != 1 {
		t.Fatalf("update of a missing post inserted a row: %d rows", count)
	}
}
