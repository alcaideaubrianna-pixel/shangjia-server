package sys

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/database/gdb"
)

// Guard the MCP ownership boundary without calling an API that creates real
// publication jobs. Both owner arguments must come from the loaded profile,
// rather than an unscoped zero or a caller-supplied tenant/account.
func TestAIOpsOfflineRepublishUsesLoadedOwner(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "ai_ops.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var method *ast.FuncDecl
	for _, declaration := range file.Decls {
		if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Name.Name == "AIOpsRepublishProfiles" {
			method = fn
			break
		}
	}
	if method == nil {
		t.Fatal("MCP republish entry point missing")
	}
	found := 0
	ast.Inspect(method.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		method, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || method.Sel.Name != "updateProfileStatus" {
			return true
		}
		found++
		if len(call.Args) != 4 {
			t.Fatalf("unexpected updateProfileStatus arguments: %d", len(call.Args))
		}
		for index, column := range map[int]string{2: "tenant_id", 3: "account_id"} {
			if !republishReadsLoadedOwner(call.Args[index], column) {
				t.Errorf("offline republish argument %d must use loaded profile %s", index, column)
			}
		}
		return true
	})
	if found != 1 {
		t.Fatalf("expected one canonical offline status operation, got %d", found)
	}
}

func republishReadsLoadedOwner(expression ast.Expr, column string) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return false
	}
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || method.Sel.Name != "Int64" {
		return false
	}
	index, ok := method.X.(*ast.IndexExpr)
	if !ok {
		return false
	}
	record, ok := index.X.(*ast.Ident)
	if !ok || record.Name != "profile" {
		return false
	}
	key, ok := index.Index.(*ast.BasicLit)
	if !ok || key.Kind != token.STRING {
		return false
	}
	value, err := strconv.Unquote(key.Value)
	return err == nil && value == column
}

func TestAIOpsRepublishAlwaysUsesManualPriority(t *testing.T) {
	for _, operation := range []string{"ai-republish:813124:request", " AI-REPUBLISH:813124:request "} {
		for _, stored := range []int{0, tgJobPriorityDefault, tgJobPriorityBulk, tgJobPriorityHistory} {
			job := telegramJobRecord{OperationNo: operation, Priority: stored}
			if got := telegramJobPriorityValue(job); got != tgJobPriorityInteractive {
				t.Errorf("operation=%q stored=%d priority=%d, want manual=%d", operation, stored, got, tgJobPriorityInteractive)
			}
			if got := telegramQueueNameByPriority(telegramJobPriorityValue(job)); got != tgQueueNameInteractive {
				t.Errorf("manual republish routed to %q", got)
			}
		}
	}
	if got := telegramJobPriorityValue(telegramJobRecord{OperationNo: "ai-republish-other:813124", Priority: tgJobPriorityDefault}); got != tgJobPriorityDefault {
		t.Fatalf("unrelated operation inherited manual priority: %d", got)
	}
	if got := telegramJobPriorityValue(telegramJobRecord{OperationNo: "ai-republish:813124", Status: "unknown"}); got != tgJobPriorityUrgent {
		t.Fatalf("unknown result must retain reconciliation priority: %d", got)
	}
	for _, alias := range []string{"", "j"} {
		sql := telegramJobEffectivePrioritySQL(alias)
		if !strings.Contains(sql, "LIKE 'ai-republish:%' THEN 1") || strings.Contains(sql, "%!") {
			t.Fatalf("manual priority SQL missing or malformed: %s", sql)
		}
	}
}

// Optional SQL/Go parity check uses only a fixed loopback PostgreSQL connection
// and SELECT expressions, with no application DSN or publication consumers.
func TestAIOpsRepublishPrioritySQLPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_LOCAL_HEAD_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL on 127.0.0.1:35432")
	}
	db, err := gdb.New(gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, job := range []telegramJobRecord{
		{OperationNo: "ai-republish:813124", Status: "pending", Priority: tgJobPriorityDefault},
		{OperationNo: " AI-REPUBLISH:813124 ", Status: "pending", Priority: tgJobPriorityBulk},
		{OperationNo: "ai-republish:813124", Status: "unknown", Priority: tgJobPriorityDefault},
		{OperationNo: "ai-republish-other:813124", Status: "pending", Priority: tgJobPriorityDefault},
		{OperationNo: "collect:813124", Status: "pending", Priority: tgJobPriorityHistory},
	} {
		value, err := db.GetValue(ctx, "SELECT "+telegramJobEffectivePrioritySQL("j")+" FROM (SELECT ?::text AS operation_no, ?::text AS status, ?::integer AS priority) j", job.OperationNo, job.Status.String(), job.Priority)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := value.Int(), telegramJobPriorityValue(job); got != want {
			t.Errorf("SQL priority=%d Go priority=%d for %+v", got, want, job)
		}
	}
}
