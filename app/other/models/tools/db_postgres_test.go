package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-admin-team/go-admin-core/v2/sdk/config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func postgresMetadataDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("GO_ADMIN_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set GO_ADMIN_TEST_POSTGRES_DSN to a dedicated test database")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close() })
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	schema := fmt.Sprintf("gen_meta_%d", time.Now().UnixNano())
	if err := tx.Exec(`CREATE SCHEMA "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`SET LOCAL search_path TO "` + schema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.AutoMigrate(&SysTables{}); err != nil {
		t.Fatal(err)
	}
	previousDriver, previousName := config.DatabaseConfig.Driver, config.GenConfig.DBName
	config.DatabaseConfig.Driver, config.GenConfig.DBName = "mysql", "not_this_tenant"
	t.Cleanup(func() { config.DatabaseConfig.Driver, config.GenConfig.DBName = previousDriver, previousName })
	return tx
}

func TestPostgresGeneratorMetadata(t *testing.T) {
	db := postgresMetadataDB(t)
	var schema string
	if err := db.Raw("SELECT current_schema()").Scan(&schema).Error; err != nil {
		t.Fatal(err)
	}
	otherSchema := schema + "_other"
	if err := db.Exec(`CREATE SCHEMA "` + otherSchema + `"`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE "` + otherSchema + `".pg_widget (id uuid PRIMARY KEY)`).Error; err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE TABLE pg_widget (id bigserial PRIMARY KEY, obsolete text, name varchar(72) NOT NULL DEFAULT 'hello', enabled boolean, amount numeric(18,4), created_at timestamptz, token uuid, payload jsonb, tags text[])`,
		`ALTER TABLE pg_widget DROP COLUMN obsolete`,
		`COMMENT ON TABLE pg_widget IS 'Widget catalog'`,
		`COMMENT ON COLUMN pg_widget.name IS 'Widget name'`,
		`CREATE TABLE pg_identity (id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, total integer GENERATED ALWAYS AS (id * 2) STORED)`,
		`CREATE TABLE pg_pair (first_id integer, second_id integer, PRIMARY KEY (first_id, second_id))`,
		`CREATE VIEW pg_widget_view AS SELECT id FROM pg_widget`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	table, err := (&DBTables{TableName: "pg_widget"}).Get(db)
	if err != nil || table.TableName != "pg_widget" || table.TableComment != "Widget catalog" {
		t.Fatalf("table metadata: %+v, %v", table, err)
	}
	list, total, err := (&DBTables{TableName: "pg_widget"}).GetPage(db, 10, 1)
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("table page: %v %d %v", list, total, err)
	}
	if _, err := (&DBTables{TableName: "pg_widget' OR 1=1 --"}).Get(db); err == nil {
		t.Fatal("table filter treated as SQL")
	}
	views, total, err := (&DBTables{TableName: "pg_widget_view"}).GetPage(db, 10, 1)
	if err != nil || total != 0 || len(views) != 0 {
		t.Fatalf("view listed as writable table: %v %d %v", views, total, err)
	}
	columns, err := (&DBColumns{TableName: "pg_widget"}).GetList(db)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"id", "name", "enabled", "amount", "created_at", "token", "payload", "tags"}
	if len(columns) != len(names) {
		t.Fatalf("columns: %+v", columns)
	}
	for i, c := range columns {
		if c.ColumnName != names[i] || c.TableSchema != schema {
			t.Fatalf("column order: %+v", columns)
		}
	}
	if columns[0].ColumnKey != "PRI" || columns[0].Extra != "auto_increment" || columns[0].DataType != "int8" {
		t.Fatalf("serial key: %+v", columns[0])
	}
	if columns[1].ColumnComment != "Widget name" || columns[1].ColumnType != "character varying(72)" || columns[1].CharacterMaximumLength != "72" || columns[1].IsNullable != "NO" || !strings.Contains(columns[1].ColumnDefault, "hello") {
		t.Fatalf("column details: %+v", columns[1])
	}
	page, count, err := (&DBColumns{TableName: "pg_widget"}).GetPage(db, 2, 2)
	if err != nil || count != len(names) || len(page) != 2 || page[0].ColumnName != "enabled" {
		t.Fatalf("column page: %+v %d %v", page, count, err)
	}
	ids, err := (&DBColumns{TableName: "pg_identity"}).GetList(db)
	if err != nil || len(ids) != 2 || ids[0].Extra != "auto_increment" || ids[1].Extra != "GENERATED" {
		t.Fatalf("identity/computed: %+v %v", ids, err)
	}
	pair, err := (&DBColumns{TableName: "pg_pair"}).GetList(db)
	if err != nil || len(pair) != 2 || pair[0].ColumnKey != "PRI" || pair[1].ColumnKey != "PRI" {
		t.Fatalf("composite key: %+v %v", pair, err)
	}
	imported := SysTables{TBName: "pg_widget"}
	if err := db.Create(&imported).Error; err != nil {
		t.Fatal(err)
	}
	_, count, err = (&DBTables{TableName: "pg_widget"}).GetPage(db, 10, 1)
	if err != nil || count != 0 {
		t.Fatalf("registered table still listed: %d %v", count, err)
	}
	if err := db.Delete(&imported).Error; err != nil {
		t.Fatal(err)
	}
	_, count, err = (&DBTables{TableName: "pg_widget"}).GetPage(db, 10, 1)
	if err != nil || count != 1 {
		t.Fatalf("soft-deleted registration hides table: %d %v", count, err)
	}
	malicious, err := (&DBColumns{TableName: "pg_widget' OR 1=1 --"}).GetList(db)
	if err != nil || len(malicious) != 0 {
		t.Fatalf("column table filter is not a bound value: %+v %v", malicious, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&DBColumns{TableName: "pg_widget"}).GetList(db.WithContext(ctx)); !errors.Is(err, context.Canceled) {
		t.Fatalf("query ignored cancellation: %v", err)
	}
}
