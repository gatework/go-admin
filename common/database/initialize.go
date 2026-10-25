package database

import (
	"crypto/sha256"
	"encoding/json"
	"sync"
	"time"

	mycasbin "github.com/go-admin-team/go-admin-core/v2/casbin"
	log "github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk"
	toolsConfig "github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg"
	toolsDB "github.com/go-admin-team/go-admin-core/v2/tools/database"
	. "github.com/go-admin-team/go-admin-core/v2/tools/gorm/gormlog"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"go-admin/common/global"
)

// Database resources are process-lifetime resources: the scheduler and Casbin
// adapter retain them. Replacing only the registry would split their identity.
var setupMu sync.Mutex
var installedDatabases = map[string][32]byte{}

func databaseFingerprint(c *toolsConfig.Database) [32]byte {
	data, _ := json.Marshal(c)
	return sha256.Sum256(data)
}

// Setup accepts the first database configuration. Later connection changes
// require a restart, including tenant additions/removals and pool settings.
func Setup() {
	setupMu.Lock()
	defer setupMu.Unlock()
	if len(installedDatabases) != 0 {
		if len(installedDatabases) != len(toolsConfig.DatabasesConfig) {
			log.Error("database configuration changed; restart required")
			return
		}
		for host, c := range toolsConfig.DatabasesConfig {
			if previous, ok := installedDatabases[host]; !ok || previous != databaseFingerprint(c) {
				log.Error("database configuration changed; restart required")
				return
			}
		}
		return
	}
	for host, c := range toolsConfig.DatabasesConfig {
		setupDatabaseLocked(host, c)
	}
}

func setupSimpleDatabase(host string, c *toolsConfig.Database) {
	setupMu.Lock()
	defer setupMu.Unlock()
	if previous, ok := installedDatabases[host]; ok {
		if previous != databaseFingerprint(c) {
			log.Error("database configuration changed; restart required")
		}
		return
	}
	setupDatabaseLocked(host, c)
}

func setupDatabaseLocked(host string, c *toolsConfig.Database) {
	if global.Driver == "" {
		global.Driver = c.Driver
	}
	log.Infof("%s => %s", host, pkg.Green(redactDSN(c.Source)))
	registers := make([]toolsDB.ResolverConfigure, len(c.Registers))
	for i := range c.Registers {
		registers[i] = toolsDB.NewResolverConfigure(
			c.Registers[i].Sources,
			c.Registers[i].Replicas,
			c.Registers[i].Policy,
			c.Registers[i].Tables)
	}
	open, err := openerFor(c.Driver)
	if err != nil {
		log.Fatal(pkg.Red(err.Error()))
	}
	open = withSqliteDefaults(c.Driver, open)

	resolverConfig := toolsDB.NewConfigure(c.Source, c.MaxIdleConns, c.MaxOpenConns, c.ConnMaxIdleTime, c.ConnMaxLifeTime, registers)
	db, err := resolverConfig.Init(&gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		Logger: New(
			logger.Config{
				SlowThreshold: time.Second,
				Colorful:      true,
				LogLevel: logger.LogLevel(
					log.DefaultLogger.Options().Level.LevelForGorm()),
			},
		),
	}, open)

	if err != nil {
		log.Fatal(pkg.Red(c.Driver+" connect error :"), err)
	} else {
		log.Info(pkg.Green(c.Driver + " connect success !"))
	}

	// Keyed by host, matching the database this enforcer reads from. Passing
	// the same key for every host would hand each one the enforcer built from
	// whichever database was configured first, and the rest would be decided
	// by a casbin_rule table that is not theirs.
	e := mycasbin.Setup(db, host)

	sdk.Runtime.SetDbByTenant(host, db)
	sdk.Runtime.SetCasbinByTenant(host, e)
	installedDatabases[host] = databaseFingerprint(c)
}
