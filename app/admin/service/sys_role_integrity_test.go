package service

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"go-admin/app/admin/models"
	"go-admin/app/admin/service/dto"
)

func integrityDB(t *testing.T) *gorm.DB {
	t.Helper()
	var dial gorm.Dialector = sqlite.Open(":memory:")
	previousDriver := config.DatabaseConfig.Driver
	t.Cleanup(func() { config.DatabaseConfig.Driver = previousDriver })
	config.DatabaseConfig.Driver = "sqlite3"
	if dsn := os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN"); dsn != "" {
		dial = postgres.Open(dsn)
		config.DatabaseConfig.Driver = "postgres"
	}
	db, err := gorm.Open(dial, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if config.DatabaseConfig.Driver == "postgres" {
		schema := fmt.Sprintf("integrity_%d", time.Now().UnixNano())
		if err := db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
			t.Fatal(err)
		}
		adminDB := db
		t.Cleanup(func() { adminDB.Exec("DROP SCHEMA " + schema + " CASCADE"); sqlDB.Close() })
		scoped, err := gorm.Open(postgres.Open(os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN")+" search_path="+schema), &gorm.Config{})
		if err != nil {
			t.Fatal(err)
		}
		db = scoped
		pool, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { pool.Close() })
	} else {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := db.AutoMigrate(&gormadapter.CasbinRule{}, &models.SysApi{}, &models.SysMenu{}, &models.SysRole{}, &models.SysDept{}, &models.SysPost{}, &models.SysDictType{}, &models.SysUser{}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"CREATE UNIQUE INDEX uk_sys_role_role_key ON sys_role(role_key,deleted_at)", "CREATE UNIQUE INDEX uk_sys_dict_type_dict_type ON sys_dict_type(dict_type,deleted_at)"} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func integrityEnforcer(t *testing.T, databases ...*gorm.DB) *casbin.SyncedEnforcer {
	t.Helper()
	m, err := casbinmodel.NewModelFromString(`[request_definition]
r = sub, obj, act
[policy_definition]
p = sub, obj, act
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = r.sub == p.sub && r.obj == p.obj && r.act == p.act`)
	if err != nil {
		t.Fatal(err)
	}
	e, err := casbin.NewSyncedEnforcer(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(databases) > 0 {
		a, err := gormadapter.NewAdapterByDBUseTableName(databases[0], "", "casbin_rule")
		if err != nil {
			t.Fatal(err)
		}
		e.SetAdapter(a)
	} else {
		e.EnableAutoSave(false)
	}
	return e
}

func TestRoleBatchDelete(t *testing.T) {
	db := integrityDB(t)
	roles := []models.SysRole{{RoleId: 101, RoleKey: "r1"}, {RoleId: 102, RoleKey: "r2"}}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatal(err)
	}
	s := &SysRole{}
	s.Orm = db
	if err := s.Remove(&dto.SysRoleDeleteReq{Ids: []int{101, 102}}, integrityEnforcer(t)); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&models.SysRole{}).Where("role_id IN ?", []int{101, 102}).Count(&count)
	if count != 0 {
		t.Fatalf("batch delete returned success but %d requested roles remain", count)
	}
}

func TestRoleRenameRevokesOldKey(t *testing.T) {
	db := integrityDB(t)
	role := models.SysRole{RoleId: 103, RoleKey: "old-role", RoleName: "role", DataScope: "1", Status: "2"}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	cb := integrityEnforcer(t, db)
	if _, err := cb.AddPolicy("old-role", "/sensitive", "GET"); err != nil {
		t.Fatal(err)
	}
	s := &SysRole{}
	s.Orm = db
	if err := s.Update(&dto.SysRoleUpdateReq{RoleId: 103, RoleKey: "new-role", RoleName: "role", DataScope: "1", Status: "2"}, cb); err != nil {
		t.Fatal(err)
	}
	if err := cb.LoadPolicy(); err != nil {
		t.Fatal(err)
	}
	allowed, err := cb.Enforce("old-role", "/sensitive", "GET")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("old role key retains authorization after role rename and removal of all menus")
	}
}

func TestRoleRollbackKeepsOldPolicies(t *testing.T) {
	if os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN") == "" {
		t.Skip("PostgreSQL transaction required")
	}
	db := integrityDB(t)
	oldMenu := models.SysMenu{MenuId: 301, MenuName: "old", SysApi: []models.SysApi{{Id: 401, Path: "/old", Action: "GET"}}}
	newMenu := models.SysMenu{MenuId: 302, MenuName: "new", SysApi: []models.SysApi{{Id: 402, Path: "/new", Action: "GET"}}}
	if err := db.Create(&oldMenu).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&newMenu).Error; err != nil {
		t.Fatal(err)
	}
	role := models.SysRole{RoleId: 501, RoleKey: "rollback-role", RoleName: "original", DataScope: "1", Status: "2", SysMenu: &[]models.SysMenu{oldMenu}}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	cb := integrityEnforcer(t, db)
	if _, err := cb.AddPolicy("rollback-role", "/old", "GET"); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE casbin_rule ADD CONSTRAINT audit_fail_new_policy CHECK(v1 <> '/new')").Error; err != nil {
		t.Fatal(err)
	}
	s := &SysRole{}
	s.Orm = db
	s.Log = logger.NewHelper(logger.DefaultLogger)
	err := s.Update(&dto.SysRoleUpdateReq{RoleId: 501, RoleKey: "rollback-role", RoleName: "changed", DataScope: "1", Status: "2", MenuIds: []int{302}}, cb)
	if err == nil {
		t.Fatal("expected injected policy insertion failure")
	}
	var got models.SysRole
	if err := db.Preload("SysMenu").First(&got, 501).Error; err != nil {
		t.Fatal(err)
	}
	if err := cb.LoadPolicy(); err != nil {
		t.Fatal(err)
	}
	allowed, err := cb.Enforce("rollback-role", "/old", "GET")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatalf("role transaction rolled back (name=%s menu=%d) but durable old policy was deleted", got.RoleName, (*got.SysMenu)[0].MenuId)
	}
}

