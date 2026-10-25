package tools

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	models "go-admin/app/other/models/tools"
)

func renderPGTemplate(t *testing.T, name string, columns []models.SysColumns) string {
	t.Helper()
	templatePath := filepath.Join("../../../../template/v4", name+".go.template")
	parsed, err := template.ParseFiles(templatePath)
	if err != nil {
		t.Fatal(err)
	}
	var result bytes.Buffer
	table := models.SysTables{TBName: "pg_record", ClassName: "PgRecord", PackageName: "admin", BusinessName: "pgRecord", ModuleName: "pg-record", PkColumn: "id", PkGoField: "Id", PkJsonField: "id", Columns: columns}
	if err := parsed.Execute(&result, table); err != nil {
		t.Fatal(err)
	}
	return result.String()
}

func TestPostgresTemplatesPreserveAssignedKeysAndScalarTypes(t *testing.T) {
	assigned := models.SysColumns{ColumnName: "id", GoField: "Id", JsonField: "id", GoType: "int", ColumnType: "integer", Pk: true, IsPk: "1", IsIncrement: "0"}
	cols := []models.SysColumns{assigned, {ColumnName: "active", GoField: "Active", JsonField: "active", GoType: "bool", ColumnType: "boolean", HtmlType: "checkbox", IsInsert: "1"}}
	if model := renderPGTemplate(t, "model", cols); strings.Contains(model, "models.Model\n") || !strings.Contains(model, "autoIncrement:false") {
		t.Fatalf("assigned integer key inferred as automatic:\n%s", model)
	}
	if dto := renderPGTemplate(t, "dto", cols); strings.Contains(dto, "json:\"-\"") {
		t.Fatalf("assigned key excluded from insert:\n%s", dto)
	}
	if ts := renderPGTemplate(t, "ts", cols); !strings.Contains(ts, "active?: boolean") {
		t.Fatalf("boolean typed as string:\n%s", ts)
	}
	if vue := renderPGTemplate(t, "vue", cols); !strings.Contains(vue, `v-model="form.model.id"`) || strings.Contains(vue, `true-value="1"`) {
		t.Fatalf("invalid assigned-key/boolean form:\n%s", vue)
	}
	assigned.GoType, assigned.ColumnType = "string", "uuid"
	if ts := renderPGTemplate(t, "ts", []models.SysColumns{assigned}); strings.Contains(ts, "ids.map(Number)") {
		t.Fatalf("UUID delete coerced to number:\n%s", ts)
	}
}

func TestPostgresTemplatesKeepComputedColumnsReadOnly(t *testing.T) {
	columns := []models.SysColumns{
		{ColumnName: "id", GoField: "Id", JsonField: "id", GoType: "int", ColumnType: "integer", Pk: true, IsIncrement: "1"},
		{ColumnName: "doubled", GoField: "Doubled", JsonField: "doubled", GoType: "int", ColumnType: "integer", IsInsert: "0", IsEdit: "0"},
		{ColumnName: "optional_count", GoField: "OptionalCount", JsonField: "optionalCount", GoType: "*int", ColumnType: "integer", IsInsert: "1", IsEdit: "1"},
	}
	if model := renderPGTemplate(t, "model", columns); !strings.Contains(model, `type:integer;->;comment:`) {
		t.Fatalf("computed column writable:\n%s", model)
	}
	dto := renderPGTemplate(t, "dto", columns)
	if strings.Contains(dto, "model.Doubled") || strings.Count(dto, "Doubled ") != 1 {
		t.Fatalf("computed column entered write DTO:\n%s", dto)
	}
	if ts := renderPGTemplate(t, "ts", columns); !strings.Contains(ts, "optionalCount?: number | null") {
		t.Fatalf("nullable scalar lost null:\n%s", ts)
	}
}

func TestPostgresNullableDefaultsKeepTransportTypes(t *testing.T) {
	columns := []models.SysColumns{
		{ColumnName: "id", GoField: "Id", JsonField: "id", GoType: "int", Pk: true, IsIncrement: "1"},
		{ColumnName: "count", GoField: "Count", JsonField: "count", GoType: "*int", IsInsert: "1", DefaultValue: "7"},
		{ColumnName: "active", GoField: "Active", JsonField: "active", GoType: "*bool", IsInsert: "1", DefaultValue: "true"},
		{ColumnName: "large", GoField: "Large", JsonField: "large", GoType: "*int64", IsInsert: "1", DefaultValue: "9007199254740993"},
	}
	vue := renderPGTemplate(t, "vue", columns)
	for _, expected := range []string{"count: 7", "active: true", "large: '9007199254740993'"} {
		if !strings.Contains(vue, expected) {
			t.Fatalf("default %s missing:\n%s", expected, vue)
		}
	}
}

func TestPostgresTemplatesPreserveColumnNamesAndQueryPresence(t *testing.T) {
	columns := []models.SysColumns{
		{ColumnName: "Id", GoField: "Id", JsonField: "id", GoType: "int", Pk: true, IsIncrement: "1"},
		{ColumnName: "count_1", GoField: "Count1", JsonField: "count1", GoType: "int", IsQuery: "1", QueryType: "EQ"},
		{ColumnName: "active", GoField: "Active", JsonField: "active", GoType: "bool", IsQuery: "1", QueryType: "EQ"},
		{ColumnName: "label", GoField: "Label", JsonField: "label", GoType: "*string", IsQuery: "1", QueryType: "LIKE"},
		{ColumnName: "payload", GoField: "Payload", JsonField: "payload", GoType: "[]byte"},
	}
	model := renderPGTemplate(t, "model", columns)
	if strings.Contains(model, "models.Model\n") || !strings.Contains(model, "column:Id;") || !strings.Contains(model, "column:count_1;") {
		t.Fatalf("incorrect literal column mapping: %s", model)
	}
	dto := renderPGTemplate(t, "dto", columns)
	for _, expected := range []string{"Count1 *int `form:", "Active *bool `form:", "Label string `form:"} {
		if !strings.Contains(dto, expected) {
			t.Errorf("missing %s in %s", expected, dto)
		}
	}
	if ts := renderPGTemplate(t, "ts", columns); !strings.Contains(ts, "payload?: string | null") {
		t.Fatalf("nil bytes lose null: %s", ts)
	}
}

func TestPostgresNullableBoolFormPreservesThreeStates(t *testing.T) {
	columns := []models.SysColumns{
		{ColumnName: "id", GoField: "Id", JsonField: "id", GoType: "int", Pk: true, IsIncrement: "1"},
		{ColumnName: "active", GoField: "Active", JsonField: "active", GoType: "*bool", HtmlType: "checkbox", IsInsert: "1"},
		{ColumnName: "enabled", GoField: "Enabled", JsonField: "enabled", GoType: "bool", HtmlType: "checkbox", IsInsert: "1"},
	}
	vue := renderPGTemplate(t, "vue", columns)
	for _, want := range []string{`<el-select v-model="form.model.active" clearable :value-on-clear="null">`, `<el-option label="true" :value="true" />`, `<el-option label="false" :value="false" />`, `<el-checkbox v-model="form.model.enabled" />`} {
		if !strings.Contains(vue, want) {
			t.Fatalf("missing %s in %s", want, vue)
		}
	}
	if strings.Contains(vue, `<el-checkbox v-model="form.model.active"`) {
		t.Fatal("nullable bool bound to two-state checkbox")
	}
}
