package models

import (
	fm "github.com/gatework/firemon"
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"strings"
	"testing"
)

func TestMigrationTextTypesArePortableAndMySQLPreservesLargeFields(t *testing.T) {
	for _, dialector := range []gorm.Dialector{postgres.Open("host=127.0.0.1 user=synthetic dbname=synthetic sslmode=disable"), mysql.New(mysql.Config{DSN: "synthetic:synthetic@tcp(127.0.0.1)/synthetic", SkipInitializeWithVersion: true}), sqlite.Open(":memory:")} {
		db, e := gorm.Open(dialector, &gorm.Config{DisableAutomaticPing: true})
		if e != nil {
			t.Fatal(e)
		}
		original := db.Dialector
		migration, e := migrationDB(db)
		if e != nil {
			t.Fatal(e)
		}
		for _, model := range Schema() {
			stmt := &gorm.Statement{DB: db}
			if e = stmt.Parse(model); e != nil {
				t.Fatal(e)
			}
			for _, field := range stmt.Schema.Fields {
				if strings.Contains(field.Tag.Get("gorm"), "type:longtext") {
					t.Fatalf("nonportable type %s.%s", stmt.Table, field.DBName)
				}
				if field.TagSettings["TYPE"] != "text" {
					continue
				}
				want := "text"
				if db.Name() == "mysql" {
					want = "longtext"
				}
				if got := migration.Migrator().FullDataTypeOf(field).SQL; got != want {
					t.Fatalf("actual migration %s %s.%s: got %s want %s", db.Name(), stmt.Table, field.DBName, got, want)
				}
				if got := migration.Dialector.DataTypeOf(field); got != want {
					t.Fatalf("%s %s.%s: got %s want %s", db.Name(), stmt.Table, field.DBName, got, want)
				}
			}
		}
		if db.Dialector != original {
			t.Fatal("migration changed caller dialector")
		}
	}
}
func TestMigrationAndLargeNativeRecordRoundTrip(t *testing.T) {
	db, e := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if e != nil {
		t.Fatal(e)
	}
	if e = Migrate(db); e != nil {
		t.Fatal(e)
	}
	record := Rule{Native: Native{SnapshotID: 1, NativeKey: "large"}, Record: fm.RuleRecord{Object: fm.Object{ID: "large"}, Description: strings.Repeat("synthetic native description ", 5000)}}
	if e = db.Create(&record).Error; e != nil {
		t.Fatal(e)
	}
	if e = Migrate(db); e != nil {
		t.Fatal(e)
	}
	var got Rule
	if e = db.First(&got, record.ID).Error; e != nil {
		t.Fatal(e)
	}
	if len(got.Record.Description) <= 65536 || got.Record.Description != record.Record.Description {
		t.Fatal("large record truncated")
	}
}
