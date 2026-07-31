package fix

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

type collectMediaSourceMeta struct {
	Kind          string `json:"kind"`
	Id            int64  `json:"id"`
	AccessHash    int64  `json:"accessHash"`
	FileReference []byte `json:"fileReference"`
	ThumbSize     string `json:"thumbSize"`
	MimeType      string `json:"mimeType"`
	DCID          int    `json:"dcId"`
	Size          int64  `json:"size"`
}

func NormalizeYoubanPublishCollectMediaMeta(ctx context.Context) error {
	result, err := g.DB().Exec(ctx, `
WITH source AS (
  SELECT id, meta_json::jsonb AS meta
  FROM hg_youban_publish_collect_event_media
  WHERE source_media_id = 0
    AND meta_json IS NOT NULL
    AND btrim(meta_json) <> ''
    AND btrim(meta_json) LIKE '{%'
)
UPDATE hg_youban_publish_collect_event_media AS media
SET source_kind = COALESCE(source.meta->>'kind', ''),
    source_media_id = (source.meta->>'id')::bigint,
    source_access_hash = COALESCE(NULLIF(source.meta->>'accessHash', ''), '0')::bigint,
    source_file_reference = CASE
      WHEN COALESCE(source.meta->>'fileReference', '') = '' THEN NULL
      ELSE decode(source.meta->>'fileReference', 'base64')
    END,
    source_thumb_size = COALESCE(source.meta->>'thumbSize', ''),
    source_mime_type = COALESCE(source.meta->>'mimeType', ''),
    source_dc_id = COALESCE(NULLIF(source.meta->>'dcId', ''), '0')::integer,
    source_size = COALESCE(NULLIF(source.meta->>'size', ''), '0')::bigint,
    updated_at = NOW()
FROM source
WHERE media.id = source.id
  AND COALESCE(source.meta->>'id', '') ~ '^[0-9]+$'
`)
	if err != nil {
		return gerror.Wrap(err, "批量迁移采集媒体元数据失败")
	}
	updated, _ := result.RowsAffected()
	g.Log().Infof(ctx, "采集媒体元数据批量迁移完成 updated:%d", updated)
	return nil
}
