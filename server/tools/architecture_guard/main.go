// architecture_guard prevents expansion of legacy publish infrastructure bypasses.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type entry struct {
	File       string `json:"file"`
	Function   string `json:"function"`
	Rule       string `json:"rule"`
	Expression string `json:"expression"`
	Digest     string `json:"digest"`
	Count      int    `json:"count"`
	Reason     string `json:"reason"`
}

func main() {
	root := flag.String("root", ".", "Go module root (server)")
	baseline := flag.String("baseline", "tools/architecture_guard/baseline.json", "reviewed exact legacy sites")
	inventory := flag.Bool("inventory", false, "print findings for review; never update baseline automatically")
	flag.Parse()
	actual, err := scan(*root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *inventory {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(actual); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	raw, err := os.ReadFile(filepath.Join(*root, *baseline))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var allowed []entry
	if err = json.Unmarshal(raw, &allowed); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	violations := compare(actual, allowed)
	if len(violations) > 0 {
		for _, v := range violations {
			fmt.Fprintln(os.Stderr, v)
		}
		os.Exit(1)
	}
	fmt.Printf("publish architecture guard passed (%d exact legacy expressions; no new bypasses)\n", len(actual))
}

func identity(e entry) string { return e.File + "|" + e.Function + "|" + e.Rule + "|" + e.Digest }
func compare(actual, allowed []entry) []string {
	limits := make(map[string]int)
	var failures []string
	for _, e := range allowed {
		if e.File == "" || e.Function == "" || e.Reason == "" || e.Count < 1 || digest(e.Expression) != e.Digest || strings.ContainsAny(e.File, "*?[]") || !exactFunction(e.Function) {
			failures = append(failures, "invalid or broad baseline entry: "+identity(e))
			continue
		}
		if _, exists := limits[identity(e)]; exists {
			failures = append(failures, "duplicate baseline entry: "+identity(e))
		}
		limits[identity(e)] = e.Count
	}
	for _, e := range actual {
		if e.Count > limits[identity(e)] {
			failures = append(failures, fmt.Sprintf("%s %s [%s] count=%d allowed=%d: %s", e.File, e.Function, e.Rule, e.Count, limits[identity(e)], e.Expression))
		}
	}
	return failures
}
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }

func scan(root string) ([]entry, error) {
	bases := []string{
		filepath.Join(root, "addons/youban_publish"),
		filepath.Join(root, "addons/telegram_collector"),
	}
	items := make(map[string]entry)
	visitBase := func(base string) error {
		return filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			found, err := inspect(filepath.ToSlash(rel), path)
			if err != nil {
				return err
			}
			for _, e := range found {
				key := identity(e)
				if old, ok := items[key]; ok {
					old.Count++
					items[key] = old
				} else {
					items[key] = e
				}
			}
			return nil
		})
	}
	for _, base := range bases {
		if _, err := os.Stat(base); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if err := visitBase(base); err != nil {
			return nil, err
		}
	}
	result := make([]entry, 0, len(items))
	for _, e := range items {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool { return identity(result[i]) < identity(result[j]) })
	return result, nil
}

