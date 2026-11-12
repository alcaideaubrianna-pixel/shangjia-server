package sys

import (
	"context"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCountCollectSourceOptionGroupsOnlySelectsGroupKeys(t *testing.T) {
	ctx := context.Background()
	err := g.DB().Transaction(ctx, func(txCtx context.Context, tx gdb.TX) error {
		if _, execErr := tx.Exec("CREATE TEMP TABLE hg_events(source_chat_id text) ON COMMIT DROP"); execErr != nil {
			return execErr
		}
		if _, execErr := tx.Exec("INSERT INTO hg_events(source_chat_id) VALUES ('chat-a'),('chat-a'),('chat-b')"); execErr != nil {
			return execErr
		}
		mod := tx.Model("hg_events e").Safe().Ctx(txCtx)
		count, countErr := countCollectSourceOptionGroups(txCtx, mod)
		if countErr != nil {
			return countErr
		}
		if count != 2 {
			t.Fatalf("group count=%d, want 2", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
