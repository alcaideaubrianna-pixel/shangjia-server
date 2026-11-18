package sys

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// Keep the ingress boundary ahead of all side effects, including relation
// repair and edit handling. Run without database or collector consumers.
func TestCollectorSelfLoopGuardPrecedesSideEffects(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "telegram_collector_adapter.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ingestCollectorBotDelivery", "ingestCollectorAccountDelivery"} {
		t.Run(name, func(t *testing.T) {
			var method *ast.FuncDecl
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == name {
					method = fn
				}
			}
			if method == nil {
				t.Fatal("missing delivery entry")
			}
			guardIndex := -1
			var call *ast.CallExpr
			for i, statement := range method.Body.List {
				assignment, ok := statement.(*ast.AssignStmt)
				if !ok || len(assignment.Rhs) != 1 {
					continue
				}
				candidate, ok := assignment.Rhs[0].(*ast.CallExpr)
				if !ok {
					continue
				}
				selector, ok := candidate.Fun.(*ast.SelectorExpr)
				if ok && selector.Sel.Name == "collectMessageFromAccountPublishChannel" {
					guardIndex, call = i, candidate
					break
				}
			}
			if guardIndex < 0 {
				t.Fatal("missing shared tenant publish target guard")
			}
			// Only global-disabled and cache-only policy early returns may
			// precede SQL. Source loading, metadata and relationship writes
			// must remain behind the live target guard.
			prefix := []string{}
			for _, statement := range method.Body.List[:guardIndex] {
				prefix = append(prefix, collectorSelfLoopReadOnlyEarlyReturn(t, statement))
			}
			wantPrefix := []string{"global"}
			if name == "ingestCollectorBotDelivery" {
				wantPrefix = append(wantPrefix, "cached_policy")
			}
			if len(prefix) != len(wantPrefix) {
				t.Fatalf("unexpected pre-guard statements: %v", prefix)
			}
			for i := range wantPrefix {
				if prefix[i] != wantPrefix[i] {
					t.Fatalf("unexpected pre-guard order: %v", prefix)
				}
			}
			for i, field := range map[int]string{1: "TenantID", 3: "SourceChatID", 4: "SourceMessageID"} {
				arg, ok := call.Args[i].(*ast.SelectorExpr)
				if !ok || arg.Sel.Name != field {
					t.Fatalf("guard argument %d must use delivery.%s", i, field)
				}
			}
			// Errors must propagate for retry; blocked deliveries must terminate
			// before relation/metadata/event writes, with a structured reason.
			for offset, condition := range []string{"err", "blocked"} {
				branch, ok := method.Body.List[guardIndex+1+offset].(*ast.IfStmt)
				if !ok {
					t.Fatalf("missing %s branch", condition)
				}
				ret, ok := branch.Body.List[len(branch.Body.List)-1].(*ast.ReturnStmt)
				if !ok || len(ret.Results) != 1 {
					t.Fatalf("%s branch must return", condition)
				}
				if condition == "err" {
					if _, ok := ret.Results[0].(*ast.CallExpr); !ok {
						t.Fatal("guard error must be wrapped and returned, never acknowledged")
					}
				} else {
					logged := false
					ast.Inspect(branch.Body, func(node ast.Node) bool {
						if lit, ok := node.(*ast.BasicLit); ok && lit.Value == `"self_loop_publish_target"` {
							logged = true
						}
						return true
					})
					if !logged {
						t.Fatal("blocked delivery must log self_loop_publish_target")
					}
				}
			}
			calls := 0
			ast.Inspect(method.Body, func(node ast.Node) bool {
				if sel, ok := node.(*ast.SelectorExpr); ok && sel.Sel.Name == "collectMessageFromAccountPublishChannel" {
					calls++
				}
				return true
			})
			if calls != 1 {
				t.Fatalf("guard calls=%d, want one lookup per delivery", calls)
			}
		})
	}
}

func collectorSelfLoopReadOnlyEarlyReturn(t *testing.T, statement ast.Stmt) string {
	t.Helper()
	branch, ok := statement.(*ast.IfStmt)
	if !ok || branch.Else != nil || len(branch.Body.List) == 0 {
		t.Fatal("only read-only early-return gates may precede the live guard")
	}
	kind := ""
	if branch.Init == nil {
		condition, ok := branch.Cond.(*ast.UnaryExpr)
		if !ok || condition.Op != token.NOT {
			t.Fatal("expected global disabled condition")
		}
		call, ok := condition.X.(*ast.CallExpr)
		if !ok {
			t.Fatal("missing global enabled call")
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "collectGlobalEnabled" {
			t.Fatal("unexpected pre-guard global gate")
		}
		kind = "global"
	} else {
		assignment, ok := branch.Init.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			t.Fatal("expected cache-only policy initialization")
		}
		decision, ok := assignment.Lhs[0].(*ast.Ident)
		if !ok || decision.Name != "decision" {
			t.Fatal("unexpected policy variable")
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok {
			t.Fatal("missing cached policy call")
		}
		function, ok := call.Fun.(*ast.Ident)
		if !ok || function.Name != "cachedCollectorDeliveryDecision" {
			t.Fatal("pre-guard policy must use cache-only classification")
		}
		condition, ok := branch.Cond.(*ast.CallExpr)
		if !ok {
			t.Fatal("expected policy deferral condition")
		}
		selector, ok := condition.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Defer" {
			t.Fatal("policy gate must only return deferred decisions")
		}
		receiver, ok := selector.X.(*ast.Ident)
		if !ok || receiver.Name != decision.Name {
			t.Fatal("policy condition must check its cached decision")
		}
		kind = "cached_policy"
	}
	last := len(branch.Body.List) - 1
	ret, ok := branch.Body.List[last].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		t.Fatal("pre-guard rejection must return nil")
	}
	value, ok := ret.Results[0].(*ast.Ident)
	if !ok || value.Name != "nil" {
		t.Fatal("pre-guard rejection must return nil")
	}
	for _, bodyStatement := range branch.Body.List[:last] {
		if _, ok := bodyStatement.(*ast.ExprStmt); !ok {
			t.Fatal("pre-guard rejection may only log before returning")
		}
		ast.Inspect(bodyStatement, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				function, ok := call.Fun.(*ast.Ident)
				if !ok || (function.Name != "logCollectorDeliverySkipped" && function.Name != "logPublishCollectorDeliveryStage" && function.Name != "string") {
					t.Fatal("unexpected side effect before live target guard")
				}
			}
			return true
		})
	}
	return kind
}
