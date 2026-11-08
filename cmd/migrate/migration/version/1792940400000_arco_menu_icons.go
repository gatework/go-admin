package version

import (
	"runtime"

	"gorm.io/gorm"

	"go-admin/app/admin/models"
	"go-admin/cmd/migrate/migration"
	common "go-admin/common/models"
)

func init() {
	_, fileName, _, _ := runtime.Caller(0)
	migration.Migrate.SetVersion(migration.GetFilename(fileName), _1792940400000ArcoMenuIcons)
}

// 直接按内置功能写入 Arco 名称，不维护旧 SVG 名称的运行时兼容层。
func _1792940400000ArcoMenuIcons(db *gorm.DB, version string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := setArcoMenuIcons(tx); err != nil {
			return err
		}
		return tx.Create(&common.Migration{Version: version}).Error
	})
}

func setArcoMenuIcons(tx *gorm.DB) error {
	icons := []struct{ name, icon string }{
		{"Admin", "apps"},
		{"Schedule", "calendar-clock"},
		{"SysTools", "tool"},
		{"Tools", "code-square"},
		{"Demo", "experiment"},
		{"SysApiManage", "link"},
		{"SysConfigSet", "settings"},
		{"SysUserManage", "user"},
		{"SysRoleManage", "safe"},
		{"SysMenuManage", "menu"},
		{"SysDeptManage", "branch"},
		{"SysPostManage", "idcard"},
		{"Dict", "book"},
		{"SysConfigManage", "storage"},
		{"Log", "folder"},
		{"SysDictDataManage", "unordered-list"},
		{"Swagger", "code-block"},
		{"Build", "interaction"},
		{"Gen", "code"},
		{"EditTable", "edit"},
		{"SysLoginLogManage", "history"},
		{"OperLog", "list"},
		{"ScheduleManage", "schedule"},
		{"JobLog", "file"},
		{"ServerMonitor", "desktop"},
		{"DemoProduct", "archive"},
	}
	for _, item := range icons {
		if err := tx.Model(&models.SysMenu{}).
			Where("menu_name = ? AND menu_type IN ? AND app_code = ?", item.name, []string{"M", "C"}, "").
			Update("icon", item.icon).Error; err != nil {
			return err
		}
	}
	return nil
}
