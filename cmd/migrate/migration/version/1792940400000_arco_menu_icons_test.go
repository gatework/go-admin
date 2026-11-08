//go:build integration

package version

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"go-admin/app/admin/models"
	common "go-admin/common/models"
)

func TestArcoMenuIconsOnlyChangesBuiltInNavigation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "icons.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&models.SysMenu{}, &common.Migration{}); err != nil {
		t.Fatal(err)
	}
	rows := []models.SysMenu{
		{MenuId: 1, MenuName: "SysRoleManage", MenuType: "C", Title: "角色", Icon: "peoples", Path: "/roles"},
		{MenuId: 2, MenuName: "CustomReport", MenuType: "C", Icon: "star"},
		{MenuId: 3, MenuName: "SysUserManage", MenuType: "F", Icon: "check"},
	}
	for i := range rows {
		if err = db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = _1792940400000ArcoMenuIcons(db, "1792940400000"); err != nil {
		t.Fatal(err)
	}
	var got []models.SysMenu
	if err = db.Order("menu_id").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Icon != "safe" || got[0].Title != "角色" || got[0].Path != "/roles" || got[1].Icon != "star" || got[2].Icon != "check" {
		t.Fatalf("unexpected menu changes: %#v", got)
	}
	var count int64
	if err = db.Model(&common.Migration{}).Where("version = ?", "1792940400000").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("migration completion: count=%d err=%v", count, err)
	}
}
