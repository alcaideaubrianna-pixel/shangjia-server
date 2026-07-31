package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const materialImportGroupMediaTable = "hg_youban_publish_material_import_group_media"

func (s *sSysPublish) materialImportGroupMediaItems(ctx context.Context, groupId int64) ([]collectMediaItem, error) {
	if groupId <= 0 {
		return nil, nil
	}
	rows, err := g.DB().Model(materialImportGroupMediaTable).Safe().Ctx(ctx).
		Where("group_id", groupId).OrderAsc("sort_index").OrderAsc("id").All()
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG导入分组媒体失败")
	}
	items := make([]collectMediaItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, collectMediaItem{
			Type: row["media_type"].String(), Purpose: row["purpose"].String(), FileId: row["source_file_id"].String(),
			FileUrl: row["file_url"].String(), StoragePath: row["storage_path"].String(), PosterUrl: row["poster_url"].String(),
			FileMd5: row["file_md5"].String(), FilePhash: row["file_phash"].String(),
			SourceKind: row["source_kind"].String(), SourceMediaId: row["source_media_id"].Int64(),
			SourceAccessHash: row["source_access_hash"].Int64(), SourceFileReference: row["source_file_reference"].Bytes(),
			SourceThumbSize: row["source_thumb_size"].String(), SourceMimeType: row["source_mime_type"].String(),
			SourceDCId: row["source_dc_id"].Int(), SourceSize: row["source_size"].Int64(),
		})
	}
	return items, nil
}

func (s *sSysPublish) replaceMaterialImportGroupMedia(ctx context.Context, groupId, taskId, tenantId, accountId int64, items []collectMediaItem) error {
	items = mergeCollectMediaItems(nil, items)
	return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Model(materialImportGroupMediaTable).Ctx(ctx).Where("group_id", groupId).Delete(); err != nil {
			return err
		}
		for index, item := range items {
			item = normalizeCollectMediaItem(item)
			if collectMediaSourceKey(item) == "" || collectPublishMediaType(item.Type) == "" {
				continue
			}
			if _, err := tx.Model(materialImportGroupMediaTable).Ctx(ctx).Data(g.Map{
				"task_id": taskId, "group_id": groupId, "tenant_id": tenantId, "account_id": accountId,
				"purpose": materialImportMediaPurpose(item), "media_type": item.Type, "sort_index": index + 1,
				"source_file_id": item.FileId, "file_url": item.FileUrl, "storage_path": item.StoragePath, "poster_url": item.PosterUrl,
				"source_kind": item.SourceKind, "source_media_id": item.SourceMediaId, "source_access_hash": item.SourceAccessHash,
				"source_file_reference": item.SourceFileReference, "source_thumb_size": item.SourceThumbSize,
				"source_mime_type": item.SourceMimeType, "source_dc_id": item.SourceDCId, "source_size": item.SourceSize,
				"file_md5": item.FileMd5, "file_phash": item.FilePhash, "created_at": gtime.Now(), "updated_at": gtime.Now(),
			}).Insert(); err != nil {
				return err
			}
		}
		return nil
	})
}
