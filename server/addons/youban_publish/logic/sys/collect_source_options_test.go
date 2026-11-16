package sys

import (
	"context"
	"os"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCountCollectSourceOptionGroupsCountsDistinctChannels(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires isolated PostgreSQL at 127.0.0.1:55484")
	}
	if sqlReviewSubprocess(t) {
		return
	}
	previousDB := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previousDB) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"}})
	ctx := context.Background()
	err := g.DB().Transaction(ctx, func(txCtx context.Context, tx gdb.TX) error {
		if _, execErr := tx.Exec("CREATE TEMP TABLE hg_events(telegram_chat_id text) ON COMMIT DROP"); execErr != nil {
			return execErr
		}
		if _, execErr := tx.Exec("INSERT INTO hg_events(source_chat_id) VALUES ('chat-a'),('chat-a'),('chat-b')"); execErr != nil {
			return execErr
		}
		mod := tx.Model("hg_events c").Safe().Ctx(txCtx)
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

func TestCollectSourceOptionsRelationsLocalPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_SQL_REVIEW_LOCAL_PG") != "1" {
		t.Skip("requires isolated PostgreSQL at 127.0.0.1:55484")
	}
	if sqlReviewSubprocess(t) {
		return
	}
	previousDB := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previousDB) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55484", User: "test", Name: "postgres"}})
	ctx := context.Background()
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			`CREATE TEMP TABLE hg_youban_publish_collect_channel(id bigint,tenant_id bigint,telegram_chat_id text,title text,username text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_source(id bigint,tenant_id bigint,account_id bigint,title text,source_username text,deleted_at timestamp) ON COMMIT DROP`,
			`CREATE TEMP TABLE hg_youban_publish_collect_account_channel(tenant_id bigint,account_id bigint,source_id bigint,channel_id bigint,deleted_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_channel VALUES(1,1,'-10042','频道甲','alpha',NULL),(2,1,'-10043','频道乙','beta',NULL),(3,2,'-10044','其他租户','other',NULL)`,
			`INSERT INTO hg_youban_publish_collect_source VALUES(10,1,5,'源甲','',NULL),(11,1,5,'重复来源','',NULL),(12,1,6,'其他账户','',NULL),(13,2,5,'其他租户','',NULL)`,
			`INSERT INTO hg_youban_publish_collect_account_channel VALUES(1,5,10,1,NULL),(1,5,11,1,NULL),(1,6,12,2,NULL),(2,5,13,3,NULL),(1,5,10,2,CURRENT_TIMESTAMP)`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		for _, tc := range []struct {
			keyword string
			want    int
		}{{"", 1}, {"@alpha", 1}, {"beta", 0}} {
			count, err := countCollectSourceOptionGroups(ctx, collectSourceOptionsModel(ctx, 1, 5, tc.keyword))
			if err != nil {
				return err
			}
			if count != tc.want {
				t.Fatalf("keyword=%q count=%d want=%d", tc.keyword, count, tc.want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
