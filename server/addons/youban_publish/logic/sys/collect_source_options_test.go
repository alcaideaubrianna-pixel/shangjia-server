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
	var generated string
	var err error
	err = g.DB().Transaction(ctx, func(txCtx context.Context, tx gdb.TX) error {
		if _, execErr := tx.Exec("CREATE TEMP TABLE events(source_chat_id text) ON COMMIT DROP"); execErr != nil {
			return execErr
		}
		mod := tx.Model("events e").Safe().Ctx(txCtx)
		generated, err = gdb.ToSQL(txCtx, func(sqlCtx context.Context) error {
			_, countErr := countCollectSourceOptionGroups(sqlCtx, mod)
			return countErr
		})
		return err
	})
	if generated == "" && err != nil {
		generated = err.Error()
	}
	upperSQL := strings.ToUpper(generated)
	if !strings.Contains(upperSQL, "SELECT E.SOURCE_CHAT_ID") || !strings.Contains(upperSQL, "GROUP BY") {
		t.Fatalf("count must select grouped keys: %s", generated)
	}
	if strings.Contains(upperSQL, "COUNT(") || strings.Contains(upperSQL, "MAX(") || strings.Contains(upperSQL, "COLLECT_SOURCE_GROUPS") {
		t.Fatalf("count retained list fields: %s", generated)
	}
}
