package main

import (
	"go/ast"
	"go/token"
	"sort"
	"strconv"
	"strings"
)

const checkErrorCodeReused = "sentinel-code-reused"

// checkErrorCodesReused reports a classified sentinel whose code another one
// already declares. errors.Error matches by code, so the two are the same error
// under errors.Is: a check for one silently catches the other. A condition
// that several packages report is declared once and referenced. Test files
// are skipped.
func checkErrorCodesReused(s *snapshot) []Finding {
	type declaration struct {
		sf   *sourceFile
		name *ast.Ident
	}
	byCode := map[string][]declaration{}
	for _, sf := range s.Files {
		if strings.HasSuffix(sf.Path, "_test.go") {
			continue
		}
		for _, d := range sf.Syntax.Decls {
			decl, ok := d.(*ast.GenDecl)
			if !ok || decl.Tok != token.VAR {
				continue
			}
			for _, spec := range decl.Specs {
				variable, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range variable.Names {
					if code, ok := classifiedCode(sf, variable.Values, i); ok {
						byCode[code] = append(byCode[code], declaration{sf, name})
					}
				}
			}
		}
	}
	codes := make([]string, 0, len(byCode))
	for code := range byCode {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	var out []Finding
	for _, code := range codes {
		all := byCode[code]
		sort.Slice(all, func(i, j int) bool { return all[i].sf.Path < all[j].sf.Path })
		for _, d := range all[1:] {
			out = append(out, s.finding(Error, checkErrorCodeReused, d.sf, d.name,
				"%s declares code %q, already declared by %s in %s; declare it once and reference it",
				d.name.Name, code, all[0].name.Name, all[0].sf.Path))
		}
	}
	return out
}

// classifiedCode is the code of an errors.New(kind, "code", msg) value.
func classifiedCode(sf *sourceFile, values []ast.Expr, index int) (string, bool) {
	if index >= len(values) {
		return "", false
	}
	call, ok := values[index].(*ast.CallExpr)
	if !ok || len(call.Args) < 2 {
		return "", false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "New" {
		return "", false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || sf.imports[pkg.Name] != classifiedErrors {
		return "", false
	}
	literal, ok := call.Args[1].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	code, err := strconv.Unquote(literal.Value)
	return code, err == nil && code != ""
}
