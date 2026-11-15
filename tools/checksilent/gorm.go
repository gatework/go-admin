package main

import "go/ast"

const checkGORMDiscarded = "gorm-discarded-result"

// checkDiscardedGORMResults covers embedded core Service receivers without DB
// shadows; local variables with the same name remain excluded.
func checkDiscardedGORMResults(s *snapshot) []Finding {
	services := serviceTypes(s)
	var findings []Finding
	for _, sf := range s.Files {
		if sf.isTest() {
			continue
		}
		for _, decl := range sf.Syntax.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			receiver, ok := serviceReceiver(sf, fn, services)
			if !ok {
				continue
			}
			findings = append(findings, discardedResults(s, sf, fn, receiver)...)
		}
	}
	return findings
}

func serviceTypes(s *snapshot) map[string]bool {
	services := make(map[string]bool)
	for _, sf := range s.Files {
		if sf.isTest() {
			continue
		}
		for name, st := range structTypes(sf) {
			if shadowsDatabase(st) {
				continue
			}
			for _, embed := range embeddedTypes(sf, st) {
				if embed[0] == "github.com/gatework/motor-core/sdk/service" && embed[1] == "Service" {
					services[sf.Pkg+"."+name] = true
				}
			}
		}
	}
	return services
}
func shadowsDatabase(st *ast.StructType) bool {
	for _, field := range st.Fields.List {
		for _, name := range field.Names {
			if name.Name == "DB" {
				return true
			}
		}
	}
	return false
}
func serviceReceiver(sf *sourceFile, fn *ast.FuncDecl, services map[string]bool) (*ast.Ident, bool) {
	if fn.Recv == nil || fn.Body == nil || len(fn.Recv.List) != 1 {
		return nil, false
	}
	receiver := fn.Recv.List[0]
	if len(receiver.Names) != 1 {
		return nil, false
	}
	typ := receiver.Type
	if ptr, ok := typ.(*ast.StarExpr); ok {
		typ = ptr.X
	}
	name, ok := typ.(*ast.Ident)
	if !ok || !services[sf.Pkg+"."+name.Name] {
		return nil, false
	}
	return receiver.Names[0], true
}
func discardedResults(s *snapshot, sf *sourceFile, fn *ast.FuncDecl, receiver *ast.Ident) []Finding {
	terminals := map[string]bool{"First": true, "Last": true, "Take": true, "Find": true, "Scan": true, "Count": true, "Create": true, "Save": true, "Updates": true, "Update": true, "UpdateColumn": true, "UpdateColumns": true, "Delete": true, "Exec": true, "FirstOrCreate": true, "FirstOrInit": true, "Transaction": true, "Commit": true, "Rows": true}
	var findings []Finding
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
		if serviceORMRoot(method.X, receiver) {
			findings = append(findings, s.finding(Error, checkGORMDiscarded, sf, statement, "database result from %s is discarded; check Error before continuing and do not let an update silently become an insert", method.Sel.Name))
		}
		return true
	})
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
	if !ok || field.Sel.Name != "DB" {
		return false
	}
	owner := field.X
	if service, ok := owner.(*ast.SelectorExpr); ok && service.Sel.Name == "Service" {
		owner = service.X
	}
	id, ok := owner.(*ast.Ident)
	return ok && id.Name == receiver.Name && id.Obj != nil && id.Obj == receiver.Obj
}
