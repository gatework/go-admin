package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk"
)

// database/sql watches the context of a query that returns rows inside a
// transaction from a goroutine of its own, which can still be reading it
// after the handler has returned. GORM wraps every write in a transaction,
// and an INSERT on SQLite or PostgreSQL returns the new key as a row. gin
// reuses its Context for the next request as soon as the handler returns, so
// a query given the gin.Context races with that reuse; given the request's
// own context, it does not. Sequential requests are enough for the reuse,
// and -race, which make test runs with, reports it.
type probeRow struct {
	ID int `gorm:"primaryKey;autoIncrement"`
}

func (probeRow) TableName() string { return "with_context_db_probe" }

func TestWithContextDBDoesNotHandQueriesThePooledContext(t *testing.T) {
	const host = "with-context-db.test"
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&probeRow{}); err != nil {
		t.Fatal(err)
	}
	previous := sdk.Runtime.DB(host)
	sdk.Runtime.SetDB(host, db)
	t.Cleanup(func() { sdk.Runtime.SetDB(host, previous) })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(WithContextDB)
	r.GET("/", func(c *gin.Context) {
		if err := c.MustGet("db").(*gorm.DB).Create(&probeRow{}).Error; err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
	})
	for i := range 20 {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = host
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d answered %d", i, w.Code)
		}
	}
}

func TestWithContextDBRejectsUnknownTenant(t *testing.T) {
	r := gin.New()
	r.Use(WithContextDB)
	reached := false
	r.GET("/", func(c *gin.Context) { reached = true; c.Status(200) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "not-configured.test"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if reached || w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unknown tenant reached handler=%v status=%d", reached, w.Code)
	}
}
