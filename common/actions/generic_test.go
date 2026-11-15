package actions_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/gatework/motor-core/jwtauth"
	"github.com/gatework/motor-core/sdk/config"

	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/common/dto"
	"github.com/gatework/motor/common/models"
	"github.com/gatework/motor/internal/testconfig"
)

// probeSearch is probeIndexReq with a search field, so a list request can
// be told apart from another in its response.
type probeSearch struct {
	dto.Pagination `search:"-"`
	Name           string `form:"name" search:"type:exact;column:name;table:action_probe_row"`
}

func (p *probeSearch) Generate() dto.Index       { o := *p; return &o }
func (p *probeSearch) Bind(c *gin.Context) error { return c.ShouldBind(p) }
func (p *probeSearch) GetNeedSearch() any        { return *p }

// probeById names rows the way every ById DTO here does.
type probeByID struct {
	dto.ObjectByID
}

// probeControl carries both the old actions' methods and ToModel, so one
// request type serves both generations of action.
type probeControl struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (s *probeControl) Bind(c *gin.Context) error { return c.ShouldBindJSON(s) }
func (s *probeControl) GetID() any                { return s.ID }

func (s *probeControl) ToModel() (*probeRow, error) {
	return &probeRow{ID: s.ID, Name: s.Name}, nil
}

// probeKey names a row by a string key, which the old actions handed to GORM
// as a SQL condition.
type probeKey struct {
	Key string `uri:"id"`
}

func (s *probeKey) Bind(c *gin.Context) error { return c.ShouldBindUri(s) }
func (s *probeKey) GetID() any                { return s.Key }

type capturingLogger struct {
	gormlogger.Interface
	mu    sync.Mutex
	stmts []string
}

func (l *capturingLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	l.mu.Lock()
	l.stmts = append(l.stmts, sql)
	l.mu.Unlock()
}

type probeRow struct {
	models.Model
	models.ControlBy
	Name string
}

func (*probeRow) TableName() string               { return "action_probe_row" }
func (e *probeRow) Generate() models.ActiveRecord { o := *e; return &o }
func (e *probeRow) GetID() any                    { return e.ID }

func probeDB(t *testing.T, name string, l gormlogger.Interface) *gorm.DB {
	t.Helper()
	if l == nil {
		l = gormlogger.Default.LogMode(gormlogger.Silent)
	}
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{Logger: l})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&probeRow{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// probeEngine serves the five routes from db, as caller 7.
func probeEngine(db *gorm.DB, register func(r gin.IRoutes)) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if db != nil {
			c.Set("db", db)
		}
		c.Set(jwtauth.JwtPayloadKey, jwtauth.MapClaims{"identity": float64(7)})
		c.Next()
	})
	register(r)
	return r
}

func newRoutes(r gin.IRoutes) {
	r.GET("/x", actions.Index[probeRow, probeSearch]())
	r.GET("/x/:id", actions.View[probeRow, probeByID]())
	r.POST("/x", actions.Create[probeRow, probeControl]())
	r.PUT("/x", actions.Update[probeRow, probeControl]())
	r.DELETE("/x", actions.Delete[probeRow, probeByID]())
}

var requestIDField = regexp.MustCompile(`"requestId":"[^"]*"`)

func serve(r *gin.Engine, method, path, body string) (int, string) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w.Code, requestIDField.ReplaceAllString(w.Body.String(), `"requestId":""`)
}

// Every action that reads or changes existing rows applies the caller's
// data permission: Index, View, Update and Delete.
func TestGenericActionsApplyDataPermission(t *testing.T) {

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	for _, req := range []struct{ method, path, body string }{
		{"GET", "/x?pageIndex=1&pageSize=10", ""},
		{"GET", "/x/1", ""},
		{"PUT", "/x", `{"id":1,"name":"a"}`},
		{"DELETE", "/x", `{"ids":[1]}`},
	} {
		cl := &capturingLogger{Interface: gormlogger.Default.LogMode(gormlogger.Silent)}
		db := probeDB(t, strings.NewReplacer("/", "_", "?", "_").Replace(t.Name()+req.method+req.path), cl)
		r := probeEngine(db, func(r gin.IRoutes) {
			self := func(c *gin.Context) {
				c.Set(actions.PermissionKey, &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 7})
			}
			r.GET("/x", self, actions.Index[probeRow, probeSearch]())
			r.GET("/x/:id", self, actions.View[probeRow, probeByID]())
			r.PUT("/x", self, actions.Update[probeRow, probeControl]())
			r.DELETE("/x", self, actions.Delete[probeRow, probeByID]())
		})
		serve(r, req.method, req.path, req.body)
		if !strings.Contains(cl.all(), "action_probe_row.create_by = ") {
			t.Errorf("%s %s ran without the data-permission scope:\n%s", req.method, req.path, cl.all())
		}
	}
}

