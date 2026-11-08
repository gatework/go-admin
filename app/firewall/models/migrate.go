package models

import (
	"errors"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// Migrate uses portable TEXT on PostgreSQL/SQLite and LONGTEXT for this module's
// unbounded text/JSON fields on MySQL. The migration-only dialect view also makes
// repeated runs preserve existing LONGTEXT rather than narrowing large records.
func Migrate(db *gorm.DB) error {
	migration, err := migrationDB(db)
	if err != nil {
		return err
	}
	return migration.AutoMigrate(Schema()...)
}
func migrationDB(db *gorm.DB) (*gorm.DB, error) {
	if db == nil {
		return nil, errors.New("nil firewall migration database")
	}
	if db.Name() != "mysql" {
		return db, nil
	}
	fields := map[*schema.Field]bool{}
	for _, model := range Schema() {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(model); err != nil {
			return nil, err
		}
		for _, field := range stmt.Schema.Fields {
			if field.TagSettings["TYPE"] == "text" {
				fields[field] = true
			}
		}
	}
	migration := db.Session(&gorm.Session{NewDB: true})
	migration.Dialector = largeTextDialect{Dialector: db.Dialector, fields: fields}
	return migration, nil
}

type largeTextDialect struct {
	gorm.Dialector
	fields map[*schema.Field]bool
}

func (d largeTextDialect) DataTypeOf(field *schema.Field) string {
	if d.fields[field] {
		return "longtext"
	}
	return d.Dialector.DataTypeOf(field)
}

func (d largeTextDialect) Migrator(db *gorm.DB) gorm.Migrator {
	migrator := d.Dialector.Migrator(db)
	if mysqlMigrator, ok := migrator.(mysql.Migrator); ok {
		mysqlMigrator.Migrator.Config.Dialector = d
		return mysqlMigrator
	}
	return migrator
}
