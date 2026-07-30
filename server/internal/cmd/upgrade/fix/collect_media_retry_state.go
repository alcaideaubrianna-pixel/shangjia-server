package fix

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

// ApplyYoubanPublishCollectMediaRetryState adds durable retry scheduling for
// collection media tasks. The PostgreSQL index is created concurrently so the
// migration can run while normal reads and writes continue.
func ApplyYoubanPublishCollectMediaRetryState(ctx context.Context) error {
	if !strings.EqualFold(g.DB().GetConfig().Type, "pgsql") {
		return gerror.New("采集媒体重试状态迁移目前只支持PostgreSQL")
	}
	if _, err := g.DB().Exec(ctx, `ALTER TABLE "hg_youban_publish_collect_event_media" ADD COLUMN IF NOT EXISTS "next_retry_at" timestamp DEFAULT NULL`); err != nil {
		return gerror.Wrap(err, "新增采集媒体下次重试时间失败")
	}
	if _, err := g.DB().Exec(ctx, `
UPDATE "hg_youban_publish_collect_event_media"
SET "next_retry_at" = NOW() + (("id" % 7200) * INTERVAL '1 second')
WHERE "cache_status" = 'pending' AND "next_retry_at" IS NULL`); err != nil {
		return gerror.Wrap(err, "错峰安排历史采集媒体重试失败")
	}
	if _, err := g.DB().Exec(ctx, `CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_collect_event_media_event_retry" ON "hg_youban_publish_collect_event_media" ("event_id", "cache_status", "next_retry_at", "id")`); err != nil {
		return gerror.Wrap(err, "创建采集媒体重试索引失败")
	}
	g.Log().Info(ctx, "采集媒体重试状态迁移完成")
	return nil
}
