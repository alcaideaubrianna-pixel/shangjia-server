package sys

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/model/input/sysin"
)

// Read the production expression rather than copying a fixed SQL predicate
// into the regression fixture; removing its grouping must fail this test.
func deliveryRecoveryConditionFormat(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate recovery source")
	}
	node, err := parser.ParseFile(token.NewFileSet(), filepath.Join(filepath.Dir(file), "delivery_recovery_cursor.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var format string
	for _, decl := range node.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok || function.Name.Name != "deliveryRecoveryPage" {
			continue
		}
		ast.Inspect(function.Body, func(node ast.Node) bool {
			// The scope-based recovery uses the grouped expression directly
			// in Where rather than assigning a fmt.Sprintf condition.
			if call, ok := node.(*ast.CallExpr); ok && len(call.Args) > 0 {
				if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "Where" {
					if literal, ok := call.Args[0].(*ast.BasicLit); ok && literal.Kind == token.STRING {
						value, unquoteErr := strconv.Unquote(literal.Value)
						if unquoteErr == nil && strings.Contains(value, " OR ") && strings.Contains(value, "next_run_at") {
							format = value
							return false
						}
					}
				}
			}
			assignment, ok := node.(*ast.AssignStmt)
			if !ok || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
				return true
			}
			name, ok := assignment.Lhs[0].(*ast.Ident)
			if !ok || name.Name != "condition" {
				return true
			}
			call, ok := assignment.Rhs[0].(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				return true
			}
			format, err = strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			return false
		})
	}
	if format == "" {
		t.Fatal("delivery recovery condition not found")
	}
	return format
}

func TestDeliveryRecoveryConditionGroupsBothBranches(t *testing.T) {
	format := strings.TrimSpace(deliveryRecoveryConditionFormat(t))
	if !strings.HasPrefix(format, "((") || !strings.HasSuffix(format, "))") {
		t.Fatalf("both OR branches must be grouped before attempt/pool filters: %s", format)
	}
}

func TestDeliveryRecoveryPoolAndAttemptFiltersApplyToPendingLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1024 || n > 65535 {
		t.Fatal("invalid disposable PostgreSQL port")
	}
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	defer func() { _ = g.DB().GetCore().ClearTableFieldsAll(context.Background()) }()
	format := deliveryRecoveryConditionFormat(t)
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_tg_collector_delivery(id bigint PRIMARY KEY,status text,scheduler_pool text,attempt_count int,updated_at timestamp,next_run_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_tg_collector_delivery VALUES
			 (1,'pending','realtime',0,'2026-11-16',NULL),
			 (2,'pending','history',0,'2026-11-16',NULL),
			 (3,'pending','realtime',5,'2026-11-16',NULL),
			 (4,'failed_retry','realtime',0,'2026-11-16','2026-11-16'),
			 (5,'failed_retry','history',0,'2026-11-16','2026-11-16'),
			 (6,'failed_retry','realtime',5,'2026-11-16','2026-11-16'),
			 (7,'pending','realtime',0,'2026-11-18',NULL),
			 (8,'failed_retry','realtime',0,'2026-11-16','2026-11-18')`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if err := g.DB().GetCore().ClearTableFieldsAll(ctx); err != nil {
			return err
		}
		cols := dao.TgCollectorDelivery.Columns()
		condition := format
		if strings.Contains(format, "%s") {
			condition = fmt.Sprintf(format, cols.Status, cols.UpdatedAt, cols.Status, cols.NextRunAt, cols.NextRunAt)
		}
		for _, tc := range []struct {
			pool string
			want []int64
		}{{"realtime", []int64{1, 4}}, {"history", []int64{2, 5}}} {
			rows, err := dao.TgCollectorDelivery.Ctx(ctx).Fields(cols.Id).
				Where(condition, sysin.DeliveryStatusPending, gtime.New("2026-11-17"),
					sysin.DeliveryStatusFailedRetry, gtime.New("2026-11-17")).
				WhereLT(cols.AttemptCount, deliveryMaxAttempts).
				Where("scheduler_pool = ?", tc.pool).
				OrderAsc(cols.Id).Limit(10).All()
			if err != nil {
				return err
			}
			var ids []int64
			for _, row := range rows {
				ids = append(ids, row[cols.Id].Int64())
			}
			if !reflect.DeepEqual(ids, tc.want) {
				t.Fatalf("pool=%s got=%v want=%v: pending bypassed attempt/pool filter", tc.pool, ids, tc.want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
