package tools

import (
	models "go-admin/app/other/models/tools"
	"testing"
)

func TestPostgresColumnTypes(t *testing.T) {
	for _, tc := range []struct{ kind, goType, html string }{
		{"int2", "int", "number"}, {"int4", "int", "number"}, {"int8", "int64", "input"},
		{"numeric", "string", "input"}, {"float4", "float32", "number"}, {"float8", "float64", "number"},
		{"bool", "bool", "checkbox"}, {"uuid", "string", "input"}, {"text", "string", "input"},
		{"varchar", "string", "input"}, {"bpchar", "string", "input"}, {"bytea", "[]byte", "input"},
		{"date", "time.Time", "datetime"}, {"timestamp", "time.Time", "datetime"}, {"timestamptz", "time.Time", "datetime"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			for _, nullable := range []string{"NO", "YES"} {
				var c models.SysColumns
				if err := mapGeneratorColumn(&c, models.DBColumns{ColumnName: "value", DataType: tc.kind, ColumnType: tc.kind, IsNullable: nullable}, "postgres"); err != nil {
					t.Fatal(err)
				}
				want := tc.goType
				if nullable == "YES" && want != "[]byte" {
					want = "*" + want
				}
				if c.GoType != want || c.HtmlType != tc.html {
					t.Fatalf("nullable=%s mapping=%+v want=%s/%s", nullable, c, want, tc.html)
				}
			}
		})
	}
}
func TestPostgresUnsupportedTypesAndGeneratedColumns(t *testing.T) {
	for _, kind := range []string{"_int4", "json", "jsonb", "int4range", "custom_domain", "interval"} {
		var c models.SysColumns
		if err := mapGeneratorColumn(&c, models.DBColumns{ColumnName: "payload", DataType: kind, ColumnType: kind}, "postgres"); err == nil {
			t.Errorf("unsupported %s accepted", kind)
		}
	}
	c := models.SysColumns{Insert: true, Edit: true, IsInsert: "1", IsEdit: "1"}
	if err := mapGeneratorColumn(&c, models.DBColumns{ColumnName: "doubled", DataType: "int4", ColumnType: "integer", Extra: "GENERATED"}, "postgres"); err != nil {
		t.Fatal(err)
	}
	if c.Insert || c.Edit || c.IsInsert != "0" || c.IsEdit != "0" {
		t.Fatalf("computed field remains writable: %+v", c)
	}
	for _, extra := range []string{"auto_increment", ""} {
		var c models.SysColumns
		if err := mapGeneratorColumn(&c, models.DBColumns{DataType: "int4", ColumnType: "integer", Extra: extra}, "postgres"); err != nil {
			t.Fatal(err)
		}
		if (c.IsIncrement == "1") != (extra != "") {
			t.Fatalf("extra=%s increment=%s", extra, c.IsIncrement)
		}
	}
}

func TestGeneratorNamesRejectUnsupportedIdentifiers(t *testing.T) {
	for _, name := range []string{"", "_leading", "trailing_", "repeated__word", "hyphen-name", "with space", "9start", "payload\";drop"} {
		if _, _, err := generatorNames(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	for name, want := range map[string]string{"order2": "Order2", "order_no": "OrderNo", "x": "X"} {
		got, _, err := generatorNames(name)
		if err != nil || got != want {
			t.Fatalf("%s = %s, %v", name, got, err)
		}
	}
}

func TestPostgresComputedPrimaryKeyIsRefused(t *testing.T) {
	c := models.SysColumns{Pk: true, IsPk: "1"}
	if err := mapGeneratorColumn(&c, models.DBColumns{ColumnName: "id", DataType: "int4", ColumnType: "integer", Extra: "GENERATED"}, "postgres"); err == nil {
		t.Fatal("generated expression primary key accepted")
	}
}
func TestPostgresAuditShapeMustMatchEmbeddedModels(t *testing.T) {
	valid := []models.DBColumns{
		{ColumnName: "create_by", DataType: "int8"}, {ColumnName: "update_by", DataType: "int4"},
		{ColumnName: "created_at", DataType: "timestamp"}, {ColumnName: "updated_at", DataType: "timestamptz"},
		{ColumnName: "deleted_at", DataType: "int8", IsNullable: "NO"},
	}
	if err := validatePostgresAuditColumns(valid); err != nil {
		t.Fatal(err)
	}
	if err := validatePostgresAuditColumns(valid[:4]); err == nil {
		t.Fatal("missing audit column accepted")
	}
	invalid := append([]models.DBColumns(nil), valid...)
	invalid[4].DataType = "timestamp"
	if err := validatePostgresAuditColumns(invalid); err == nil {
		t.Fatal("timestamp soft delete accepted")
	}
}

func TestPostgresAutomaticNonPrimaryColumnsAreReadOnly(t *testing.T) {
	c := models.SysColumns{Insert: true, Edit: true, IsInsert: "1", IsEdit: "1"}
	if err := mapGeneratorColumn(&c, models.DBColumns{ColumnName: "sequence", DataType: "int4", ColumnType: "integer", Extra: "auto_increment"}, "postgres"); err != nil {
		t.Fatal(err)
	}
	if c.IsInsert != "0" || c.IsEdit != "0" {
		t.Fatalf("automatic column writable: %+v", c)
	}
}
func TestTypedDefaultsRejectInvalidGeneratedLiterals(t *testing.T) {
	for _, tc := range []struct{ kind, value string }{{"*int", "seven"}, {"bool", "yes"}, {"float64", "NaN"}, {"*float32", "Infinity"}, {"int", "007"}, {"float64", "0x1p2"}, {"int64", "9223372036854775808"}} {
		if err := validateTypedColumnDefault(models.SysColumns{GoType: tc.kind, DefaultValue: tc.value}); err == nil {
			t.Errorf("accepted %s %s", tc.kind, tc.value)
		}
	}
	for _, tc := range []struct{ kind, value string }{{"*int", "7"}, {"bool", "false"}, {"float64", "1.25"}, {"*int64", "9007199254740993"}, {"string", "seven"}} {
		if err := validateTypedColumnDefault(models.SysColumns{GoType: tc.kind, DefaultValue: tc.value}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPostgresAuditMetadataMatchesEmbeddedTypes(t *testing.T) {
	for name, kind := range map[string]string{"create_by": "int8", "update_by": "int8", "deleted_at": "int8", "created_at": "timestamp", "updated_at": "timestamptz"} {
		var c models.SysColumns
		if err := mapGeneratorColumn(&c, models.DBColumns{ColumnName: name, DataType: kind, IsNullable: "YES"}, "postgres"); err != nil {
			t.Fatal(err)
		}
		want := "int"
		if name == "created_at" || name == "updated_at" {
			want = "time.Time"
		}
		if c.GoType != want {
			t.Fatalf("embedded audit %s mapped to %s, want %s", name, c.GoType, want)
		}
	}
}

func TestPostgresGeneratorRejectsQuotedCaseIdentifiers(t *testing.T) {
	for _, name := range []string{"displayName", "Id", "ALL_CAPS", "_name", "name_", "two__words"} {
		if err := validatePostgresGeneratorIdentifier(name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	for _, name := range []string{"id", "display_name", "count_1"} {
		if err := validatePostgresGeneratorIdentifier(name); err != nil {
			t.Fatal(err)
		}
	}
}
