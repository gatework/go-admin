package version

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMotorIdentityOnlyNormalizesShippedNames(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for _, sql := range []string{
		"CREATE TABLE sys_api (id INTEGER PRIMARY KEY, handle TEXT, action TEXT, path TEXT)",
		"CREATE TABLE sys_config (id INTEGER PRIMARY KEY, config_key TEXT, config_value TEXT)",
		"INSERT INTO sys_api VALUES (1, 'go-admin/app/admin/apis.SysApi.Get-fm', 'GET', '/api/v1/sys-api/:id'), (2, 'example.com/custom.SysApi.Get-fm', 'GET', '/custom'), (3, 'motor/app/other/handler/tools.Gen.GenMenuAndApi-fm', 'GET', '/api/v1/gen/todb/:tableId'), (4, 'example.com/custom.Gen.GenMenuAndApi-fm', 'GET', '/custom-gen')",
		"INSERT INTO sys_config VALUES (1,'sys_app_name','go-admin管理系统'),(2,'sys_app_name','Custom console')",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := normalizeMotorIdentity(db); err != nil {
			t.Fatal(err)
		}
	}
	var handles []string
	if err := db.Table("sys_api").Order("id").Pluck("handle", &handles).Error; err != nil {
		t.Fatal(err)
	}
	if handles[0] != "GET /api/v1/sys-api/:id" || handles[1] != "example.com/custom.SysApi.Get-fm" {
		t.Fatal(handles)
	}
	if handles[2] != "GET /api/v1/gen/todb/:tableId" || handles[3] != "example.com/custom.Gen.GenMenuAndApi-fm" {
		t.Fatal(handles)
	}
	var names []string
	if err := db.Table("sys_config").Order("id").Pluck("config_value", &names).Error; err != nil {
		t.Fatal(err)
	}
	if names[0] != "motor" || names[1] != "Custom console" {
		t.Fatal(names)
	}
}
