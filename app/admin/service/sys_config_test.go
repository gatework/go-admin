package service

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/go-admin-team/go-admin-core/v2/logger"
	"gorm.io/gorm"

	"go-admin/app/admin/models"
	"go-admin/app/admin/service/dto"
)

func configDictDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := integrityDB(t)
	if err := db.AutoMigrate(&models.SysConfig{}, &models.SysDictData{}); err != nil {
		t.Fatal(err)
	}
	return db
}

type configDictUpdateCase struct {
	table  string
	model  any
	update func(*gorm.DB, int) error
}

func checkConfigDictUpdateBoundaries(t *testing.T, tc configDictUpdateCase) {
	t.Helper()
	for _, state := range []string{"missing", "read failure", "deleted after read", "soft deleted after read"} {
		t.Run(state, func(t *testing.T) {
			db := configDictDB(t)
			if tc.table == "sys_dict_data" {
				if err := db.Create(&models.SysDictType{DictType: "one"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if state != "missing" {
				if err := db.Create(tc.model).Error; err != nil {
					t.Fatal(err)
				}
			}
			failure := errors.New("injected read failure")
			if state == "read failure" {
				if err := db.Callback().Query().Before("gorm:query").Register("evolution:fail_read", func(tx *gorm.DB) {
					if tx.Statement.Table == tc.table {
						tx.AddError(failure)
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			insideTransaction := false
			if state == "deleted after read" || state == "soft deleted after read" {
				once := false
				if err := db.Callback().Query().After("gorm:query").Register("evolution:delete_after_read", func(tx *gorm.DB) {
					if tx.Statement.Table == tc.table && !once {
						once = true
						_, insideTransaction = tx.Statement.ConnPool.(gorm.TxCommitter)
						deleter := tx.Session(&gorm.Session{NewDB: true})
						if state == "deleted after read" {
							deleter = deleter.Unscoped()
						}
						if err := deleter.Delete(tc.model).Error; err != nil {
							tx.AddError(err)
						}
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			err := tc.update(db, 1)
			if err == nil {
				t.Fatal("update succeeded without a readable existing row")
			}
			if state == "read failure" && !errors.Is(err, failure) {
				t.Fatalf("read failure hidden: %v", err)
			}
			if err := db.Callback().Query().Remove("evolution:fail_read"); err != nil {
				t.Fatal(err)
			}
			var count int64
			if err := db.Table(tc.table).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			want := int64(0)
			if state == "read failure" || state == "soft deleted after read" || insideTransaction {
				want = 1
			}
			if count != want {
				t.Fatalf("update inserted/recreated rows: count=%d want=%d", count, want)
			}
		})
	}
}

func TestConfigUpdateBoundaries(t *testing.T) {
	row := &models.SysConfig{}
	row.Id = 1
	row.ConfigKey = "one"
	checkConfigDictUpdateBoundaries(t, configDictUpdateCase{table: "sys_config", model: row, update: func(db *gorm.DB, id int) error {
		s := &SysConfig{}
		s.Orm = db
		s.Log = logger.NewHelper(logger.DefaultLogger)
		return s.Update(&dto.SysConfigControl{Id: id, ConfigKey: "one", ConfigValue: "changed"})
	}})
}

func TestConfigUpdatePreservesCreatorAndWritesZeroValues(t *testing.T) {
	db := configDictDB(t)
	row := models.SysConfig{ConfigKey: "one", ConfigValue: "old", Remark: "old"}
	row.Id = 1
	row.CreateBy = 10
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	s := &SysConfig{}
	s.Orm = db
	s.Log = logger.NewHelper(logger.DefaultLogger)
	req := &dto.SysConfigControl{Id: 1, ConfigKey: "one"}
	req.CreateBy = 999
	req.UpdateBy = 20
	if err := s.Update(req); err != nil {
		t.Fatal(err)
	}
	var got models.SysConfig
	if err := db.First(&got, 1).Error; err != nil {
		t.Fatal(err)
	}
	if got.ConfigValue != "" || got.Remark != "" || got.CreateBy != 10 || got.UpdateBy != 20 {
		t.Fatalf("update fields = %+v", got)
	}
}

func TestConfigSetBatchRollsBackAndKeepsMissingKeySemantics(t *testing.T) {
	for _, method := range []string{"SetSysConfig", "UpdateForSet"} {
		for _, failure := range []string{"missing", "read", "write"} {
			t.Run(method+"/"+failure, func(t *testing.T) {
				db := configDictDB(t)
				rows := []models.SysConfig{{ConfigKey: "one", ConfigValue: "old"}, {ConfigKey: "two", ConfigValue: "old"}}
				if err := db.Create(&rows).Error; err != nil {
					t.Fatal(err)
				}
				injected := errors.New("injected second setting failure")
				if failure == "read" {
					n := 0
					if err := db.Callback().Query().Before("gorm:query").Register("evolution:setting_read", func(tx *gorm.DB) {
						if tx.Statement.Table == "sys_config" {
							n++
							if n == 2 {
								tx.AddError(injected)
							}
						}
					}); err != nil {
						t.Fatal(err)
					}
				}
				if failure == "write" {
					n := 0
					if err := db.Callback().Update().Before("gorm:update").Register("evolution:setting_write", func(tx *gorm.DB) {
						if tx.Statement.Table == "sys_config" {
							n++
							if n == 2 {
								tx.AddError(injected)
							}
						}
					}); err != nil {
						t.Fatal(err)
					}
				}
				req := []dto.GetSetSysConfigReq{{ConfigKey: "one", ConfigValue: "new"}, {ConfigKey: "two", ConfigValue: "new"}}
				if failure == "missing" {
					req[1].ConfigKey = "missing"
				}
				s := &SysConfig{}
				s.Orm = db
				s.Log = logger.NewHelper(logger.DefaultLogger)
				var err error
				if method == "SetSysConfig" {
					err = s.SetSysConfig(&req)
				} else {
					err = s.UpdateForSet(&req)
				}
				skip := method == "SetSysConfig" && failure == "missing"
				if skip && err != nil {
					t.Fatal(err)
				}
				if !skip && err == nil {
					t.Fatal("setting batch hid failure")
				}
				if failure != "missing" && !errors.Is(err, injected) {
					t.Fatalf("injected error lost: %v", err)
				}
				var got models.SysConfig
				if err := db.First(&got, rows[0].Id).Error; err != nil {
					t.Fatal(err)
				}
				want := "old"
				if skip {
					want = "new"
				}
				if got.ConfigValue != want {
					t.Fatalf("partial batch persisted: %q want %q", got.ConfigValue, want)
				}
				var count int64
				if err := db.Model(&models.SysConfig{}).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if count != 2 {
					t.Fatalf("set unexpectedly inserted key: count=%d", count)
				}
			})
		}
	}
}

func TestConfigGetWithKeyExcludesDeletedRows(t *testing.T) {
	db := configDictDB(t)
	row := models.SysConfig{ConfigKey: "deleted", ConfigValue: "hidden"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&row).Error; err != nil {
		t.Fatal(err)
	}
	s := &SysConfig{}
	s.Orm = db
	s.Log = logger.NewHelper(logger.DefaultLogger)
	var got dto.GetSysConfigByKEYForServiceResp
	if err := s.GetWithKey(&dto.SysConfigByKeyReq{ConfigKey: "deleted"}, &got); err == nil {
		t.Fatalf("deleted config returned: %+v", got)
	}
}

func TestConfigDictInsertStoresCreatorAndReturnsID(t *testing.T) {
	for _, name := range []string{"config", "dict data", "dict type"} {
		t.Run(name, func(t *testing.T) {
			db := configDictDB(t)
			var id int
			var creator int
			switch name {
			case "config":
				s := &SysConfig{}
				s.Orm = db
				s.Log = logger.NewHelper(logger.DefaultLogger)
				req := &dto.SysConfigControl{ConfigKey: "new"}
				req.CreateBy = 42
				if err := s.Insert(req); err != nil {
					t.Fatal(err)
				}
				id = req.Id
				var row models.SysConfig
				if err := db.First(&row).Error; err != nil {
					t.Fatal(err)
				}
				creator = row.CreateBy
			case "dict data":
				if err := db.Create(&models.SysDictType{DictType: "new"}).Error; err != nil {
					t.Fatal(err)
				}
				s := &SysDictData{}
				s.Orm = db
				s.Log = logger.NewHelper(logger.DefaultLogger)
				req := &dto.SysDictDataInsertReq{DictType: "new"}
				req.CreateBy = 42
				if err := s.Insert(req); err != nil {
					t.Fatal(err)
				}
				id = req.Id
				var row models.SysDictData
				if err := db.First(&row).Error; err != nil {
					t.Fatal(err)
				}
				creator = row.CreateBy
			case "dict type":
				s := &SysDictType{}
				s.Orm = db
				s.Log = logger.NewHelper(logger.DefaultLogger)
				req := &dto.SysDictTypeInsertReq{DictType: "new"}
				req.CreateBy = 42
				if err := s.Insert(req); err != nil {
					t.Fatal(err)
				}
				id = req.Id
				var row models.SysDictType
				if err := db.First(&row).Error; err != nil {
					t.Fatal(err)
				}
				creator = row.CreateBy
			}
			if id <= 0 || creator != 42 {
				t.Fatalf("insert returned id=%d persisted creator=%d", id, creator)
			}
		})
	}
}

func TestConfigSetReturnsCommitFailure(t *testing.T) {
	if os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN") == "" {
		t.Skip("PostgreSQL required for deferred commit failure")
	}
	for _, method := range []string{"SetSysConfig", "UpdateForSet"} {
		t.Run(method, func(t *testing.T) {
			db := configDictDB(t)
			row := models.SysConfig{ConfigKey: "one", ConfigValue: "original"}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			for _, sql := range []string{`CREATE FUNCTION reject_config_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected config commit failure'; END $$`, `CREATE CONSTRAINT TRIGGER reject_config_commit AFTER UPDATE ON sys_config DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_config_commit()`} {
				if err := db.Exec(sql).Error; err != nil {
					t.Fatal(err)
				}
			}
			req := []dto.GetSetSysConfigReq{{ConfigKey: "one", ConfigValue: "changed"}}
			s := &SysConfig{}
			s.Orm = db
			s.Log = logger.NewHelper(logger.DefaultLogger)
			var err error
			if method == "SetSysConfig" {
				err = s.SetSysConfig(&req)
			} else {
				err = s.UpdateForSet(&req)
			}
			if err == nil || !strings.Contains(err.Error(), "injected config commit failure") {
				t.Fatalf("commit error=%v", err)
			}
			var got models.SysConfig
			if err := db.First(&got, row.Id).Error; err != nil {
				t.Fatal(err)
			}
			if got.ConfigValue != "original" {
				t.Fatalf("failed commit persisted value %q", got.ConfigValue)
			}
		})
	}
}
