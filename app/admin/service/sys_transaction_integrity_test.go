package service

import (
	"os"
	"strings"
	"testing"

	"go-admin/app/admin/models"
	"go-admin/app/admin/service/dto"
)

// PostgreSQL checks this constraint at COMMIT, after all writes have succeeded.
func TestAdminMutationsReturnCommitFailure(t *testing.T) {
	if os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN") == "" {
		t.Skip("set GO_ADMIN_TEST_POSTGRES_DSN for deferred PostgreSQL constraints")
	}
	for _, name := range []string{"dept insert", "dept update", "menu insert", "menu update", "role insert", "role update", "role remove", "role scope", "role status"} {
		t.Run(name, func(t *testing.T) {
			db := integrityDB(t)
			dept := models.SysDept{DeptId: 1, DeptName: "original", DeptPath: "/0/1/"}
			menu := models.SysMenu{MenuId: 1, MenuName: "original", Paths: "/0/1"}
			role := models.SysRole{RoleId: 1, RoleKey: "original", RoleName: "original", Status: "2", DataScope: "1"}
			for _, record := range []any{&dept, &menu, &role} {
				if err := db.Create(record).Error; err != nil {
					t.Fatal(err)
				}
			}
			cb := integrityEnforcer(t, db)
			if _, err := cb.AddPolicy("original", "/old", "GET"); err != nil {
				t.Fatal(err)
			}
			table := "sys_" + strings.Fields(name)[0]
			statements := []string{`CREATE FUNCTION reject_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected commit failure'; END $$`, "CREATE CONSTRAINT TRIGGER reject_commit AFTER INSERT OR UPDATE ON " + table + " DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_commit()"}
			for _, sql := range statements {
				if err := db.Exec(sql).Error; err != nil {
					t.Fatal(err)
				}
			}
			d := &SysDept{}
			d.Orm = db
			m := &SysMenu{}
			m.Orm = db
			r := &SysRole{}
			r.Orm = db
			var err error
			switch name {
			case "dept insert":
				err = d.Insert(&dto.SysDeptInsertReq{DeptId: 2, DeptName: "new"})
			case "dept update":
				err = d.Update(&dto.SysDeptUpdateReq{DeptId: 1, DeptName: "changed"})
			case "menu insert":
				err = m.Insert(&dto.SysMenuInsertReq{MenuId: 2, MenuName: "new"}).Error
			case "menu update":
				err = m.Update(&dto.SysMenuUpdateReq{MenuId: 1, MenuName: "changed"}).Error
			case "role insert":
				err = r.Insert(&dto.SysRoleInsertReq{RoleId: 2, RoleKey: "new"}, cb)
			case "role update":
				err = r.Update(&dto.SysRoleUpdateReq{RoleId: 1, RoleKey: "changed"}, cb)
			case "role remove":
				err = r.Remove(&dto.SysRoleDeleteReq{Ids: []int{1}}, cb)
			case "role scope":
				err = r.UpdateDataScope(&dto.RoleDataScopeReq{RoleId: 1, DataScope: "2", DeptIds: []int{1}}).Error
			case "role status":
				err = r.UpdateStatus(&dto.UpdateStatusReq{RoleId: 1, Status: "1"})
			}
			if err == nil || !strings.Contains(err.Error(), "injected commit failure") {
				t.Fatalf("commit error = %v", err)
			}
			var count int64
			if err := db.Table(table).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 1 {
				t.Fatalf("failed commit changed row count: %d", count)
			}
			var got models.SysRole
			if err := db.First(&got, 1).Error; err != nil {
				t.Fatal(err)
			}
			if got.RoleKey != "original" || got.DataScope != "1" || got.Status != "2" {
				t.Fatalf("failed commit changed role: %+v", got)
			}
			if err := cb.LoadPolicy(); err != nil {
				t.Fatal(err)
			}
			allowed, err := cb.Enforce("original", "/old", "GET")
			if err != nil || !allowed {
				t.Fatalf("failed commit changed policy: allowed=%v error=%v", allowed, err)
			}
		})
	}
}

func TestConcurrentHierarchyMovesCannotCreateCycle(t *testing.T) {
	if os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN") == "" {
		t.Skip("set GO_ADMIN_TEST_POSTGRES_DSN for concurrent row locking")
	}
	for _, kind := range []string{"dept", "menu"} {
		t.Run(kind, func(t *testing.T) {
			db := integrityDB(t)
			if kind == "dept" {
				rows := []models.SysDept{{DeptId: 1, DeptPath: "/0/1/"}, {DeptId: 2, DeptPath: "/0/2/"}}
				if err := db.Create(&rows).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				rows := []models.SysMenu{{MenuId: 1, Paths: "/0/1"}, {MenuId: 2, Paths: "/0/2"}}
				if err := db.Create(&rows).Error; err != nil {
					t.Fatal(err)
				}
			}
			results := make(chan error, 2)
			start := make(chan struct{})
			for id := 1; id <= 2; id++ {
				go func(id int) {
					<-start
					if kind == "dept" {
						s := &SysDept{}
						s.Orm = db
						results <- s.Update(&dto.SysDeptUpdateReq{DeptId: id, ParentId: 3 - id})
					} else {
						s := &SysMenu{}
						s.Orm = db
						results <- s.Update(&dto.SysMenuUpdateReq{MenuId: id, ParentId: 3 - id}).Error
					}
				}(id)
			}
			close(start)
			first, second := <-results, <-results
			if (first == nil) == (second == nil) {
				t.Fatalf("expected one valid move and one cycle rejection: %v, %v", first, second)
			}
			var roots int64
			if err := db.Table("sys_" + kind).Where("parent_id = 0").Count(&roots).Error; err != nil {
				t.Fatal(err)
			}
			if roots != 1 {
				t.Fatalf("hierarchy root count=%d, want 1", roots)
			}
		})
	}
}
