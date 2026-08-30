package fix

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

// ApplyContentProfilePublicIndexes aligns the partial-index predicates with
// every import status accepted by the public profile query.
func ApplyContentProfilePublicIndexes(ctx context.Context) error {
	if !strings.EqualFold(g.DB().GetConfig().Type, "pgsql") {
		return gerror.New("公开资料索引迁移目前只支持PostgreSQL")
	}

	statements := []string{
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_content_profile_public_latest_v2" ON "hg_content_profile" ("source_created_at" DESC, "source_note_id" DESC, "id" DESC) WHERE "status"=1 AND "review_status"='approved' AND "import_status" IN ('imported','duplicate','feiniu_sync','collect') AND "visibility" IN ('public','member_only') AND "deleted_at" IS NULL`,
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_content_profile_public_area_latest_v2" ON "hg_content_profile" ("province", "city", "source_created_at" DESC, "source_note_id" DESC, "id" DESC) WHERE "status"=1 AND "review_status"='approved' AND "import_status" IN ('imported','duplicate','feiniu_sync','collect') AND "visibility" IN ('public','member_only') AND "deleted_at" IS NULL`,
		`CREATE INDEX CONCURRENTLY IF NOT EXISTS "idx_content_media_public_image_profile" ON "hg_content_media" ("profile_id") WHERE "status"=1 AND "media_type"='image' AND COALESCE("display_storage_path", '')<>''`,
	}
	for _, statement := range statements {
		if _, err := g.DB().Exec(ctx, statement); err != nil {
			return gerror.Wrap(err, "创建公开资料列表索引失败")
		}
	}
	g.Log().Info(ctx, "公开资料列表索引创建完成")
	return nil
}