func TestRoleBatchDeleteRemovesAllAssociationsAndPolicies(t *testing.T) {
	db := integrityDB(t)
	menus := []models.SysMenu{{MenuId: 1, Paths: "/0/1"}}
	depts := []models.SysDept{{DeptId: 1, DeptPath: "/0/1/"}}
	roles := []models.SysRole{{RoleId: 1, RoleKey: "one", SysMenu: &menus, SysDept: depts}, {RoleId: 2, RoleKey: "two", SysMenu: &menus, SysDept: depts}, {RoleId: 3, RoleKey: "keep"}}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatal(err)
	}
	cb := integrityEnforcer(t, db)
	for _, key := range []string{"one", "two", "keep"} {
		if _, err := cb.AddPolicy(key, "/api", "GET"); err != nil {
			t.Fatal(err)
		}
	}
	s := &SysRole{}
	s.Orm = db
	if err := s.Remove(&dto.SysRoleDeleteReq{Ids: []int{1, 2, 2, 999}}, cb); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"sys_role_menu", "sys_role_dept"} {
		var count int64
		if err := db.Table(table).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s retains %d associations", table, count)
		}
	}
	for _, key := range []string{"one", "two", "keep"} {
		allowed, err := cb.Enforce(key, "/api", "GET")
		if err != nil {
			t.Fatal(err)
		}
		if allowed != (key == "keep") {
			t.Fatalf("policy %s allowed=%v", key, allowed)
		}
	}
	var remaining models.SysRole
	if err := db.First(&remaining, 3).Error; err != nil {
		t.Fatal(err)
	}
}

