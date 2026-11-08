package version

import (
	"runtime"

	adminmodels "go-admin/app/admin/models"
	"go-admin/cmd/migrate/migration"
	common "go-admin/common/models"
	"gorm.io/gorm"
)

func init() {
	_, fileName, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(fileName), _1792992300000NetworkInventoryEditRead)
}

func _1792992300000NetworkInventoryEditRead(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := bindNetworkInventoryEditRead(tx); err != nil {
			return err
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}

// Editing starts by loading the current record/version. Grant that dependency
// with edit itself; an independently hidden view action must not break editing.
func bindNetworkInventoryEditRead(tx *gorm.DB) error {
	var menu adminmodels.SysMenu
	if err := tx.Where("app_code = ? AND seed_code = ?", "network", "device-edit").First(&menu).Error; err != nil {
		return err
	}
	var endpoint adminmodels.SysApi
	if err := tx.Where("app_code = ? AND path = ? AND action = ?", "network", "/api/v1/network-device/:id", "GET").First(&endpoint).Error; err != nil {
		return err
	}
	if err := tx.Model(&menu).Association("SysApi").Append(&endpoint); err != nil {
		return err
	}
	var roles []adminmodels.SysRole
	if err := tx.Model(&adminmodels.SysRole{}).Joins("JOIN sys_role_menu ON sys_role_menu.role_id = sys_role.role_id").Where("sys_role_menu.menu_id = ?", menu.MenuId).Find(&roles).Error; err != nil {
		return err
	}
	for _, role := range roles {
		if err := tx.Exec(`INSERT INTO casbin_rule (ptype, v0, v1, v2, v3, v4, v5)
			SELECT 'p', ?, ?, 'GET', '', '', '' WHERE NOT EXISTS
			(SELECT 1 FROM casbin_rule WHERE ptype = 'p' AND v0 = ? AND v1 = ? AND v2 = 'GET')`,
			role.RoleKey, endpoint.Path, role.RoleKey, endpoint.Path).Error; err != nil {
			return err
		}
	}
	return nil
}
