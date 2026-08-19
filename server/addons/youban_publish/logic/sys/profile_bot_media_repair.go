package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) rebuildBotProfileMedia(ctx context.Context, profileIds []int64, dryRun bool) (*sysin.CollectProfileMediaRebuildResult, error) {
	result := &sysin.CollectProfileMediaRebuildResult{ProfileIDs: make([]int64, 0, len(profileIds))}
	for _, profileId := range profileIds {
		var profile struct {
			ID         int64  `json:"id"`
			SourceType string `json:"source_type"`
		}
		if err := g.DB().Model("hg_content_profile").Safe().Ctx(ctx).Fields("id,source_type").Where("id", profileId).WhereNull("deleted_at").Scan(&profile); err != nil {
			return result, gerror.Wrap(err, "读取Bot资料失败")
		}
		if profile.ID == 0 || profile.SourceType != "youban_publish" {
			continue
		}
		result.Candidates++
		result.ProfileIDs = append(result.ProfileIDs, profileId)
		rows, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Fields("id,tenant_id,media_type,tg_file_id,storage_path,file_url").
			Where("profile_id", profileId).WhereNull("deleted_at").OrderAsc("sort_index").All()
		if err != nil {
			return result, gerror.Wrap(err, "读取Bot资料媒体失败")
		}
		recoverable := 0
		for _, row := range rows {
			if strings.TrimSpace(row["storage_path"].String()) != "" || !strings.HasPrefix(strings.TrimSpace(row["tg_file_id"].String()), "copy:") {
				continue
			}
			chatID, messageID, parseErr := parseBotMediaCopyReference(row["tg_file_id"].String())
			if parseErr != nil {
				return result, parseErr
			}
			var source struct {
				BotID   int64  `json:"bot_id"`
				RawJSON string `json:"raw_json"`
			}
			if err = g.DB().Model("hg_youban_bot_message").Safe().Ctx(ctx).Fields("bot_id,raw_json").Where("chat_id", chatID).Where("message_id", messageID).Scan(&source); err != nil {
				return result, gerror.Wrap(err, "读取Bot原始媒体消息失败")
			}
			fileID, fileName, sourceErr := botMessageMediaFileID(source.RawJSON, row["media_type"].String())
			if sourceErr != nil {
				return result, gerror.Wrapf(sourceErr, "Bot原始媒体不可恢复 mediaId:%d", row["id"].Int64())
			}
			if source.BotID <= 0 {
				return result, gerror.Newf("Bot原始媒体缺少Bot ID mediaId:%d", row["id"].Int64())
			}
			recoverable++
			if dryRun {
				continue
			}
			downloaded, downloadErr := s.downloadBotTelegramMedia(ctx, row["tenant_id"].Int64(), source.BotID, collectMediaItem{Type: row["media_type"].String(), FileId: fileID})
			if downloadErr != nil {
				return result, gerror.Wrapf(downloadErr, "恢复Bot资料媒体失败 mediaId:%d", row["id"].Int64())
			}
			item := downloaded.Item
			if _, err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).Where("id", row["id"].Int64()).Data(g.Map{
				"attachment_id": downloaded.AttachmentId, "name": firstNonEmpty(fileName, row["media_type"].String()),
				"file_url": downloaded.FileUrl, "storage_path": item.StoragePath,
				"original_attachment_id": downloaded.AttachmentId, "original_file_url": downloaded.FileUrl, "original_storage_path": item.StoragePath,
				"tg_file_id": fileID, "md5": item.FileMd5, "size": item.SourceSize, "mime_type": item.SourceMimeType,
				"tg_cache_asset_hash": mediaAssetHash(item.FileMd5, item.StoragePath, downloaded.FileUrl), "tg_cache_status": tgCacheStatusValid,
				"processing_status": mediaProcessingUploaded, "processing_error": "", "updated_at": gtime.Now(),
			}).Update(); err != nil {
				return result, gerror.Wrapf(err, "更新Bot资料媒体失败 mediaId:%d", row["id"].Int64())
			}
		}
		if recoverable > 0 {
			result.Recoverable++
			if !dryRun {
				if err = s.syncBotProfileMedia(ctx, profileId); err != nil {
					return result, err
				}
				result.Requeued++
			}
		}
	}
	return result, nil
}

func parseBotMediaCopyReference(value string) (string, int, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 3 || parts[0] != "copy" {
		return "", 0, gerror.New("Bot媒体复制引用无效")
	}
	messageID, err := strconv.Atoi(parts[2])
	if err != nil || messageID <= 0 {
		return "", 0, gerror.New("Bot媒体消息ID无效")
	}
	return parts[1], messageID, nil
}

func botMessageMediaFileID(rawJSON string, mediaType string) (string, string, error) {
	var message models.Message
	if err := json.Unmarshal([]byte(rawJSON), &message); err != nil {
		return "", "", gerror.Wrap(err, "解析Bot原始媒体消息失败")
	}
	if strings.EqualFold(strings.TrimSpace(mediaType), "video") && message.Video != nil {
		fileID := strings.TrimSpace(message.Video.FileID)
		if fileID == "" {
			return "", "", gerror.New("Bot原始视频缺少file_id")
		}
		return fileID, firstNonEmpty(message.Video.FileName, fmt.Sprintf("video_%d.mp4", message.ID)), nil
	}
	if len(message.Photo) > 0 {
		photo := message.Photo[len(message.Photo)-1]
		fileID := strings.TrimSpace(photo.FileID)
		if fileID == "" {
			return "", "", gerror.New("Bot原始图片缺少file_id")
		}
		return fileID, fmt.Sprintf("photo_%d.jpg", message.ID), nil
	}
	return "", "", gerror.New("Bot原始消息不包含对应媒体")
}
