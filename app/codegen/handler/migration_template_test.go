package handler

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
	"text/template"

	"github.com/gatework/motor/app/codegen/models"
)

func TestGeneratorMigrationUsesCanonicalIDs(t *testing.T) {
	parsed, err := template.ParseFiles(filepath.Join("../../../template", "api_migrate.template"))
	if err != nil {
		t.Fatal(err)
	}
	var rendered bytes.Buffer
	data := struct {
		models.SysTables
		GenerateTime string
	}{SysTables: genTable(), GenerateTime: "1794697200001"}
	if err := parsed.Execute(&rendered, data); err != nil {
		t.Fatal(err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "migration.go", rendered.Bytes(), 0)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		if id, ok := node.(*ast.Ident); ok {
			found[id.Name] = true
		}
		return true
	})
	if !found["MenuID"] || !found["ParentID"] || found["MenuId"] || found["ParentId"] {
		t.Fatalf("generated migration uses inconsistent ID declarations or selectors: %v", found)
	}
}
