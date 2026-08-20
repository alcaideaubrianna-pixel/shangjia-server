package sys

import (
	"context"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCountCollectSourceOptionGroupsOnlySelectsGroupKeys(t *testing.T) {
	ctx := context.Background()
	mod := g.DB().Model("events e").Safe().Ctx(ctx)
	sql, err := gdb.ToSQL(ctx, func(sqlCtx context.Context) error {
		_, countErr := countCollectSourceOptionGroups(sqlCtx, mod)
		return countErr
	})
	generated := sql
	if generated == "" && err != nil {
		generated = err.Error()
	}
	upperSQL := strings.ToUpper(generated)
	if !strings.Contains(upperSQL, "SELECT E.SOURCE_ID,E.SOURCE_CHAT_ID") || !strings.Contains(upperSQL, "GROUP BY") {
		t.Fatalf("count must select grouped keys: %s", generated)
	}
	if strings.Contains(upperSQL, "COUNT(") || strings.Contains(upperSQL, "MAX(") || strings.Contains(upperSQL, "COLLECT_SOURCE_GROUPS") {
		t.Fatalf("count retained list fields: %s", generated)
	}
}
