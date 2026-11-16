package main

import (
	"go/ast"
	"go/token"
	"strings"
	"unicode"
)

const checkErrorName = "sentinel-error-name"

// checkErrorNames enforces the project's Err suffix without renaming imported
// errors. It inspects package variables, never ordinary local err variables.
func checkErrorNames(s *snapshot) []Finding {
	var out []Finding
	for _, sf := range s.Files {
		for _, declaration := range sf.Syntax.Decls {
			decl, ok := declaration.(*ast.GenDecl)
			if !ok || decl.Tok != token.VAR {
				continue
			}
			for _, spec := range decl.Specs {
				variable, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range variable.Names {
					if name.Name == "_" || strings.HasSuffix(name.Name, "Err") {
						continue
					}
					if hasErrorPrefix(name.Name) || isErrorConstructor(sf, variable.Values, i) {
						out = append(out, s.finding(Error, checkErrorName, sf, name,
							"sentinel %s must use an Err suffix (see docs/standards.md)", name.Name))
					}
				}
			}
		}
	}
	return out
}

func hasErrorPrefix(name string) bool {
	if !strings.HasPrefix(name, "Err") && !strings.HasPrefix(name, "err") {
		return false
	}
	return len(name) > 3 && unicode.IsUpper([]rune(name[3:])[0])
}

func isErrorConstructor(sf *sourceFile, values []ast.Expr, index int) bool {
	if index >= len(values) {
		return false
	}
	call, ok := values[index].(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	switch sf.imports[pkg.Name] {
	case "errors", classifiedErrors:
		return selector.Sel.Name == "New"
	case "fmt":
		return selector.Sel.Name == "Errorf"
	default:
		return false
	}
}

// classifiedErrors declares the project's classified sentinels; they follow
// the same naming rule as errors.New.
const classifiedErrors = "github.com/gatework/motor/common/errors"
