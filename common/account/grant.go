package account

import (
	"context"

	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk/contract/actions"
)

// StatusEnabled is the status of an enabled user or role in sys_user and
// sys_role.
const StatusEnabled = "2"

// Grant is what a user may do now: their enabled user row and enabled role,
// read fresh from the database. Background work that acts for a user long
// after the request that started it re-reads its grant before acting, so a
// user disabled or a role narrowed meanwhile stops the work.
type Grant struct {
	UserID    int
	DeptID    int
	RoleID    int
	RoleKey   string
	DataScope string
}

// CurrentGrant reads userID's grant. ok is false when the user does not
// exist, is deleted or disabled, or has a deleted or disabled role.
func CurrentGrant(ctx context.Context, db *gorm.DB, userID int) (g Grant, ok bool, err error) {
	var u User
	found := db.WithContext(ctx).Where("user_id = ? AND status = ? AND deleted_at = 0", userID, StatusEnabled).Limit(1).Find(&u)
	if found.Error != nil || found.RowsAffected == 0 {
		return g, false, found.Error
	}
	var r Role
	found = db.WithContext(ctx).Where("role_id = ? AND status = ? AND deleted_at = 0", u.RoleID, StatusEnabled).Limit(1).Find(&r)
	if found.Error != nil || found.RowsAffected == 0 {
		return g, false, found.Error
	}
	return Grant{UserID: u.UserID, DeptID: u.DeptID, RoleID: r.RoleID, RoleKey: r.RoleKey, DataScope: r.DataScope}, true, nil
}

// Permission is the grant's data permission, for scoping queries.
func (g Grant) Permission() *actions.DataPermission {
	return &actions.DataPermission{DataScope: g.DataScope, UserID: g.UserID, DeptID: g.DeptID, RoleID: g.RoleID}
}

// Holds reports whether the grant's role holds the permission key (a menu's
// "module:resource:action", such as "network:fabric:execute"). The
// administrator holds every permission.
func (g Grant) Holds(ctx context.Context, db *gorm.DB, permission string) (bool, error) {
	if IsAdmin(g.RoleKey) {
		return true, nil
	}
	var n int64
	err := db.WithContext(ctx).Table("sys_role_menu rm").
		Joins("JOIN sys_menu m ON m.menu_id = rm.menu_id").
		Where("rm.role_id = ? AND m.permission = ? AND m.deleted_at = 0", g.RoleID, permission).
		Count(&n).Error
	return n > 0, err
}