// A string key is compared as a value. Handed to Where on its own, "1=1"
// is read as a SQL condition and matches every row.
func TestGenericViewMatchesAStringKeyAsAValue(t *testing.T) {
	db := probeDB(t, t.Name(), nil)
	if err := db.Create(&probeRow{Name: "a"}).Error; err != nil {
		t.Fatal(err)
	}
	r := probeEngine(db, func(r gin.IRoutes) { r.GET("/x/:id", actions.View[probeRow, probeKey]()) })

	_, body := serve(r, "GET", "/x/1=1", "")
	var res struct{ Code int }
	_ = json.Unmarshal([]byte(body), &res)
	if res.Code != http.StatusNotFound {
		t.Errorf("GET /x/1=1 answered %s; want the row not found", body)
	}
}

// Without a database in the request the old actions wrote nothing, which a
// client reads as an empty 200.
func TestGenericActionsAnswerAMissingDatabase(t *testing.T) {
	r := probeEngine(nil, newRoutes)
	for _, req := range []struct{ method, path, body string }{
		{"GET", "/x?pageIndex=1&pageSize=10", ""},
		{"GET", "/x/1", ""},
		{"POST", "/x", `{"name":"a"}`},
		{"PUT", "/x", `{"id":1}`},
		{"DELETE", "/x", `{"ids":[1]}`},
	} {
		_, body := serve(r, req.method, req.path, req.body)
		if !strings.Contains(body, `"code":500`) || !strings.Contains(body, "数据库连接获取失败") {
			t.Errorf("%s %s answered %q; want a 500 naming the connection", req.method, req.path, body)
		}
	}
}

// Concurrent requests to one route each see only what they asked for. The
// generic actions build every value per request, so there is nothing to
// share; this holds that shape.
func TestGenericIndexKeepsConcurrentRequestsApart(t *testing.T) {
	db := probeDB(t, t.Name(), nil)
	for i := range 20 {
		if err := db.Create(&probeRow{Name: fmt.Sprintf("n%d", i)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	r := probeEngine(db, newRoutes)

	var wg sync.WaitGroup
	for i := range 20 {
		name := fmt.Sprintf("n%d", i)
		wg.Go(func() {
			for range 10 {
				_, body := serve(r, "GET", "/x?pageIndex=1&pageSize=10&name="+name, "")
				var res struct {
					Data struct{ List []probeRow }
				}
				if err := json.Unmarshal([]byte(body), &res); err != nil {
					t.Errorf("decoding %q: %v", body, err)
					return
				}
				if len(res.Data.List) != 1 || res.Data.List[0].Name != name {
					t.Errorf("asked for %s, got %+v", name, res.Data.List)
					return
				}
			}
		})
	}
	wg.Wait()
}

func (l *capturingLogger) all() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.stmts, "\n")
}

// A row that is missing and a row outside the caller's data permission answer
// alike - 404, as View does - so a client cannot probe for rows it may not see.
// Update and Delete used to answer 403 "no permission" for a missing row too.
func TestGenericChangesToUnreachableRowsAnswerNotFound(t *testing.T) {
	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})
	db := probeDB(t, t.Name(), nil)
	someoneElses := probeRow{Name: "theirs"}
	someoneElses.SetCreateBy(99)
	if err := db.Create(&someoneElses).Error; err != nil {
		t.Fatal(err)
	}
	r := probeEngine(db, func(r gin.IRoutes) {
		self := func(c *gin.Context) {
			c.Set(actions.PermissionKey, &actions.DataPermission{DataScope: actions.DataScopeSelf, UserID: 7})
		}
		r.PUT("/x", self, actions.Update[probeRow, probeControl]())
		r.DELETE("/x", self, actions.Delete[probeRow, probeByID]())
	})
	for _, req := range []struct{ method, body string }{
		{"PUT", fmt.Sprintf(`{"id":%d,"name":"mine now"}`, someoneElses.ID)},
		{"PUT", `{"id":4242,"name":"missing"}`},
		{"DELETE", fmt.Sprintf(`{"ids":[%d]}`, someoneElses.ID)},
		{"DELETE", `{"ids":[4242]}`},
	} {
		status, body := serve(r, req.method, "/x", req.body)
		var res struct{ Code int }
		_ = json.Unmarshal([]byte(body), &res)
		if res.Code != http.StatusNotFound {
			t.Errorf("%s %s: status %d body %s; want not found", req.method, req.body, status, body)
		}
	}
	var stored probeRow
	if err := db.First(&stored, someoneElses.ID).Error; err != nil || stored.Name != "theirs" {
		t.Fatalf("row outside the caller's scope changed: %+v %v", stored, err)
	}
}
