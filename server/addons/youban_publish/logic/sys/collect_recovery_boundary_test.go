package sys

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// Keep legacy, unscoped repair handlers out of the automatic recovery entry.
// This boundary must hold even if an obsolete deployment flag is still set.
func TestCollectAutomaticRecoveryOnlyCallsScopedHandlers(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "collect_recovery.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"recoverCollectMediaSourceScopes": true,
		"recoverCollectEvents":            true,
	}
	found := false
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != "recoverCollectOnce" {
			continue
		}
		found = true
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			receiver, ok := selector.X.(*ast.Ident)
			if ok && receiver.Name == "s" && !allowed[selector.Sel.Name] {
				t.Errorf("automatic recovery calls unreviewed handler %s", selector.Sel.Name)
			}
			return true
		})
	}
	if !found {
		t.Fatal("automatic recovery entry not found")
	}
}
