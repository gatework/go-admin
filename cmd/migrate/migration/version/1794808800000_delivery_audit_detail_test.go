package version

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDeliveryAuditDetailColumnsAreAddedOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE sys_delivery_audit (id INTEGER PRIMARY KEY, order_id INTEGER, action TEXT)").Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := addDeliveryAuditDetail(db); err != nil {
			t.Fatal(err)
		}
	}
	for _, column := range []string{"scripts_sha256", "result"} {
		if !db.Migrator().HasColumn(&deliveryAuditDetailRow{}, column) {
			t.Errorf("%s missing", column)
		}
	}
}
