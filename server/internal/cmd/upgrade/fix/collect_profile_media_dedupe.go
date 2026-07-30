package fix

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const collectProfileMediaDedupeBatchSize = 500

// DedupeYoubanPublishCollectProfileMedia removes duplicate active media rows
// produced by historical concurrent collection rebuilds. Different assets at
// the same sort index are preserved.
func DedupeYoubanPublishCollectProfileMedia(ctx context.Context) error {
	total := 0
	for {
		rows, err := g.DB().GetAll(ctx, `
WITH ranked AS (
    SELECT id, profile_id,
           ROW_NUMBER() OVER (
               PARTITION BY profile_id, purpose, sort_index,
                            COALESCE(NULLIF(md5, ''), NULLIF(storage_path, ''), NULLIF(file_url, ''), id::text)
               ORDER BY id
           ) AS row_no
    FROM hg_youban_publish_media
    WHERE deleted_at IS NULL
)
SELECT id, profile_id
FROM ranked
WHERE row_no > 1
ORDER BY id
LIMIT ?`, collectProfileMediaDedupeBatchSize)
		if err != nil {
			return gerror.Wrap(err, "读取历史采集重复媒体失败")
		}
		if len(rows) == 0 {
			break
		}
		mediaIds := make([]int64, 0, len(rows))
		profileIds := make([]int64, 0, len(rows))
		for _, row := range rows {
			mediaIds = append(mediaIds, row["id"].Int64())
			profileIds = append(profileIds, row["profile_id"].Int64())
		}
		now := gtime.Now()
		if _, err = g.DB().Model("hg_youban_publish_media_phash_bucket").Safe().Ctx(ctx).WhereIn("media_id", mediaIds).Delete(); err != nil {
			return gerror.Wrap(err, "清理重复媒体PHash分桶失败")
		}
		if _, err = g.DB().Model("hg_youban_publish_media_phash_lsh").Safe().Ctx(ctx).WhereIn("media_id", mediaIds).Delete(); err != nil {
			return gerror.Wrap(err, "清理重复媒体LSH索引失败")
		}
		if _, err = g.DB().Model("hg_youban_publish_media").Safe().Ctx(ctx).
			WhereIn("id", mediaIds).
			WhereNull("deleted_at").
			Data(g.Map{"deleted_at": now, "updated_at": now}).Update(); err != nil {
			return gerror.Wrap(err, "删除历史采集重复媒体失败")
		}
		profileIds = uniqueFixInt64s(profileIds)
		for _, profileId := range profileIds {
			if profileId <= 0 {
				continue
			}
			if _, err = g.DB().Exec(ctx, `
UPDATE hg_content_profile
SET image_count = (SELECT COUNT(*) FROM hg_youban_publish_media WHERE profile_id = ? AND media_type = 'image' AND deleted_at IS NULL),
    video_count = (SELECT COUNT(*) FROM hg_youban_publish_media WHERE profile_id = ? AND media_type = 'video' AND deleted_at IS NULL),
    updated_at = ?
WHERE id = ?`, profileId, profileId, now, profileId); err != nil {
				return gerror.Wrapf(err, "更新资料媒体数量失败 profileId:%d", profileId)
			}
		}
		total += len(mediaIds)
		g.Log().Infof(ctx, "历史采集重复媒体清理进度 batch:%d total:%d", len(mediaIds), total)
	}
	g.Log().Infof(ctx, "历史采集重复媒体清理完成 total:%d", total)
	return nil
}

func uniqueFixInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
