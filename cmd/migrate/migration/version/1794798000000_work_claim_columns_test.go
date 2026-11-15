package version

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// A database migrated before the rename has lease_token/lease_until on every
// job table; one created afterwards already has claim_token/claim_until,
// because the earlier migrations build these tables from the runtime models.
// Both must end with the claim columns and indexes; running twice is harmless.
func TestWorkClaimColumnsAreRenamedOnceWhereverTheyExist(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range workClaimTables {
		columns := "id INTEGER PRIMARY KEY, lease_until DATETIME"
		if table.hasToken {
			columns += ", lease_token TEXT"
		}
		if err := db.Exec("CREATE TABLE " + table.name + " (" + columns + ")").Error; err != nil {
			t.Fatal(err)
		}
		if table.indexed {
			if err := db.Exec("CREATE INDEX idx_" + table.name + "_lease_until ON " + table.name + " (lease_until)").Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	for range 2 {
		if err := renameWorkClaimColumns(db); err != nil {
			t.Fatal(err)
		}
	}
	m := db.Migrator()
	for _, table := range workClaimTables {
		for _, gone := range []string{"lease_token", "lease_until"} {
			if m.HasColumn(table.name, gone) {
				t.Errorf("%s still has %s", table.name, gone)
			}
		}
		if !m.HasColumn(table.name, "claim_until") {
			t.Errorf("%s has no claim_until", table.name)
		}
		if table.hasToken != m.HasColumn(table.name, "claim_token") {
			t.Errorf("%s claim_token present = %v, want %v", table.name, !table.hasToken, table.hasToken)
		}
		if table.indexed && !m.HasIndex(table.name, "idx_"+table.name+"_claim_until") {
			t.Errorf("%s lost its claim_until index", table.name)
		}
	}
}
