package health

import (
	"context"
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/gatework/motor-core/sdk"
)

func TestNamedTenantDatabaseCanBecomeReady(t *testing.T) {
	freshRuntime(t)
	var dial = sqlite.Open(":memory:")
	if dsn := os.Getenv("MOTOR_TEST_POSTGRES_DSN"); dsn != "" {
		dial = postgres.Open(dsn)
	}
	db, err := gorm.Open(dial, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	sdk.Runtime.SetDB("tenant.example", db)
	if err = sqlDB.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = pingDB(context.Background()); err != nil {
		t.Fatalf("reachable configured tenant rejected by readiness: %v (default tenant=%q)", err, sdk.Runtime.DefaultTenant())
	}
}