func inspect(file, path string) ([]entry, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}
	var found []entry
	publishFile := strings.HasPrefix(file, "addons/youban_publish/")
	add := func(fn, rule string, n ast.Node) {
		var b bytes.Buffer
		_ = format.Node(&b, fset, n)
		s := b.String()
		found = append(found, entry{file, fn, rule, s, digest(s), 1, "Existing site retained for compatibility; changes require architecture review."})
	}
	imports := make(map[string]string)
	for _, im := range f.Imports {
		p, _ := strconv.Unquote(im.Path.Value)
		name := filepath.Base(p)
		if im.Name != nil {
			name = im.Name.Name
		}
		imports[name] = p
		if strings.HasPrefix(file, "addons/telegram_collector/") &&
			(strings.Contains(p, "/youban_publish") || strings.Contains(p, "/telegram_job")) {
			add("<imports>", "collector-publish-coupling", im)
		}
		if publishFile && (strings.Contains(p, "/redis") || strings.HasSuffix(p, "/gredis")) {
			add("<imports>", "redis-import", im)
		}
		if strings.Contains(file, "/controller/") && (strings.Contains(p, "/dao") || strings.Contains(p, "/logic/")) {
			add("<imports>", "controller-layer-import", im)
		}
		if name == "." {
			add("<imports>", "dot-import", im)
		}
	}
	visit := func(fn string, n ast.Node) {
		protectedContext := false
		ast.Inspect(n, func(node ast.Node) bool {
			if id, ok := node.(*ast.Ident); ok && (id.Name == "YoubanPublishTgJob" || id.Name == "telegramJobRecord" || id.Name == "telegramJobTransition" || protectedType(id.Name)) {
				protectedContext = true
			}
			return true
		})
		ast.Inspect(n, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if !publishFile {
					return true
				}
				if sel, ok := x.Fun.(*ast.SelectorExpr); ok {
					name := sel.Sel.Name
					id, _ := sel.X.(*ast.Ident)
					pkg := ""
					if id != nil {
						pkg = imports[id.Name]
					}
					if publishFile && ((pkg == "github.com/gogf/gf/v2/frame/g" && name == "Redis") || (pkg == "hotgo/internal/library/cache" && (name == "RedisClient" || strings.HasPrefix(name, "ZSet"))) || strings.HasPrefix(name, "GroupScript") || name == "Do" || name == "Eval" || name == "EvalSha" || strings.HasPrefix(name, "GroupHash") || strings.HasPrefix(name, "GroupSortedSet") || strings.HasPrefix(name, "GroupList") || strings.HasPrefix(name, "GroupString") || strings.HasPrefix(name, "GroupGeneric")) {
						add(fn, "redis-primitive", x)
					}
					if publishFile && (strings.Contains(pkg, "/redis") || strings.HasSuffix(pkg, "/gredis")) {
						add(fn, "redis-client", x)
					}
					if publishFile && protectedType(name) {
						add(fn, "state-cast", x)
					}
				}
				if publishFile {
					if id, ok := x.Fun.(*ast.Ident); ok && protectedType(id.Name) {
						add(fn, "state-cast", x)
					}
				}
			case *ast.KeyValueExpr:
				if !publishFile {
					return true
				}
				if key, ok := x.Key.(*ast.BasicLit); ok && key.Kind == token.STRING {
					value, _ := strconv.Unquote(key.Value)
					if stateField(value) && (value != "status" || protectedContext || lifecycleValue(x.Value)) {
						add(fn, "state-map-write", x)
					}
				}
			case *ast.AssignStmt:
				if !publishFile {
					return true
				}
				for _, lhs := range x.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok {
						switch sel.Sel.Name {
						case "Status", "DispatchStatus", "SendPhase", "PublishTaskStatus":
							if sel.Sel.Name == "Status" && !protectedContext && !jobObject(sel.X) {
								break
							}
							add(fn, "state-selector-write", x)
						}
					}
					if idx, ok := lhs.(*ast.IndexExpr); ok {
						if lit, ok := idx.Index.(*ast.BasicLit); ok && lit.Kind == token.STRING {
							key, _ := strconv.Unquote(lit.Value)
							if stateField(key) && (key != "status" || protectedContext || assignmentLifecycle(x)) {
								add(fn, "state-index-write", x)
							}
						}
					}
				}
			case *ast.BasicLit:
				if x.Kind == token.STRING {
					value, _ := strconv.Unquote(x.Value)
					lower := strings.ToLower(value)
					if strings.HasPrefix(file, "addons/telegram_collector/") &&
						strings.Contains(lower, " from ") &&
						(strings.Contains(lower, "tg_job") || strings.Contains(lower, "tg_outbox")) {
						add(fn, "collector-tg-state-sql", x)
					}
					if strings.HasPrefix(file, "addons/telegram_collector/") &&
						strings.Contains(lower, " from ") && strings.Contains(lower, "collect_event") &&
						!strings.Contains(lower, " where ") && !strings.Contains(lower, " limit ") {
						add(fn, "collector-global-sql-scan", x)
					}
					if publishFile && strings.Contains(lower, "update") && (strings.Contains(lower, "set status") || strings.Contains(lower, "set send_phase") || strings.Contains(lower, "set dispatch_status") || strings.Contains(lower, "set publish_task_status")) {
						add(fn, "state-sql-write", x)
					}
				}
			}
			return true
		})
	}
	for _, decl := range f.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			name := fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				var b bytes.Buffer
				_ = format.Node(&b, fset, fn.Recv.List[0].Type)
				name = b.String() + "." + name
			}
			visit(name, fn)
		} else {
			if gen, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range gen.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.Name == "telegramJobRecord" {
						if st, ok := ts.Type.(*ast.StructType); ok {
							for _, field := range st.Fields.List {
								for _, name := range field.Names {
									if name.Name == "Status" || name.Name == "DispatchStatus" || name.Name == "SendPhase" {
										if id, ok := field.Type.(*ast.Ident); ok && id.Name == "string" {
											add("type:telegramJobRecord", "untyped-job-field", field)
										}
									}
								}
							}
						}
					}
				}
			}
			visit("<declarations>", decl)
		}
	}
	return found, nil
}
func stateField(s string) bool {
	switch s {
	case "status", "dispatch_status", "send_phase", "publish_task_status":
		return true
	}
	return false
}
func protectedType(s string) bool {
	switch s {
	case "telegramJobLifecycle", "telegramSendPhaseState", "telegramDispatchState", "telegramJobSource", "JobStatus", "DispatchStatus", "SendPhase", "PublishTaskStatus":
		return true
	}
	return false
}

// Pointer receiver syntax is an exact Go identifier, never a glob exemption.
var functionPattern = regexp.MustCompile(`^(?:\*?[A-Za-z_][A-Za-z0-9_]*\.)?[A-Za-z_][A-Za-z0-9_]*$`)

func exactFunction(s string) bool {
	return s == "<imports>" || s == "<declarations>" || s == "type:telegramJobRecord" || functionPattern.MatchString(s)
}
func jobObject(n ast.Expr) bool {
	id, ok := n.(*ast.Ident)
	return ok && (id.Name == "job" || id.Name == "tgJob" || id.Name == "telegramJob")
}
func assignmentLifecycle(n *ast.AssignStmt) bool {
	for _, rhs := range n.Rhs {
		if lifecycleValue(rhs) {
			return true
		}
	}
	return false
}
func lifecycleValue(n ast.Expr) bool {
	found := false
	ast.Inspect(n, func(node ast.Node) bool {
		switch x := node.(type) {
		case *ast.Ident:
			if strings.HasPrefix(x.Name, "JobStatus") || strings.HasPrefix(x.Name, "telegramLifecycle") {
				found = true
			}
		case *ast.BasicLit:
			if x.Kind == token.STRING {
				v, _ := strconv.Unquote(x.Value)
				switch v {
				case "pending", "sending", "sent", "failed_retry", "confirming", "unknown", "superseded":
					found = true
				}
			}
		}
		return true
	})
	return found
}
