package models

import "testing"

// Deleting a generator table must take its column definitions with it. The
// delete used to touch sys_tables only, so every removed table left its
// sys_columns rows behind.
func TestBatchDeleteRemovesTablesAndTheirColumns(t *testing.T) {
	db := openAtomicDB(t)
	removed := SysTables{TBName: "sys_widget", PackageName: "biz", BusinessName: "widget"}
	kept := SysTables{TBName: "sys_gadget", PackageName: "biz", BusinessName: "gadget"}
	for _, table := range []*SysTables{&removed, &kept} {
		if err := db.Create(table).Error; err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"id", "name"} {
			if err := db.Create(&SysColumns{TableID: table.TableID, ColumnName: name}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}

	var e SysTables
	if _, err := e.BatchDelete(db, []int{removed.TableID}); err != nil {
		t.Fatal(err)
	}

	count := func(model any, tableID int) int64 {
		t.Helper()
		var n int64
		if err := db.Unscoped().Model(model).Where("table_id = ?", tableID).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(&SysTables{}, removed.TableID); n != 0 {
		t.Fatalf("table row left behind: %d", n)
	}
	if n := count(&SysColumns{}, removed.TableID); n != 0 {
		t.Fatalf("orphaned column rows: %d", n)
	}
	if count(&SysTables{}, kept.TableID) != 1 || count(&SysColumns{}, kept.TableID) != 2 {
		t.Fatal("delete reached a table it was not asked to remove")
	}
}
