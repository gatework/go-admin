//go:build sqlite3

package database

import (
	"errors"
	"path/filepath"
	"testing"

	mycasbin "github.com/gatework/motor-core/casbin"

	"github.com/gatework/motor-core/sdk"
	coreconfig "github.com/gatework/motor-core/sdk/config"
)

func TestReloadKeepsDatabaseAndPolicySourceTogether(t *testing.T) {
	tenant := "reload-" + filepath.Base(t.TempDir())
	t.Cleanup(func() { setupMu.Lock(); delete(installedDatabases, tenant); setupMu.Unlock() })
	firstConfig := &coreconfig.Database{Driver: "sqlite3", Source: filepath.Join(t.TempDir(), "first.db"), MaxIdleConns: 1, MaxOpenConns: 2}
	if err := setupSimpleDatabase(tenant, firstConfig); err != nil {
		t.Fatal(err)
	}
	first := sdk.Runtime.DB(tenant)
	pool, _ := first.DB()
	defer pool.Close()
	enforcer := sdk.Runtime.Casbin(tenant)
	if err := setupSimpleDatabase(tenant, firstConfig); err != nil {
		t.Fatal(err)
	}
	if sdk.Runtime.DB(tenant) != first {
		t.Fatal("unchanged reload replaced live pool")
	}
	secondConfig := *firstConfig
	secondConfig.Source = filepath.Join(t.TempDir(), "second.db")
	if err := setupSimpleDatabase(tenant, &secondConfig); err == nil {
		t.Fatal("changed database accepted without restart")
	}
	if sdk.Runtime.DB(tenant) != first || sdk.Runtime.Casbin(tenant) != enforcer {
		t.Fatal("database configuration changed without restarting dependent resources")
	}
	if err := pool.Ping(); err != nil {
		t.Fatal(err)
	}
}

func setupSimpleDatabase(host string, c *coreconfig.Database) error {
	setupMu.Lock()
	defer setupMu.Unlock()
	if previous, ok := installedDatabases[host]; ok {
		if previous != databaseFingerprint(c) {
			return errors.New("database: configuration changed; restart required")
		}
		return nil
	}
	db, err := openDatabase(c, "warn")
	if err != nil {
		return err
	}
	sdk.Runtime.SetDB(host, db)
	sdk.Runtime.SetCasbin(host, mycasbin.Setup(db, host))
	installedDatabases[host] = databaseFingerprint(c)
	return nil
}
