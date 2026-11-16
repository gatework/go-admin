package version

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestGeneratorWriteGrantsMoveToPost(t *testing.T) {
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
	createCasbinRule(t, db)
	for _, sql := range []string{
		"CREATE TABLE sys_api (id INTEGER PRIMARY KEY, handle TEXT, path TEXT, action TEXT)",
		"INSERT INTO sys_api VALUES (1, 'GET /api/v1/gen/todb/:tableId', '/api/v1/gen/todb/:tableId', 'GET'), (2, 'GET /api/v1/gen/preview/:tableId', '/api/v1/gen/preview/:tableId', 'GET')",
		"INSERT INTO casbin_rule (id, ptype, v0, v1, v2) VALUES (1, 'p', 'developer', '/api/v1/gen/todb/:tableId', 'GET'), (2, 'p', 'developer', '/api/v1/gen/preview/:tableId', 'GET')",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := moveGeneratorWritesToPost(db); err != nil {
			t.Fatal(err)
		}
	}
	var apis, grants []string
	if err := db.Table("sys_api").Order("id").Pluck("action", &apis).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("casbin_rule").Order("id").Pluck("v2", &grants).Error; err != nil {
		t.Fatal(err)
	}
	if apis[0] != "POST" || apis[1] != "GET" || grants[0] != "POST" || grants[1] != "GET" {
		t.Fatalf("apis %v grants %v", apis, grants)
	}
}
