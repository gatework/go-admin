package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"

	"go-admin/app/admin/service/dto"
)

// Generate SQL with each registered dialect. SQLite supplies only the transaction
// connection: DryRun prevents executing another dialect's statements against it.
func TestAdminMutationLockSQL(t *testing.T) {
	for _, dialect := range []string{"sqlserver", "postgres", "mysql", "sqlite"} {
		for _, operation := range []struct {
			name, table string
			run         func(*gorm.DB) error
		}{
			{"role update", "sys_role", func(db *gorm.DB) error {
				s := &SysRole{}
				s.Orm = db
				return s.Update(&dto.SysRoleUpdateReq{RoleId: 7, RoleKey: "operator"}, nil)
			}},
			{"role remove", "sys_role", func(db *gorm.DB) error {
				s := &SysRole{}
				s.Orm = db
				return s.Remove(&dto.SysRoleDeleteReq{Ids: []int{7, 8}}, nil)
			}},
			{"role scope", "sys_role", func(db *gorm.DB) error {
				s := &SysRole{}
				s.Orm = db
				return s.UpdateDataScope(&dto.RoleDataScopeReq{RoleId: 7, DataScope: "2"}).Error
			}},
			{"dept tree", "sys_dept", lockDeptTree},
			{"menu tree", "sys_menu", lockMenuTree},
		} {
			t.Run(dialect+"/"+operation.name, func(t *testing.T) {
				base, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
				if err != nil {
					t.Fatal(err)
				}
				pool, err := base.DB()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { pool.Close() })
				db := base.Session(&gorm.Session{DryRun: true})
				var dial gorm.Dialector
				switch dialect {
				case "sqlserver":
					dial = sqlserver.New(sqlserver.Config{Conn: pool})
				case "postgres":
					dial = postgres.New(postgres.Config{Conn: pool})
				case "mysql":
					dial = mysql.New(mysql.Config{Conn: pool, SkipInitializeWithVersion: true})
				}
				if dial != nil {
					db, err = gorm.Open(dial, &gorm.Config{DryRun: true, DisableAutomaticPing: true})
					if err != nil {
						t.Fatal(err)
					}
				}
				stopped := errors.New("stop after generated locking query")
				var statement string
				if err := db.Callback().Query().After("gorm:query").Register("locking:capture", func(tx *gorm.DB) { statement = tx.Statement.SQL.String(); tx.AddError(stopped) }); err != nil {
					t.Fatal(err)
				}
				if err := operation.run(db); err == nil {
					t.Fatal("mutation did not stop at captured query")
				}
				if statement == "" {
					t.Fatal("no locking query generated")
				}
				if !strings.Contains(statement, operation.table) || !strings.Contains(statement, "deleted_at") {
					t.Fatalf("locking query lost model/soft-delete scope: %s", statement)
				}
				switch dialect {
				case "sqlserver":
					if strings.Contains(statement, "FOR UPDATE") || !strings.Contains(statement, `FROM "`+operation.table+`" WITH (UPDLOCK, HOLDLOCK)`) {
						t.Fatalf("SQL Server must lock through table hints: %s", statement)
					}
				case "postgres", "mysql":
					if !strings.Contains(statement, "FOR UPDATE") || strings.Contains(statement, "UPDLOCK") {
						t.Fatalf("row lock missing or wrong dialect: %s", statement)
					}
				case "sqlite":
					if strings.Contains(statement, "FOR UPDATE") || strings.Contains(statement, "UPDLOCK") {
						t.Fatalf("SQLite must retain its native transaction locking: %s", statement)
					}
				}
				t.Log(statement)
			})
		}
	}
}
