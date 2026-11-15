package migrate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// chdirWithTemplates runs the test from an empty directory that has the
// repository's template/ directory in it, which is what genFile reads from
// the working directory.
func chdirWithTemplates(t *testing.T) string {
	t.Helper()
	tmpl, err := filepath.Abs(filepath.Join("..", "..", "template"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(tmpl, filepath.Join(dir, "template")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return dir
}

func TestGenFileWritesTheMigration(t *testing.T) {
	repository, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	dir := chdirWithTemplates(t)
	out := filepath.Join(dir, "cmd", "migrate", "migration", "version-local")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := genFile(); err != nil {
		t.Fatalf("genFile: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(out, "*_migrate.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("generated files = %v, want one", files)
	}

	// The generated file joins cmd/migrate/migration/version-local, so it must
	// declare that directory's package. It once declared version_local and could
	// not be compiled.
	generated, err := parser.ParseFile(token.NewFileSet(), files[0], nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("generated migration does not parse: %v", err)
	}
	doc, err := filepath.Abs(filepath.Join(repository, "cmd", "migrate", "migration", "version-local", "doc.go"))
	if err != nil {
		t.Fatal(err)
	}
	existing, err := parser.ParseFile(token.NewFileSet(), doc, nil, parser.PackageClauseOnly)
	if err != nil {
		t.Fatal(err)
	}
	if generated.Name.Name != existing.Name.Name {
		t.Fatalf("generated package %q, directory package %q", generated.Name.Name, existing.Name.Name)
	}
	for _, decl := range generated.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && strings.Contains(fn.Name.Name, "_") {
			t.Fatalf("generated function %q is not MixedCaps", fn.Name.Name)
		}
	}
}

func TestWithTableOptionsReachesStatementsStartedFromTheHandle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := withTableOptions(db, "postgres").Table("t").Get("gorm:table_options"); ok {
		t.Fatal("table options applied to a non-MySQL database")
	}
	options, ok := withTableOptions(db, "mysql").Table("t").Get("gorm:table_options")
	if !ok || options != "ENGINE=InnoDB CHARSET=utf8mb4" {
		t.Fatalf("table options not carried into a new statement: %v %v", options, ok)
	}
	// The handle must stay a reusable base: conditions from one statement must
	// not leak into the next.
	base := withTableOptions(db, "mysql")
	base.Where("a = ?", 1)
	if len(base.Table("t").Statement.Clauses) != 0 {
		t.Fatal("handle accumulates clauses across statements")
	}
}

// Run from a directory without cmd/migrate/migration, writing the file fails.
// The failure used to end the process from inside pkg.FileCreate and, before
// that, was dropped by genFile's caller; it now reaches the caller.
func TestGenFileReportsAFileItCannotWrite(t *testing.T) {
	chdirWithTemplates(t)

	if err := genFile(); err == nil {
		t.Fatal("genFile returned nil with no directory to write into")
	}
}
