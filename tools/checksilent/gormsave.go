package main

import "go/ast"

const checkGORMSave = "gorm-save"

// checkGORMSaves reports calls to GORM's Save. On a row that was read earlier
// Save writes every column back - re-inserting the row if it was deleted
// meanwhile - and carries no version predicate, so a concurrent change is
// silently overwritten. Create a new row, or Updates/Select an existing one
// under a version or ownership condition. A package-level Save function (such
// as connection.Save) is not GORM's and is not reported.
func checkGORMSaves(s *snapshot) []Finding {
	var out []Finding
	for _, sf := range s.Files {
		if sf.isTest() {
			continue
		}
		ast.Inspect(sf.Syntax, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Save" {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && sf.imports[id.Name] != "" {
				return true // a package function
			}
			out = append(out, s.finding(Error, checkGORMSave, sf, sel.Sel,
				"GORM Save writes back a row that may have been deleted or changed; use Create, or Updates under a version condition"))
			return true
		})
	}
	return out
}
