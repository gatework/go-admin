package health

import (
	"context"
	"github.com/glebarez/sqlite"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"testing"
)

func TestNamedTenantDatabaseCanBecomeReady(t *testing.T) {
	freshRuntime(t)
	var dial gorm.Dialector = sqlite.Open(":memory:")
	if dsn := os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN"); dsn != "" {
		dial = postgres.Open(dsn)
	}
	db, err := gorm.Open(dial, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	sdk.Runtime.SetDbByTenant("tenant.example", db)
	if err = sqlDB.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = pingDB(context.Background()); err != nil {
		t.Fatalf("reachable configured tenant rejected by readiness: %v (default tenant=%q)", err, sdk.Runtime.GetDefaultTenant())
	}
}
