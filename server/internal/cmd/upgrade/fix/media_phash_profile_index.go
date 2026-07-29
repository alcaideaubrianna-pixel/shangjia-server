package fix

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

const (
	mediaPHashBucketProfileIndex = "idx_ybp_media_phash_bucket_profile_id"
	mediaPHashLshProfileIndex    = "idx_ybp_media_phash_lsh_profile_id"
)

// ApplyYoubanPublishMediaPHashProfileIndexes creates the indexes required by
// profile cleanup. It is intentionally an explicit maintenance command because
// PostgreSQL builds these indexes outside the normal request path.
func ApplyYoubanPublishMediaPHashProfileIndexes(ctx context.Context) error {
	if !strings.EqualFold(g.DB().GetConfig().Type, "pgsql") {
		return gerror.New("媒体PHash资料索引迁移目前只支持PostgreSQL")
	}

	for _, statement := range []string{
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_media_phash_bucket_profile_id" ON "hg_youban_publish_media_phash_bucket" ("profile_id")`,
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_ybp_media_phash_lsh_profile_id" ON "hg_youban_publish_media_phash_lsh" ("profile_id")`,
	} {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return gerror.Wrap(err, "创建媒体PHash资料索引失败")
		}
	}
	g.Log().Info(ctx, "媒体PHash资料索引创建完成")
	return nil
}
