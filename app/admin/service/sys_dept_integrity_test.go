package service

import (
	"errors"
	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"go-admin/common/actions"
	"os"
	"testing"

	"go-admin/app/admin/models"
	"go-admin/app/admin/service/dto"
	"gorm.io/gorm"
)

func TestDeptRejectsMissingUpdateAndInvalidParent(t *testing.T) {
	for _, tc := range []struct {
		name       string
		id, parent int
	}{{"missing", 999, 0}, {"self", 1, 1}, {"descendant", 1, 2}, {"missing parent", 1, 999}} {
		t.Run(tc.name, func(t *testing.T) {
			db := integrityDB(t)
			depts := []models.SysDept{{DeptId: 1, DeptPath: "/0/1/"}, {DeptId: 2, ParentId: 1, DeptPath: "/0/1/2/"}}
			if err := db.Create(&depts).Error; err != nil {
				t.Fatal(err)
			}
			s := &SysDept{}
			s.Orm = db
			if err := s.Update(&dto.SysDeptUpdateReq{DeptId: tc.id, ParentId: tc.parent}); err == nil {
				t.Fatal("invalid update succeeded")
			}
			var got models.SysDept
			if err := db.First(&got, 1).Error; err != nil {
				t.Fatal(err)
			}
			if got.ParentId != 0 || got.DeptPath != "/0/1/" {
				t.Fatalf("failed update changed root: %+v", got)
			}
		})
	}
}

func TestDeptInsertMissingParentDoesNotCreate(t *testing.T) {
	db := integrityDB(t)
	s := &SysDept{}
	s.Orm = db
	if err := s.Insert(&dto.SysDeptInsertReq{ParentId: 999, DeptName: "orphan"}); err == nil {
		t.Fatal("insert with missing parent succeeded")
	}
	var count int64
	if err := db.Model(&models.SysDept{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("created %d orphan departments", count)
	}
}

func TestDeptMoveRollsBackIfDescendantWriteFails(t *testing.T) {
	db := integrityDB(t)
	depts := []models.SysDept{{DeptId: 1, DeptPath: "/0/1/"}, {DeptId: 2, DeptPath: "/0/2/"}, {DeptId: 3, ParentId: 1, DeptPath: "/0/1/3/"}, {DeptId: 4, ParentId: 3, DeptPath: "/0/1/3/4/"}}
	if err := db.Create(&depts).Error; err != nil {
		t.Fatal(err)
	}
	injected := errors.New("descendant write failed")
	if err := db.Callback().Update().Before("gorm:update").Register("integrity:fail_descendant", func(tx *gorm.DB) {
		if d, ok := tx.Statement.Model.(*models.SysDept); ok && d.DeptId == 4 {
			tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	s := &SysDept{}
	s.Orm = db
	if err := s.Update(&dto.SysDeptUpdateReq{DeptId: 3, ParentId: 2}); !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected failure", err)
	}
	for _, want := range depts {
		var got models.SysDept
		if err := db.First(&got, want.DeptId).Error; err != nil {
			t.Fatal(err)
		}
		if got.ParentId != want.ParentId || got.DeptPath != want.DeptPath {
			t.Fatalf("partial move persisted: got %+v, want %+v", got, want)
		}
	}
}

func TestDeptMoveDescendantPath(t *testing.T) {
	db := integrityDB(t)
	depts := []models.SysDept{{DeptId: 1, DeptName: "A", DeptPath: "/0/1/"}, {DeptId: 2, DeptName: "B", DeptPath: "/0/2/"}, {DeptId: 3, ParentId: 1, DeptName: "C", DeptPath: "/0/1/3/"}, {DeptId: 4, ParentId: 3, DeptName: "D", DeptPath: "/0/1/3/4/"}}
	if err := db.Create(&depts).Error; err != nil {
		t.Fatal(err)
	}
	s := &SysDept{}
	s.Orm = db
	if err := s.Update(&dto.SysDeptUpdateReq{DeptId: 3, ParentId: 2, DeptName: "C", Status: 2}); err != nil {
		t.Fatal(err)
	}
	var child models.SysDept
	if err := db.First(&child, 4).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.SysUser{}); err != nil {
		t.Fatal(err)
	}
	creator := models.SysUser{UserId: 10, DeptId: 4}
	if err := db.Create(&creator).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE audit_resources (id INTEGER PRIMARY KEY, create_by INTEGER)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO audit_resources VALUES (1,10)").Error; err != nil {
		t.Fatal(err)
	}
	previousDP := config.ApplicationConfig.EnableDP
	config.ApplicationConfig.EnableDP = true
	defer func() { config.ApplicationConfig.EnableDP = previousDP }()
	var oldVisible, newVisible int64
	db.Table("audit_resources").Scopes(actions.Permission("audit_resources", &actions.DataPermission{DataScope: actions.DataScopeDeptTree, DeptId: 1})).Count(&oldVisible)
	db.Table("audit_resources").Scopes(actions.Permission("audit_resources", &actions.DataPermission{DataScope: actions.DataScopeDeptTree, DeptId: 2})).Count(&newVisible)
	if child.DeptPath != "/0/2/3/4/" {
		t.Fatalf("descendant path = %q, expected /0/2/3/4/; old parent sees %d resources, new parent sees %d", child.DeptPath, oldVisible, newVisible)
	}
}

func TestDeptCommitFailure(t *testing.T) {
	if os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN") == "" {
		t.Skip("PostgreSQL required for deferred constraint")
	}
	db := integrityDB(t)
	if err := db.Exec("ALTER TABLE sys_dept ADD CONSTRAINT audit_unique_dept_name UNIQUE(dept_name) DEFERRABLE INITIALLY DEFERRED").Error; err != nil {
		t.Fatal(err)
	}
	existing := models.SysDept{DeptName: "duplicate", DeptPath: "/0/1/"}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatal(err)
	}
	s := &SysDept{}
	s.Orm = db
	err := s.Insert(&dto.SysDeptInsertReq{DeptName: "duplicate", Status: 2})
	var count int64
	db.Model(&models.SysDept{}).Where("dept_name = ?", "duplicate").Count(&count)
	if err == nil {
		t.Fatalf("commit failed deferred uniqueness constraint but Insert returned nil; matching rows=%d", count)
	}
}
