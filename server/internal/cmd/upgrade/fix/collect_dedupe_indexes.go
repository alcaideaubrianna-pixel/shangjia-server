package fix

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func ApplyYoubanPublishCollectDedupeIndexes(ctx context.Context) error {
	if !strings.EqualFold(g.DB().GetConfig().Type, "pgsql") {
		return gerror.New("采集去重索引迁移目前只支持PostgreSQL")
	}
	statements := []string{
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_collect_event_text_dedupe" ON "hg_youban_publish_collect_event" ("tenant_id", "account_id", "text_hash", "received_at" DESC, "id" DESC)`,
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_collect_event_key_dedupe" ON "hg_youban_publish_collect_event" ("tenant_id", "account_id", "dedupe_key", "received_at" DESC, "id" DESC)`,
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_collect_event_recent" ON "hg_youban_publish_collect_event" ("tenant_id", "account_id", "id" DESC)`,
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return gerror.Wrap(err, "创建采集去重索引失败")
		}
	}
	g.Log().Info(ctx, "采集去重索引创建完成")
	return nil
}
