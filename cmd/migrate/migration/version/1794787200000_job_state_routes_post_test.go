package version

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestJobStateRouteGrantsMoveToPost(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { pool.Close() })
	for _, sql := range []string{
		"CREATE TABLE sys_api (id INTEGER PRIMARY KEY, handle TEXT, path TEXT, action TEXT)",
		"CREATE TABLE casbin_rule (id INTEGER PRIMARY KEY, ptype TEXT, v0 TEXT, v1 TEXT, v2 TEXT)",
		"INSERT INTO sys_api VALUES (57, 'GET /api/v1/job/remove/:id', '/api/v1/job/remove/:id', 'GET'), (58, 'GET /api/v1/job/start/:id', '/api/v1/job/start/:id', 'GET'), (59, 'GET /api/v1/sysjob', '/api/v1/sysjob', 'GET')",
		"INSERT INTO casbin_rule VALUES (1, 'p', 'operator', '/api/v1/job/start/:id', 'GET'), (2, 'p', 'operator', '/api/v1/sysjob', 'GET')",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := moveJobStateRoutesToPost(db); err != nil {
			t.Fatal(err)
		}
	}
	var actions []string
	if err := db.Table("sys_api").Order("id").Pluck("action", &actions).Error; err != nil {
		t.Fatal(err)
	}
	if actions[0] != "POST" || actions[1] != "POST" || actions[2] != "GET" {
		t.Fatalf("sys_api methods %v", actions)
	}
	var grants []string
	if err := db.Table("casbin_rule").Order("id").Pluck("v2", &grants).Error; err != nil {
		t.Fatal(err)
	}
	if grants[0] != "POST" || grants[1] != "GET" {
		t.Fatalf("grant methods %v", grants)
	}
}
