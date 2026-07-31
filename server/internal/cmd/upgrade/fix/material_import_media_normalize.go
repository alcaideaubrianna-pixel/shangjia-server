package fix

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func NormalizeYoubanPublishMaterialImportMedia(ctx context.Context) error {
	var migrated int64
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		expectedValue, txErr := tx.GetValue(`
SELECT COALESCE(SUM(jsonb_array_length(media_json::jsonb)), 0)
FROM hg_youban_publish_material_import_group
WHERE media_json IS NOT NULL
  AND btrim(media_json) NOT IN ('', '[]')
  AND btrim(media_json) LIKE '[%'
`)
		if txErr != nil {
			return gerror.Wrap(txErr, "统计旧TG导入媒体失败")
		}
		expected := expectedValue.Int64()
		if _, txErr = tx.Exec(`
DELETE FROM hg_youban_publish_material_import_group_media
WHERE group_id IN (
  SELECT id FROM hg_youban_publish_material_import_group
  WHERE media_json IS NOT NULL
    AND btrim(media_json) NOT IN ('', '[]')
    AND btrim(media_json) LIKE '[%'
)
`); txErr != nil {
			return gerror.Wrap(txErr, "清理TG导入旧媒体关系失败")
		}
		result, txErr := tx.Exec(`
WITH expanded AS (
  SELECT groups.id AS group_id,
         groups.task_id,
         groups.tenant_id,
         groups.account_id,
         media.value AS item,
         media.ordinality::integer AS sort_index,
         COALESCE(
           NULLIF(media.value->>'debugMetaJson', ''),
           NULLIF(media.value->>'metaJson', ''),
           '{}'
         )::jsonb AS meta
  FROM hg_youban_publish_material_import_group AS groups
  CROSS JOIN LATERAL jsonb_array_elements(groups.media_json::jsonb) WITH ORDINALITY AS media(value, ordinality)
  WHERE groups.media_json IS NOT NULL
    AND btrim(groups.media_json) NOT IN ('', '[]')
    AND btrim(groups.media_json) LIKE '[%'
)
INSERT INTO hg_youban_publish_material_import_group_media (
  task_id, group_id, tenant_id, account_id, purpose, media_type, sort_index,
  source_file_id, file_url, storage_path, poster_url,
  source_kind, source_media_id, source_access_hash, source_file_reference,
  source_thumb_size, source_mime_type, source_dc_id, source_size,
  file_md5, file_phash, created_at, updated_at
)
SELECT task_id,
       group_id,
       tenant_id,
       account_id,
       CASE WHEN item->>'purpose' = 'verify' THEN 'verify' ELSE 'display' END,
       COALESCE(item->>'type', ''),
       sort_index,
       COALESCE(item->>'fileId', ''),
       COALESCE(item->>'fileUrl', ''),
       COALESCE(item->>'storagePath', ''),
       COALESCE(item->>'posterUrl', ''),
       COALESCE(NULLIF(item->>'sourceKind', ''), meta->>'kind', ''),
       COALESCE(NULLIF(item->>'sourceMediaId', ''), NULLIF(meta->>'id', ''), '0')::bigint,
       COALESCE(NULLIF(item->>'sourceAccessHash', ''), NULLIF(meta->>'accessHash', ''), '0')::bigint,
       CASE
         WHEN COALESCE(NULLIF(item->>'sourceFileReference', ''), meta->>'fileReference', '') = '' THEN NULL
         ELSE decode(COALESCE(NULLIF(item->>'sourceFileReference', ''), meta->>'fileReference'), 'base64')
       END,
       COALESCE(NULLIF(item->>'sourceThumbSize', ''), meta->>'thumbSize', ''),
       COALESCE(NULLIF(item->>'sourceMimeType', ''), meta->>'mimeType', ''),
       COALESCE(NULLIF(item->>'sourceDcId', ''), NULLIF(meta->>'dcId', ''), '0')::integer,
       COALESCE(NULLIF(item->>'sourceSize', ''), NULLIF(meta->>'size', ''), '0')::bigint,
       COALESCE(item->>'fileMd5', ''),
       COALESCE(item->>'filePhash', ''),
       NOW(),
       NOW()
FROM expanded
`)
		if txErr != nil {
			return gerror.Wrap(txErr, "批量迁移TG导入媒体失败")
		}
		migrated, _ = result.RowsAffected()
		if migrated != expected {
			return gerror.Newf("TG导入媒体迁移数量不一致 expected:%d actual:%d", expected, migrated)
		}
		if _, txErr = tx.Exec(`ALTER TABLE "hg_youban_publish_material_import_group" DROP COLUMN IF EXISTS "media_json"`); txErr != nil {
			return gerror.Wrap(txErr, "删除TG导入旧媒体JSON字段失败")
		}
		return nil
	})
	if err != nil {
		return err
	}
	g.Log().Infof(ctx, "TG导入媒体批量结构化迁移完成 media:%d", migrated)
	return nil
}
