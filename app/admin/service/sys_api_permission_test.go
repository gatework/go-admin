package service

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk/config"

	"github.com/gatework/motor/app/admin/models"
	"github.com/gatework/motor/app/admin/service/dto"
	"github.com/gatework/motor/common/actions"
	"github.com/gatework/motor/internal/testconfig"
)

// An update the data permission excludes has to be refused, and refused in a
// way that does not tell the caller whether the row exists. First reports both
// cases the same way - no rows - so the message has to come from there rather
// than from a RowsAffected check the error return has already skipped past.
func TestSysAPIUpdateRefusesARowOutsideTheDataPermission(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:sysapi-perm?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	if err := db.AutoMigrate(&models.SysAPI{}); err != nil {
		t.Skipf("automigrate: %v", err)
	}

	testconfig.Apply(t, func(settings *config.Config) {
		settings.
			Application.EnableDP = true
	})

	// Owned by user 1.
	row := models.SysAPI{Handle: "h", Title: "t", Path: "/api/v1/probe", Type: "BUS", Action: "GET",
		CreateBy: 1}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}

	e := &Route{DB: db, Logger: logger.NewHelper(logger.Default())}
	req := &dto.RouteUpdate{ID: row.ID, Title: "changed"}

	// User 2, scope 5: only rows they created.
	outsider := &actions.DataPermission{DataScope: "5", UserID: 2, DeptID: 1, RoleID: 2}
	err = e.Update(req, outsider)
	if err == nil {
		t.Fatal("the update was allowed on a row the data permission excludes")
	}
	if !strings.Contains(err.Error(), "无权更新该数据") {
		t.Errorf("refused with %q, want the permission message; a raw database error tells the "+
			"caller the row exists", err)
	}

	var after models.SysAPI
	if err := db.First(&after, row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if after.Title != "t" {
		t.Errorf("the row was modified: title is now %q", after.Title)
	}

	// The owner still gets through, so the scope is refusing rather than
	// everything failing.
	owner := &actions.DataPermission{DataScope: "5", UserID: 1, DeptID: 1, RoleID: 1}
	if err := e.Update(&dto.RouteUpdate{ID: row.ID, Title: "by owner"}, owner); err != nil {
		t.Fatalf("the owner could not update their own row: %v", err)
	}
}

func TestSysAPIUpdateRechecksExistenceAndPermissionAtWrite(t *testing.T) {
	for _, scenario := range []string{"delete", "ownership changes"} {
		t.Run(scenario, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			pool, _ := db.DB()
			t.Cleanup(func() { pool.Close() })
			if err := db.AutoMigrate(&models.SysAPI{}); err != nil {
				t.Fatal(err)
			}

			testconfig.Apply(t, func(settings *config.Config) {
				settings.
					Application.EnableDP = true
			})

			row := models.SysAPI{Title: "before", Path: "/probe", Action: "GET",
				CreateBy: 1}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			changed := false
			if err := db.Callback().Query().After("gorm:after_query").Register("change_after_read", func(tx *gorm.DB) {
				if changed || tx.Statement.Table != "sys_api" || tx.RowsAffected == 0 {
					return
				}
				changed = true
				next := tx.Session(&gorm.Session{NewDB: true})
				if scenario == "delete" {
					err = next.Unscoped().Delete(&models.SysAPI{}, row.ID).Error
				} else {
					err = next.Model(&models.SysAPI{}).Where("id = ?", row.ID).Update("create_by", 2).Error
				}
				if err != nil {
					t.Error(err)
				}
			}); err != nil {
				t.Fatal(err)
			}
			s := Route{DB: db, Logger: logger.NewHelper(logger.Default())}
			err = s.Update(&dto.RouteUpdate{ID: row.ID, Title: "must not write"}, &actions.DataPermission{DataScope: "5", UserID: 1})
			if err == nil {
				t.Fatal("write accepted after row disappeared or left permission scope")
			}
			if scenario == "delete" {
				var count int64
				db.Unscoped().Model(&models.SysAPI{}).Count(&count)
				if count != 0 {
					t.Fatal("deleted row resurrected")
				}
			} else {
				var after models.SysAPI
				if err := db.First(&after, row.ID).Error; err != nil {
					t.Fatal(err)
				}
				if after.CreateBy != 2 || after.Title != "before" {
					t.Fatalf("row changed after ownership transfer: %+v", after)
				}
			}
		})
	}
}
