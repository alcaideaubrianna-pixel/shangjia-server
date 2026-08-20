package sys

import (
	"context"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCountCollectSourceOptionGroupsUsesSubquery(t *testing.T) {
	ctx := context.Background()
	mod := g.DB().Model("events e").Safe().Ctx(ctx)
	sql, err := gdb.ToSQL(ctx, func(sqlCtx context.Context) error {
		_, countErr := countCollectSourceOptionGroups(sqlCtx, mod)
		return countErr
	})
	if err != nil {
		t.Fatalf("build grouped count SQL: %v", err)
	}
	upperSQL := strings.ToUpper(sql)
	if !strings.Contains(upperSQL, "FROM (SELECT") || !strings.Contains(upperSQL, "COUNT(1)") {
		t.Fatalf("count must wrap grouped query: %s", sql)
	}
	if strings.Contains(upperSQL, "COUNT(E.SOURCE_ID") || strings.Contains(upperSQL, "MAX(E.TITLE)") {
		t.Fatalf("count retained list fields: %s", sql)
	}
}
