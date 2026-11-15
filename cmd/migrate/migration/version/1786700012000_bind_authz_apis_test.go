package version

import (
	"testing"

	"github.com/gatework/motor/app/admin/models"
)

const (
	roleUpdateMenuID = 226
	jobEditMenuID    = 463
	jobRemoveMenuID  = 464
)

func TestBindAuthzAPIsGrantsWhatARoleAlreadyHolds(t *testing.T) {
	db := openGenMenuDB(t)
	for _, m := range []models.SysMenu{
		{MenuID: roleUpdateMenuID, MenuName: "修改角色", Permission: "admin:sysRole:update", MenuType: "F"},
		{MenuID: jobEditMenuID, MenuName: "修改定时任务", Permission: "job:sysJob:edit", MenuType: "F"},
		{MenuID: jobRemoveMenuID, MenuName: "删除定时任务", Permission: "job:sysJob:remove", MenuType: "F"},
	} {
		if err := db.Create(&m).Error; err != nil {
			t.Fatal(err)
		}
	}
	addRole(t, db, 2, "roleAdmin", roleUpdateMenuID)
	addRole(t, db, 3, "jobEditor", jobEditMenuID)
	addRole(t, db, 4, "jobRemover", jobRemoveMenuID)
	addRole(t, db, 5, "clerk")

	if err := bindAuthzAPIs(db); err != nil {
		t.Fatal(err)
	}

	for role, want := range map[string][]string{
		"roleAdmin":  {"PUT /api/v1/role-status"},
		"jobEditor":  {"GET /api/v1/job/start/:id"},
		"jobRemover": {"GET /api/v1/job/remove/:id"},
		"clerk":      nil,
	} {
		if got := policies(t, db, role); !equal(got, want) {
			t.Errorf("%s holds %v, want %v", role, got, want)
		}
	}
}

func TestBindAuthzAPIsReusesSeededRowAndIsIdempotent(t *testing.T) {
	db := openGenMenuDB(t)
	if err := db.Create(&models.SysMenu{MenuID: roleUpdateMenuID, Permission: "admin:sysRole:update", MenuType: "F"}).Error; err != nil {
		t.Fatal(err)
	}
	seeded := models.SysAPI{ID: 153, Path: "/api/v1/role-status", Action: "PUT"}
	if err := db.Create(&seeded).Error; err != nil {
		t.Fatal(err)
	}
	addRole(t, db, 2, "roleAdmin", roleUpdateMenuID)

	for i := range 2 {
		if err := bindAuthzAPIs(db); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}

	var n int64
	db.Model(&models.SysAPI{}).Where("path = ?", seeded.Path).Count(&n)
	if n != 1 {
		t.Errorf("%d rows for %s, want the seeded one only", n, seeded.Path)
	}
	if got := boundAPIs(t, db, roleUpdateMenuID); got != 1 {
		t.Errorf("menu bound to %d APIs after two runs, want 1", got)
	}
	if got := policies(t, db, "roleAdmin"); len(got) != 1 {
		t.Errorf("roleAdmin holds %v after two runs, want one policy", got)
	}
}

func TestBindAuthzAPIsSkipsMenusThatDoNotExist(t *testing.T) {
	db := openGenMenuDB(t)
	if err := bindAuthzAPIs(db); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(&models.SysAPI{}).Count(&n)
	if n != 0 {
		t.Errorf("%d APIs created with no menu to bind, want 0", n)
	}
}
