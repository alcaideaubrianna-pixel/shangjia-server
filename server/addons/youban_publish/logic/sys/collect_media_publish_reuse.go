package sys

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	pdao "hotgo/addons/youban_publish/internal/dao"
)

// Match persisted delivery identity before invoking either Telegram downloader.
// Text and profile numbers are deliberately not used as asset identities.
func (s *sSysPublish) reusePublishedCollectMedia(ctx context.Context, row *collectEventMediaRow) (bool, error) {
	if row == nil || row.TenantId <= 0 || row.SourceMessageId <= 0 {
		return false, nil
	}
	chatID := normalizeTelegramChannelChatID(row.SourceChatId)
	if chatID == "" {
		return false, nil
	}
	mediaCols := pdao.YoubanPublishMedia.Columns()
	message, err := g.DB().Model(publishTgMessageTable+" m").Safe().Ctx(ctx).
		Fields("a.id,a.file_url,a.storage_path,a.original_file_url,a.original_storage_path,a.edited_file_url,a.edited_storage_path,a.edit_status,a.md5,a.perceptual_hash,a.poster_url,a.poster_storage_path,a.tg_file_id,a.tg_thumb_file_id,a.tg_cache_status,a.tg_cache_asset_hash,a.updated_at AS asset_updated_at,m.sent_at,m.tg_file_id AS sent_file_id").
		InnerJoin(pdao.YoubanPublishMedia.Table()+" a", "a."+mediaCols.Id+"=m.media_id AND a."+mediaCols.TenantId+"=m.tenant_id").
		Where("m.tenant_id", row.TenantId).
		WhereIn("m.target_chat_id", []string{chatID, canonicalCollectSourceChatID(chatID)}).
		Where("m.tg_message_id", row.SourceMessageId).
		Where("m.status", "sent").WhereNull("m.deleted_at").
		WhereNull("a."+mediaCols.DeletedAt).
		Where("a."+mediaCols.MediaType, collectPublishMediaType(row.MediaType)).
		OrderDesc("m.id").One()
	if err != nil {
		return false, gerror.Wrap(err, "读取平台已推送媒体失败")
	}
	if message.IsEmpty() {
		return false, nil
	}
	media := newProfileMediaFromRecord(message)
	asset := media.EffectiveAsset()
	// The normal send path refreshes file_id after recording sent_at. Permit that
	// update only while its cache still identifies the same effective asset.
	updatedAt, sentAt := message["asset_updated_at"].GTime(), message["sent_at"].GTime()
	if updatedAt == nil || sentAt == nil {
		return false, nil
	}
	if updatedAt.After(sentAt) {
		hash := telegramMediaCacheAssetHash(collectPublishMediaType(row.MediaType), asset.Hash,
			normalizeMediaFileURL(message[mediaCols.PosterUrl].String(), message[mediaCols.PosterStoragePath].String()), message[mediaCols.PosterStoragePath].String(), message[mediaCols.TgThumbFileId].String())
		fileID := media.ValidTgFileIdForHash(hash)
		if fileID == "" || fileID != message["sent_file_id"].String() {
			return false, nil
		}
	}
	fileURL := normalizeMediaFileURL(asset.FileUrl, asset.StoragePath)
	md5 := strings.TrimSpace(message[mediaCols.Md5].String())
	// Historical assets without a digest continue through the repair/download path.
	if md5 == "" || fileURL == "" {
		return false, nil
	}
	posterURL := normalizeMediaFileURL(message[mediaCols.PosterUrl].String(), message[mediaCols.PosterStoragePath].String())
	phash := strings.TrimSpace(message[mediaCols.PerceptualHash].String())
	meta := make(map[string]interface{})
	_ = json.Unmarshal([]byte(row.MetaJson), &meta)
	if meta == nil {
		meta = make(map[string]interface{})
	}
	meta["reused_published_media_id"] = message[mediaCols.Id].Int64()
	metaJSON, _ := json.Marshal(meta)
	cols := pdao.YoubanPublishCollectEventMedia.Columns()
	_, err = pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Where(cols.Id, row.Id).Where(cols.TenantId, row.TenantId).
		Data(g.Map{
			cols.FileUrl: fileURL, cols.StoragePath: asset.StoragePath, cols.PosterUrl: posterURL,
			cols.FileMd5: md5, cols.FilePhash: phash, cols.CacheStatus: collectMediaCacheReady,
			cols.ErrorMessage: "", cols.CacheHit: 1, cols.UpdatedAt: gtime.Now(),
			cols.MetaJson: string(metaJSON),
		}).Update()
	if err != nil {
		return false, gerror.Wrap(err, "写入平台媒体复用结果失败")
	}
	row.FileUrl, row.StoragePath, row.PosterUrl = fileURL, asset.StoragePath, posterURL
	row.FileMd5, row.FilePhash, row.CacheStatus, row.ErrorMessage = md5, phash, collectMediaCacheReady, ""
	row.MetaJson = string(metaJSON)
	g.Log().Infof(ctx, "复用平台已推送媒体 eventMediaId:%d mediaId:%d sourceChatId:%s sourceMessageId:%d", row.Id, message[mediaCols.Id].Int64(), chatID, row.SourceMessageId)
	return true, nil
}

// Preserve the published asset's metadata, including optional absent pHash/poster.
// Reuse must not fall through to ffprobe or remote fingerprint downloads.
func reusedPublishedCollectMetadata(item collectMediaItem) *mediaAssetMetadata {
	if strings.TrimSpace(item.FileMd5) == "" || strings.TrimSpace(item.FileUrl) == "" {
		return nil
	}
	var meta struct {
		MediaID int64 `json:"reused_published_media_id"`
	}
	if json.Unmarshal([]byte(item.DebugMetaJson), &meta) != nil || meta.MediaID <= 0 {
		return nil
	}
	return &mediaAssetMetadata{MD5: item.FileMd5, PerceptualHash: item.FilePhash, PosterURL: item.PosterUrl}
}
