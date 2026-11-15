package database

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	mycasbin "github.com/gatework/motor-core/casbin"
	log "github.com/gatework/motor-core/logger"
	"github.com/gatework/motor-core/sdk"
	coreconfig "github.com/gatework/motor-core/sdk/config"
	dbconfig "github.com/gatework/motor-core/tools/database"
	"github.com/gatework/motor-core/tools/gorm/gormlog"
)

// Pools remain process-owned because schedulers and policy adapters retain them.
var setupMu sync.Mutex
var installedDatabases = map[string][32]byte{}

func databaseFingerprint(c *coreconfig.Database) [32]byte {
	data, _ := json.Marshal(c)
	return sha256.Sum256(data)
}

// Setup prepares all pools before publishing tenant resources. Changing an
// installed database requires restart so policies and schedulers cannot diverge.
func Setup(settings *coreconfig.Settings) error {
	if settings == nil {
		return errors.New("database: settings are required")
	}
	setupMu.Lock()
	defer setupMu.Unlock()
	databases := settings.Settings.Databases
	if len(installedDatabases) != 0 {
		if len(installedDatabases) != len(databases) {
			return errors.New("database: configuration changed; restart required")
		}
		for host, c := range databases {
			if previous, ok := installedDatabases[host]; !ok || previous != databaseFingerprint(&c) {
				return errors.New("database: configuration changed; restart required")
			}
		}
		return nil
	}
	prepared := make(map[string]*gorm.DB, len(databases))
	for host, c := range databases {
		db, err := openDatabase(&c, settings.Settings.Logger.Level)
		if err != nil {
			for _, opened := range prepared {
				if pool, poolErr := opened.DB(); poolErr == nil {
					err = errors.Join(err, pool.Close())
				}
			}
			return fmt.Errorf("database: tenant %q: %w", host, err)
		}
		prepared[host] = db
	}
	for host, db := range prepared {
		c := databases[host]
		sdk.Runtime.SetDB(host, db)
		sdk.Runtime.SetCasbin(host, mycasbin.Setup(db, host))
		installedDatabases[host] = databaseFingerprint(&c)
	}
	return nil
}

func openDatabase(c *coreconfig.Database, level string) (*gorm.DB, error) {
	log.NewHelper(log.Default()).Infof("%s => %s", c.Driver, redactDSN(c.Source))
	registers := make([]dbconfig.ResolverConfigure, len(c.Registers))
	for i, r := range c.Registers {
		registers[i] = dbconfig.NewResolverConfigure(r.Sources, r.Replicas, r.Policy, r.Tables)
	}
	open, err := openerFor(c.Driver)
	if err != nil {
		return nil, err
	}
	open = withSqliteDefaults(c.Driver, open)
	logLevel := logger.Warn
	switch strings.ToLower(level) {
	case "debug", "info":
		logLevel = logger.Info
	case "error", "fatal":
		logLevel = logger.Error
	}
	resolved := dbconfig.NewConfigure(c.Source, c.MaxIdleConns, c.MaxOpenConns, c.ConnMaxIdleTime, c.ConnMaxLifeTime, registers)
	// TranslateError turns each driver's unique-violation error into
	// gorm.ErrDuplicatedKey, so a duplicate can be answered as a conflict
	// instead of matched by driver-specific text.
	return resolved.Init(&gorm.Config{TranslateError: true, NamingStrategy: schema.NamingStrategy{SingularTable: true}, Logger: gormlog.New(logger.Config{SlowThreshold: time.Second, Colorful: true, LogLevel: logLevel})}, open)
}