func TestRoleUpdatesDoNotCreateMissingRows(t *testing.T) {
	for _, name := range []string{"update", "scope", "status"} {
		t.Run(name, func(t *testing.T) {
			db := integrityDB(t)
			s := &SysRole{}
			s.Orm = db
			cb := integrityEnforcer(t, db)
			var err error
			switch name {
			case "update":
				err = s.Update(&dto.SysRoleUpdateReq{RoleId: 999, RoleKey: "missing"}, cb)
			case "scope":
				err = s.UpdateDataScope(&dto.RoleDataScopeReq{RoleId: 999, DataScope: "1"}).Error
			case "status":
				err = s.UpdateStatus(&dto.UpdateStatusReq{RoleId: 999, Status: "2"})
			}
			if err == nil {
				t.Fatal("missing role mutation succeeded")
			}
			var count int64
			if err := db.Model(&models.SysRole{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("mutation created %d roles", count)
			}
		})
	}
}

type failingPolicyLoader struct{ persist.Adapter }

func (f failingPolicyLoader) LoadPolicy(casbinmodel.Model) error {
	return errors.New("injected policy reload failure")
}

func TestRolePolicyReloadFailureIsExplicitAndDeniesStalePolicy(t *testing.T) {
	db := integrityDB(t)
	if err := db.Create(&models.SysRole{RoleId: 1, RoleKey: "original"}).Error; err != nil {
		t.Fatal(err)
	}
	cb := integrityEnforcer(t, db)
	if _, err := cb.AddPolicy("original", "/old", "GET"); err != nil {
		t.Fatal(err)
	}
	cb.SetAdapter(failingPolicyLoader{cb.GetAdapter()})
	s := &SysRole{}
	s.Orm = db
	err := s.Update(&dto.SysRoleUpdateReq{RoleId: 1, RoleKey: "changed"}, cb)
	if err == nil || !strings.Contains(err.Error(), "committed but policy reload failed") {
		t.Fatalf("error = %v, want explicit post-commit failure", err)
	}
	var got models.SysRole
	if err := db.First(&got, 1).Error; err != nil {
		t.Fatal(err)
	}
	if got.RoleKey != "changed" {
		t.Fatalf("committed role key = %q", got.RoleKey)
	}
	allowed, err := cb.Enforce("original", "/old", "GET")
	if err != nil || allowed {
		t.Fatalf("stale policy allowed=%v error=%v", allowed, err)
	}
	var count int64
	if err := db.Model(&gormadapter.CasbinRule{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("old durable policies retained: %d", count)
	}
}

func TestRolePolicyWriteFailureRollsBackDatabaseAndKeepsCache(t *testing.T) {
	for _, name := range []string{"insert", "update", "delete"} {
		t.Run(name, func(t *testing.T) {
			db := integrityDB(t)
			menu := models.SysMenu{MenuId: 1, SysApi: []models.SysApi{{Id: 1, Path: "/new", Action: "GET"}}}
			if err := db.Create(&menu).Error; err != nil {
				t.Fatal(err)
			}
			if name != "insert" {
				if err := db.Create(&models.SysRole{RoleId: 1, RoleKey: "existing", RoleName: "original"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			cb := integrityEnforcer(t, db)
			if _, err := cb.AddPolicy("existing", "/old", "GET"); err != nil {
				t.Fatal(err)
			}
			injected := errors.New("injected policy persistence failure")
			fail := func(tx *gorm.DB) {
				if tx.Statement.Table == "casbin_rule" {
					tx.AddError(injected)
				}
			}
			if name == "delete" {
				if err := db.Callback().Delete().Before("gorm:delete").Register("integrity:policy_delete", fail); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := db.Callback().Create().Before("gorm:create").Register("integrity:policy_create", fail); err != nil {
					t.Fatal(err)
				}
			}
			s := &SysRole{}
			s.Orm = db
			var err error
			switch name {
			case "insert":
				err = s.Insert(&dto.SysRoleInsertReq{RoleId: 1, RoleKey: "existing", MenuIds: []int{1}}, cb)
			case "update":
				err = s.Update(&dto.SysRoleUpdateReq{RoleId: 1, RoleKey: "existing", RoleName: "changed", MenuIds: []int{1}}, cb)
			case "delete":
				err = s.Remove(&dto.SysRoleDeleteReq{Ids: []int{1}}, cb)
			}
			if !errors.Is(err, injected) {
				t.Fatalf("error=%v, want policy failure", err)
			}
			var count int64
			if err := db.Model(&models.SysRole{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			want := int64(1)
			if name == "insert" {
				want = 0
			}
			if count != want {
				t.Fatalf("failed role write left %d roles, want %d", count, want)
			}
			if name != "insert" {
				var got models.SysRole
				if err := db.First(&got, 1).Error; err != nil {
					t.Fatal(err)
				}
				if got.RoleName != "original" {
					t.Fatalf("role name changed: %q", got.RoleName)
				}
			}
			if err := db.Table("sys_role_menu").Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("failed policy write left %d menu associations", count)
			}
			for _, reload := range []bool{false, true} {
				if reload {
					if err := cb.LoadPolicy(); err != nil {
						t.Fatal(err)
					}
				}
				allowed, err := cb.Enforce("existing", "/old", "GET")
				if err != nil || !allowed {
					t.Fatalf("rollback lost old policy reload=%v allowed=%v err=%v", reload, allowed, err)
				}
			}
		})
	}
}

func TestRoleRenameCannotTakeAnotherRolesPoliciesWithoutUniqueIndex(t *testing.T) {
	db := integrityDB(t)
	if err := db.Exec("DROP INDEX uk_sys_role_role_key").Error; err != nil {
		t.Fatal(err)
	}
	roles := []models.SysRole{{RoleId: 1, RoleKey: "one"}, {RoleId: 2, RoleKey: "two"}}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatal(err)
	}
	cb := integrityEnforcer(t, db)
	for _, key := range []string{"one", "two"} {
		if _, err := cb.AddPolicy(key, "/"+key, "GET"); err != nil {
			t.Fatal(err)
		}
	}
	s := &SysRole{}
	s.Orm = db
	if err := s.Update(&dto.SysRoleUpdateReq{RoleId: 1, RoleKey: "two"}, cb); err == nil {
		t.Fatal("rename to another active role key succeeded")
	}
	for _, key := range []string{"one", "two"} {
		allowed, err := cb.Enforce(key, "/"+key, "GET")
		if err != nil || !allowed {
			t.Fatalf("policy %s lost: allowed=%v err=%v", key, allowed, err)
		}
	}
	var got models.SysRole
	if err := db.First(&got, 1).Error; err != nil {
		t.Fatal(err)
	}
	if got.RoleKey != "one" {
		t.Fatalf("role key changed: %q", got.RoleKey)
	}
}

func TestRoleRejectsBlankKey(t *testing.T) {
	for _, name := range []string{"insert", "update"} {
		t.Run(name, func(t *testing.T) {
			db := integrityDB(t)
			cb := integrityEnforcer(t, db)
			s := &SysRole{}
			s.Orm = db
			if err := db.Create(&models.SysRole{RoleId: 1, RoleKey: "existing"}).Error; err != nil {
				t.Fatal(err)
			}
			var err error
			if name == "insert" {
				err = s.Insert(&dto.SysRoleInsertReq{RoleKey: " "}, cb)
			} else {
				err = s.Update(&dto.SysRoleUpdateReq{RoleId: 1, RoleKey: " "}, cb)
			}
			if err == nil {
				t.Fatal("blank role key accepted")
			}
		})
	}
}
