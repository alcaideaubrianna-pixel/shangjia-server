package sys

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCollectorPackageKeepsCollectionBoundary prevents the collector package
// from growing a direct dependency on publish or Telegram send APIs. The
// collector may persist collection data and hand off a delivery record; the
// publish adapter and publish workers own downstream publication.
func TestCollectorPackageKeepsCollectionBoundary(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture guard location")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	fset := token.NewFileSet()

	var violations []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(importPath, "/youban_publish") {
				violations = append(violations, path+": imports "+importPath)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			name := selector.Sel.Name
			for _, forbidden := range []string{
				"SendMessage", "SendMedia", "SendPhoto", "SendVideo", "SendDocument",
				"ForwardMessages", "EditMessage", "DeleteMessages",
			} {
				if name == forbidden {
					violations = append(violations, path+": calls "+name)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) > 0 {
		t.Fatalf("collector boundary violations:\n%s", strings.Join(violations, "\n"))
	}
}

// TestCollectorRuntimeRoleBoundary ensures the delivery runtime remains
// attached to push-worker. A collector-only deployment must ingest and persist
// source events without becoming a Telegram delivery consumer.
func TestCollectorRuntimeRoleBoundary(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture guard location")
	}
	mainPath := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", "main.go"))
	content, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	source := string(content)
	if !strings.Contains(source, "runrole.PushWorker") || !strings.Contains(source, "StartDeliveryRuntime") {
		t.Fatalf("delivery runtime is no longer explicitly gated by push-worker")
	}
	if strings.Contains(source, "CollectorWorker) {\n\t\tcollectorservice.Collector().StartDeliveryRuntime") {
		t.Fatalf("collector-worker must not start the delivery runtime")
	}
}
