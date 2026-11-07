package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-admin-team/go-admin-core/v2/logger"
	"gorm.io/gorm"

	"go-admin/app/admin/models"
	"go-admin/app/admin/service/dto"
)

func TestDictTypeUpdateBoundaries(t *testing.T) {
	checkConfigDictUpdateBoundaries(t, configDictUpdateCase{table: "sys_dict_type", model: &models.SysDictType{ID: 1, DictType: "one"}, update: func(db *gorm.DB, id int) error {
		s := &SysDictType{}
		s.Orm = db
		s.Log = logger.NewHelper(logger.DefaultLogger)
		return s.Update(&dto.SysDictTypeUpdateReq{Id: id, DictType: "one", DictName: "changed"})
	}})
}

func TestDictTypeUpdatePreservesCreatorAndWritesZeroValues(t *testing.T) {
	db := configDictDB(t)
	row := models.SysDictType{ID: 1, DictType: "one", Status: 2, Remark: "old"}
	row.CreateBy = 10
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	s := &SysDictType{}
	s.Orm = db
	s.Log = logger.NewHelper(logger.DefaultLogger)
	req := &dto.SysDictTypeUpdateReq{Id: 1, DictType: "one"}
	req.CreateBy = 999
	req.UpdateBy = 20
	if err := s.Update(req); err != nil {
		t.Fatal(err)
	}
	var got models.SysDictType
	if err := db.First(&got, 1).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != 0 || got.Remark != "" || got.CreateBy != 10 || got.UpdateBy != 20 {
		t.Fatalf("update fields = %+v", got)
	}
}

func TestDictTypeRenameMovesOnlyActiveChildren(t *testing.T) {
	db := configDictDB(t)
	parent := models.SysDictType{ID: 1, DictType: "old"}
	if err := db.Create(&parent).Error; err != nil {
		t.Fatal(err)
	}
	children := []models.SysDictData{{DictCode: 1, DictType: "old", DictLabel: "active"}, {DictCode: 2, DictType: "old", DictLabel: "historical"}}
	if err := db.Create(&children).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&children[1]).Error; err != nil {
		t.Fatal(err)
	}
	s := &SysDictType{}
	s.Orm = db
	s.Log = logger.NewHelper(logger.DefaultLogger)
	if err := s.Update(&dto.SysDictTypeUpdateReq{Id: 1, DictType: "old-renamed"}); err != nil {
		t.Fatal(err)
	}
	var active, historical models.SysDictData
	if err := db.First(&active, 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Unscoped().First(&historical, 2).Error; err != nil {
		t.Fatal(err)
	}
	if active.DictType != "old-renamed" || historical.DictType != "old" || historical.DeletedAt == 0 {
		t.Fatalf("rename damaged child/history: active=%+v historical=%+v", active, historical)
	}
	data := &SysDictData{}
	data.Orm = db
	data.Log = logger.NewHelper(logger.DefaultLogger)
	var oldOptions []models.SysDictData
	if err := data.GetAll(&dto.SysDictDataGetPageReq{DictType: "old"}, &oldOptions); err != nil {
		t.Fatal(err)
	}
	if len(oldOptions) != 0 {
		t.Fatalf("old option key still returns %d rows", len(oldOptions))
	}
}

