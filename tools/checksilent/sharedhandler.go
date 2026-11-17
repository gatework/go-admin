package main

import "go/ast"

const checkSharedHandlerState = "shared-handler-state"

// checkSharedHandlerStates reports a route handler method with a pointer
// receiver that assigns to the receiver's fields. A route handler is a type
// embedding api.API; routes register one value of it for every request, so
// such a method writes request state - the gin context, the tenant database -
// where a concurrent request reads it. Use a value receiver, which gets a copy
// per call, or keep the state in locals. Per-request helper types are not
// shared and are not reported.
func checkSharedHandlerStates(s *snapshot) []Finding {
	handlers := handlerTypes(s)
	var out []Finding
	for _, sf := range s.Files {
		if sf.isTest() {
			continue
		}
		for _, decl := range sf.Syntax.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			receiver, typ, ok := pointerReceiver(fn)
			if !ok || !handlers[sf.Pkg+"."+typ] {
				continue
			}
			for _, sel := range receiverFieldAssignments(fn.Body, receiver) {
				out = append(out, s.finding(Error, checkSharedHandlerState, sf, sel,
					"%s.%s assigns %s.%s through a pointer receiver; the handler is shared by every request - use a value receiver",
					typ, fn.Name.Name, receiver, sel.Sel.Name))
			}
		}
	}
	return out
}

// handlerTypes names, as "package.Type", every struct embedding API.
func handlerTypes(s *snapshot) map[string]bool {
	out := map[string]bool{}
	for _, sf := range s.Files {
		for name, st := range structTypes(sf) {
			for _, field := range st.Fields.List {
				if len(field.Names) == 0 && embeddedName(field.Type) == "API" {
					out[sf.Pkg+"."+name] = true
				}
			}
		}
	}
	return out
}

func embeddedName(t ast.Expr) string {
	switch t := t.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return t.Sel.Name
	}
	return ""
}

func pointerReceiver(fn *ast.FuncDecl) (receiver, typ string, ok bool) {
	if fn.Recv == nil || len(fn.Recv.List) != 1 || len(fn.Recv.List[0].Names) != 1 {
		return "", "", false
	}
	star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return "", "", false
	}
	id, ok := star.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}
	return fn.Recv.List[0].Names[0].Name, id.Name, true
}

func receiverFieldAssignments(body *ast.BlockStmt, receiver string) []*ast.SelectorExpr {
	var out []*ast.SelectorExpr
	ast.Inspect(body, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok {
			for _, lhs := range assign.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok {
					if id, ok := sel.X.(*ast.Ident); ok && id.Name == receiver {
						out = append(out, sel)
					}
				}
			}
		}
		return true
	})
	return out
}
