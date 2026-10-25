package main

import "go/ast"

const checkGORMDiscarded = "gorm-discarded-result"

// checkDiscardedGORMResults deliberately covers only proven service receivers:
// an embedded core Service owns Orm, and explicit Orm shadows are excluded.
// It is a narrow guard against the First-then-Save regression, not a substitute
// for type-aware errcheck or a claim to detect every ignored database error.
func checkDiscardedGORMResults(s *snapshot) []Finding {
	services := make(map[string]bool)
	for _, sf := range s.Files {
		if sf.isTest() {
			continue
		}
		for name, st := range structTypes(sf) {
			shadow := false
			for _, field := range st.Fields.List {
				for _, n := range field.Names {
					if n.Name == "Orm" {
						shadow = true
					}
				}
			}
			if shadow {
				continue
			}
			for _, embed := range embeddedTypes(sf, st) {
				if embed[0] == "github.com/go-admin-team/go-admin-core/v2/sdk/service" && embed[1] == "Service" {
					services[sf.Pkg+"."+name] = true
				}
			}
		}
	}
	terminals := map[string]bool{"First": true, "Last": true, "Take": true, "Find": true, "Scan": true, "Count": true, "Create": true, "Save": true, "Updates": true, "Update": true, "UpdateColumn": true, "UpdateColumns": true, "Delete": true, "Exec": true, "FirstOrCreate": true, "FirstOrInit": true, "Transaction": true, "Commit": true, "Rows": true}
	var findings []Finding
	for _, sf := range s.Files {
		if sf.isTest() {
			continue
		}
		for _, decl := range sf.Syntax.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Body == nil || len(fn.Recv.List) != 1 || len(fn.Recv.List[0].Names) != 1 {
				continue
			}
			receiver := fn.Recv.List[0]
			typ := receiver.Type
			if ptr, ok := typ.(*ast.StarExpr); ok {
				typ = ptr.X
			}
			name, ok := typ.(*ast.Ident)
			if !ok || !services[sf.Pkg+"."+name.Name] {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				statement, ok := node.(*ast.ExprStmt)
				if !ok {
					return true
				}
				call, ok := statement.X.(*ast.CallExpr)
				if !ok {
					return true
				}
				method, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || !terminals[method.Sel.Name] {
					return true
				}
				if serviceORMRoot(method.X, receiver.Names[0]) {
					findings = append(findings, s.finding(Error, checkGORMDiscarded, sf, statement, "database result from %s is discarded; check Error before continuing and do not let an update silently become an insert", method.Sel.Name))
				}
				return true
			})
		}
	}
	return findings
}

func serviceORMRoot(expr ast.Expr, receiver *ast.Ident) bool {
	if call, ok := expr.(*ast.CallExpr); ok {
		if method, ok := call.Fun.(*ast.SelectorExpr); ok {
			return serviceORMRoot(method.X, receiver)
		}
		return false
	}
	field, ok := expr.(*ast.SelectorExpr)
	if !ok || field.Sel.Name != "Orm" {
		return false
	}
	owner := field.X
	if service, ok := owner.(*ast.SelectorExpr); ok && service.Sel.Name == "Service" {
		owner = service.X
	}
	id, ok := owner.(*ast.Ident)
	return ok && id.Name == receiver.Name && id.Obj != nil && id.Obj == receiver.Obj
}
