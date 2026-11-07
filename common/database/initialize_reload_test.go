//go:build sqlite3

package database

import (
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	coreconfig "github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"path/filepath"
	"testing"
)

func TestReloadKeepsDatabaseAndPolicySourceTogether(t *testing.T) {
	tenant := "reload-" + filepath.Base(t.TempDir())
	t.Cleanup(func() { setupMu.Lock(); delete(installedDatabases, tenant); setupMu.Unlock() })
	firstConfig := &coreconfig.Database{Driver: "sqlite3", Source: filepath.Join(t.TempDir(), "first.db"), MaxIdleConns: 1, MaxOpenConns: 2}
	setupSimpleDatabase(tenant, firstConfig)
	first := sdk.Runtime.GetDbByTenant(tenant)
	pool, _ := first.DB()
	defer pool.Close()
	enforcer := sdk.Runtime.GetCasbinByTenant(tenant)
	setupSimpleDatabase(tenant, firstConfig)
	if sdk.Runtime.GetDbByTenant(tenant) != first {
		t.Fatal("unchanged reload replaced live pool")
	}
	secondConfig := *firstConfig
	secondConfig.Source = filepath.Join(t.TempDir(), "second.db")
	setupSimpleDatabase(tenant, &secondConfig)
	if sdk.Runtime.GetDbByTenant(tenant) != first || sdk.Runtime.GetCasbinByTenant(tenant) != enforcer {
		t.Fatal("database configuration changed without restarting dependent resources")
	}
	if err := pool.Ping(); err != nil {
		t.Fatal(err)
	}
}
