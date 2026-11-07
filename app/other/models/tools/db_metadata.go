package tools

import (
	"errors"
	"fmt"

	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"gorm.io/gorm"
)

// Metadata belongs to the request's connection, not the process-wide default
// driver. PostgreSQL uses current_schema(), matching generated unqualified
// table names and the connection's search_path. Gen.DBName remains MySQL-only.
func metadataTables(tx *gorm.DB) (*gorm.DB, error) {
	driver, err := metadataDriver(tx)
	if err != nil {
		return nil, err
	}
	if driver == "postgres" {
		return tx.Table("(?) AS generator_tables", tx.Raw(postgresTablesSQL)), nil
	}
	return tx.Table("information_schema.tables").Where("table_schema = ?", config.GenConfig.DBName), nil
}

func metadataColumns(tx *gorm.DB) (*gorm.DB, error) {
	driver, err := metadataDriver(tx)
	if err != nil {
		return nil, err
	}
	if driver == "postgres" {
		return tx.Table("(?) AS generator_columns", tx.Raw(postgresColumnsSQL)), nil
	}
	return tx.Table("information_schema.columns").Where("table_schema = ?", config.GenConfig.DBName), nil
}

func metadataDriver(tx *gorm.DB) (string, error) {
	if tx == nil || tx.Config == nil || tx.Dialector == nil {
		return "", errors.New("code generation requires a database connection")
	}
	switch driver := tx.Dialector.Name(); driver {
	case "mysql", "postgres":
		return driver, nil
	default:
		return "", fmt.Errorf("code generation supports MySQL and PostgreSQL; unsupported driver %q", driver)
	}
}

const postgresTablesSQL = `SELECT c.relname AS "TABLE_NAME",
 'PostgreSQL'::text AS "ENGINE",
 greatest(c.reltuples, 0)::bigint::text AS "TABLE_ROWS",
 ''::text AS "TABLE_COLLATION", ''::text AS "CREATE_TIME", ''::text AS "UPDATE_TIME",
 coalesce(pg_catalog.obj_description(c.oid, 'pg_class'), '') AS "TABLE_COMMENT"
 FROM pg_catalog.pg_class c
 JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
 WHERE n.nspname = current_schema() AND c.relkind IN ('r', 'p')
 AND c.relpersistence <> 't' AND pg_catalog.has_table_privilege(c.oid, 'SELECT')`

const postgresColumnsSQL = `SELECT n.nspname AS "TABLE_SCHEMA", c.relname AS "TABLE_NAME",
 a.attname AS "COLUMN_NAME", a.attnum AS "ORDINAL_POSITION",
 coalesce(pg_catalog.pg_get_expr(d.adbin, d.adrelid), '') AS "COLUMN_DEFAULT",
 CASE WHEN a.attnotnull THEN 'NO' ELSE 'YES' END AS "IS_NULLABLE",
 t.typname AS "DATA_TYPE",
 CASE WHEN t.typname IN ('varchar', 'bpchar') AND a.atttypmod >= 4
      THEN (a.atttypmod - 4)::text ELSE '' END AS "CHARACTER_MAXIMUM_LENGTH",
 ''::text AS "CHARACTER_SET_NAME",
 pg_catalog.format_type(a.atttypid, a.atttypmod) AS "COLUMN_TYPE",
 CASE WHEN EXISTS (SELECT 1 FROM pg_catalog.pg_constraint p
                   WHERE p.conrelid = c.oid AND p.contype = 'p' AND a.attnum = ANY(p.conkey))
      THEN 'PRI' ELSE '' END AS "COLUMN_KEY",
 CASE WHEN a.attgenerated <> '' THEN 'GENERATED'
      WHEN a.attidentity <> '' OR pg_catalog.pg_get_expr(d.adbin, d.adrelid) LIKE 'nextval(%'
      THEN 'auto_increment' ELSE '' END AS "EXTRA",
 coalesce(pg_catalog.col_description(c.oid, a.attnum), '') AS "COLUMN_COMMENT"
 FROM pg_catalog.pg_attribute a
 JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
 JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
 JOIN pg_catalog.pg_type t ON t.oid = a.atttypid
 LEFT JOIN pg_catalog.pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
 WHERE n.nspname = current_schema() AND c.relkind IN ('r', 'p') AND c.relpersistence <> 't'
 AND a.attnum > 0 AND NOT a.attisdropped AND pg_catalog.has_table_privilege(c.oid, 'SELECT')`