func TestDictTypeRenameRollbackAndDuplicateKey(t *testing.T) {
	for _, failure := range []string{"child write", "duplicate key"} {
		t.Run(failure, func(t *testing.T) {
			db := configDictDB(t)
			parents := []models.SysDictType{{ID: 1, DictType: "old"}}
			if failure == "duplicate key" {
				parents = append(parents, models.SysDictType{ID: 2, DictType: "new"})
			}
			if err := db.Create(&parents).Error; err != nil {
				t.Fatal(err)
			}
			child := models.SysDictData{DictCode: 1, DictType: "old"}
			if err := db.Create(&child).Error; err != nil {
				t.Fatal(err)
			}
			if failure == "child write" {
				if err := db.Callback().Update().Before("gorm:update").Register("evolution:fail_child_rename", func(tx *gorm.DB) {
					if tx.Statement.Table == "sys_dict_data" {
						tx.AddError(errors.New("injected child rename failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			s := &SysDictType{}
			s.Orm = db
			s.Log = logger.NewHelper(logger.DefaultLogger)
			if err := s.Update(&dto.SysDictTypeUpdateReq{Id: 1, DictType: "new"}); err == nil {
				t.Fatal("invalid rename succeeded")
			}
			var parent models.SysDictType
			if err := db.First(&parent, 1).Error; err != nil {
				t.Fatal(err)
			}
			var got models.SysDictData
			if err := db.First(&got, 1).Error; err != nil {
				t.Fatal(err)
			}
			if parent.DictType != "old" || got.DictType != "old" {
				t.Fatalf("partial rename persisted: parent=%+v child=%+v", parent, got)
			}
		})
	}
}

func TestDictTypeDeleteRefusesActiveChildrenAndRetainsHistory(t *testing.T) {
	db := configDictDB(t)
	parents := []models.SysDictType{{ID: 1, DictType: "one"}, {ID: 2, DictType: "two"}}
	if err := db.Create(&parents).Error; err != nil {
		t.Fatal(err)
	}
	child := models.SysDictData{DictCode: 1, DictType: "two"}
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	s := &SysDictType{}
	s.Orm = db
	s.Log = logger.NewHelper(logger.DefaultLogger)
	req := &dto.SysDictTypeDeleteReq{Ids: []int{1, 2}}
	if err := s.Remove(req); err == nil {
		t.Fatal("deleted dictionary type with active children")
	}
	var count int64
	if err := db.Model(&models.SysDictType{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatal("batch delete partially persisted")
	}
	if err := db.Delete(&child).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(req); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.SysDictType{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("empty types not deleted: count=%d", count)
	}
	var historical models.SysDictData
	if err := db.Unscoped().First(&historical, 1).Error; err != nil {
		t.Fatal(err)
	}
	if historical.DictType != "two" || historical.DeletedAt == 0 {
		t.Fatalf("history altered: %+v", historical)
	}
}

func TestDictionaryMutationsReturnCommitFailure(t *testing.T) {
	if os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN") == "" {
		t.Skip("PostgreSQL required for deferred commit failure")
	}
	for _, name := range []string{"rename", "delete type", "insert data", "update data"} {
		t.Run(name, func(t *testing.T) {
			db := configDictDB(t)
			if err := db.Create(&models.SysDictType{ID: 1, DictType: "old"}).Error; err != nil {
				t.Fatal(err)
			}
			if name != "delete type" {
				if err := db.Create(&models.SysDictData{DictCode: 1, DictType: "old", DictLabel: "original"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			table := "sys_dict_type"
			if strings.Contains(name, "data") {
				table = "sys_dict_data"
			}
			for _, sql := range []string{`CREATE FUNCTION reject_dict_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected dictionary commit failure'; END $$`, "CREATE CONSTRAINT TRIGGER reject_dict_commit AFTER INSERT OR UPDATE ON " + table + " DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION reject_dict_commit()"} {
				if err := db.Exec(sql).Error; err != nil {
					t.Fatal(err)
				}
			}
			parent := &SysDictType{}
			parent.Orm = db
			child := &SysDictData{}
			child.Orm = db
			var err error
			switch name {
			case "rename":
				err = parent.Update(&dto.SysDictTypeUpdateReq{Id: 1, DictType: "new"})
			case "delete type":
				err = parent.Remove(&dto.SysDictTypeDeleteReq{Ids: []int{1}})
			case "insert data":
				err = child.Insert(&dto.SysDictDataInsertReq{Id: 2, DictType: "old"})
			case "update data":
				err = child.Update(&dto.SysDictDataUpdateReq{Id: 1, DictType: "old", DictLabel: "changed"})
			}
			if err == nil || !strings.Contains(err.Error(), "injected dictionary commit failure") {
				t.Fatalf("commit error=%v", err)
			}
			var got models.SysDictType
			if err := db.First(&got, 1).Error; err != nil {
				t.Fatal(err)
			}
			if got.DictType != "old" {
				t.Fatalf("parent changed after failed commit: %+v", got)
			}
			var rows []models.SysDictData
			if err := db.Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			if name == "delete type" {
				if len(rows) != 0 {
					t.Fatal("unexpected child")
				}
			} else if len(rows) != 1 || rows[0].DictType != "old" || rows[0].DictLabel != "original" {
				t.Fatalf("children changed after failed commit: %+v", rows)
			}
		})
	}
}

func TestConcurrentDictionaryParentChangeRejectsLateChild(t *testing.T) {
	if os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN") == "" {
		t.Skip("PostgreSQL required for concurrent parent locks")
	}
	for _, action := range []string{"rename", "delete"} {
		t.Run(action, func(t *testing.T) {
			db := configDictDB(t)
			if err := db.Create(&models.SysDictType{ID: 1, DictType: "old"}).Error; err != nil {
				t.Fatal(err)
			}
			type requestKey struct{}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			locked := make(chan struct{})
			childStarted := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			t.Cleanup(func() { once.Do(func() { close(release) }) })
			if err := db.Callback().Query().After("gorm:query").Register("evolution:hold_parent", func(tx *gorm.DB) {
				if tx.Statement.Table == "sys_dict_type" && tx.Statement.Context.Value(requestKey{}) == "parent" {
					select {
					case <-locked:
					default:
						close(locked)
						<-release
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			if err := db.Callback().Query().Before("gorm:query").Register("evolution:child_started", func(tx *gorm.DB) {
				if tx.Statement.Table == "sys_dict_type" && tx.Statement.Context.Value(requestKey{}) == "child" {
					close(childStarted)
				}
			}); err != nil {
				t.Fatal(err)
			}
			parentResult := make(chan error, 1)
			childResult := make(chan error, 1)
			go func() {
				s := &SysDictType{}
				s.Orm = db.WithContext(context.WithValue(ctx, requestKey{}, "parent"))
				if action == "rename" {
					parentResult <- s.Update(&dto.SysDictTypeUpdateReq{Id: 1, DictType: "new"})
				} else {
					parentResult <- s.Remove(&dto.SysDictTypeDeleteReq{Ids: []int{1}})
				}
			}()
			select {
			case <-locked:
			case <-ctx.Done():
				t.Fatal("parent did not acquire dictionary lock")
			}
			go func() {
				s := &SysDictData{}
				s.Orm = db.WithContext(context.WithValue(ctx, requestKey{}, "child"))
				childResult <- s.Insert(&dto.SysDictDataInsertReq{DictType: "old"})
			}()
			select {
			case <-childStarted:
			case <-ctx.Done():
				t.Fatal("child did not attempt parent lookup")
			}
			once.Do(func() { close(release) })
			if err := <-parentResult; err != nil {
				t.Fatal(err)
			}
			if err := <-childResult; !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("late child error=%v, want missing old parent", err)
			}
			var count int64
			if err := db.Model(&models.SysDictData{}).Where("dict_type = ?", "old").Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatalf("orphan children=%d", count)
			}
		})
	}
}

func TestDictTypeRejectsBlankKey(t *testing.T) {
	for _, operation := range []string{"insert", "update"} {
		t.Run(operation, func(t *testing.T) {
			db := configDictDB(t)
			if err := db.Create(&models.SysDictType{ID: 1, DictType: "existing"}).Error; err != nil {
				t.Fatal(err)
			}
			s := &SysDictType{}
			s.Orm = db
			s.Log = logger.NewHelper(logger.DefaultLogger)
			var err error
			if operation == "insert" {
				err = s.Insert(&dto.SysDictTypeInsertReq{DictType: " "})
			} else {
				err = s.Update(&dto.SysDictTypeUpdateReq{Id: 1, DictType: " "})
			}
			if err == nil {
				t.Fatal("blank dictionary key accepted")
			}
		})
	}
}
