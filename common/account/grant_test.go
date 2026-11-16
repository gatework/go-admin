package account

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func grantDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&User{}, &Role{}); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"CREATE TABLE sys_menu (menu_id INTEGER PRIMARY KEY, permission TEXT, deleted_at INTEGER NOT NULL DEFAULT 0)",
		"CREATE TABLE sys_role_menu (role_id INTEGER, menu_id INTEGER)",
		"INSERT INTO sys_menu (menu_id, permission) VALUES (1, 'network:fabric:execute'), (2, 'network:fabric:view')",
		"INSERT INTO sys_menu (menu_id, permission, deleted_at) VALUES (3, 'network:fabric:retired', 5)",
		"INSERT INTO sys_role_menu (role_id, menu_id) VALUES (2, 1), (2, 3)",
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	roles := []Role{
		{RoleID: 1, RoleKey: AdminRoleKey, Status: StatusEnabled, DataScope: "1"},
		{RoleID: 2, RoleKey: "operator", Status: StatusEnabled, DataScope: "3"},
		{RoleID: 3, RoleKey: "locked", Status: "1", DataScope: "1"},
	}
	users := []User{
		{UserID: 7, Username: "admin", RoleID: 1, DeptID: 1, Status: StatusEnabled},
		{UserID: 8, Username: "operator", RoleID: 2, DeptID: 4, Status: StatusEnabled},
		{UserID: 9, Username: "disabled", RoleID: 2, Status: "1"},
		{UserID: 10, Username: "locked-role", RoleID: 3, Status: StatusEnabled},
		{UserID: 11, Username: "deleted", RoleID: 2, Status: StatusEnabled, DeletedAt: 5},
	}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func TestCurrentGrantReadsOnlyEnabledUsersWithEnabledRoles(t *testing.T) {
	db := grantDB(t)
	ctx := context.Background()
	g, ok, err := CurrentGrant(ctx, db, 8)
	if err != nil || !ok || g.RoleKey != "operator" || g.DeptID != 4 {
		t.Fatalf("operator: %+v %v %v", g, ok, err)
	}
	if p := g.Permission(); p.UserID != 8 || p.DeptID != 4 || p.RoleID != 2 || p.DataScope != "3" {
		t.Fatalf("data permission %+v", p)
	}
	for _, id := range []int{9, 10, 11, 404} {
		if _, ok, err := CurrentGrant(ctx, db, id); ok || err != nil {
			t.Errorf("user %d: ok %v, err %v; want no grant", id, ok, err)
		}
	}
}

func TestGrantHoldsOnlyLivePermissionsAndAdminHoldsAll(t *testing.T) {
	db := grantDB(t)
	ctx := context.Background()
	operator, _, _ := CurrentGrant(ctx, db, 8)
	admin, _, _ := CurrentGrant(ctx, db, 7)
	for _, tc := range []struct {
		g          Grant
		permission string
		want       bool
	}{
		{operator, "network:fabric:execute", true},
		{operator, "network:fabric:view", false},    // not granted to the role
		{operator, "network:fabric:retired", false}, // menu deleted
		{admin, "network:fabric:view", true},
	} {
		if got, err := tc.g.Holds(ctx, db, tc.permission); err != nil || got != tc.want {
			t.Errorf("%s holds %s = %v %v, want %v", tc.g.RoleKey, tc.permission, got, err, tc.want)
		}
	}
}
