package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) rebuildBotProfileMedia(ctx context.Context, profileIds []int64, dryRun bool) (*sysin.CollectProfileMediaRebuildResult, error) {
	result := &sysin.CollectProfileMediaRebuildResult{ProfileIDs: make([]int64, 0, len(profileIds))}
	for _, profileId := range profileIds {
		var profile struct {
			ID         int64       `json:"id"`
			SourceType string      `json:"source_type"`
			CreatedAt  *gtime.Time `json:"created_at"`
		}
		if err := g.DB().Model("hg_content_profile").Safe().Ctx(ctx).Fields("id,source_type,created_at").Where("id", profileId).WhereNull("deleted_at").Scan(&profile); err != nil {
			return result, gerror.Wrap(err, "读取Bot资料失败")
		}
		if profile.ID == 0 || profile.SourceType != "youban_publish" {
			continue
		}
		result.Candidates++
		result.ProfileIDs = append(result.ProfileIDs, profileId)
		rows, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Fields("id,tenant_id,account_id,media_type,name,tg_file_id,storage_path,file_url").
			Where("profile_id", profileId).WhereNull("deleted_at").OrderAsc("sort_index").All()
		if err != nil {
			return result, gerror.Wrap(err, "读取Bot资料媒体失败")
		}
		recoverable := 0
		for _, row := range rows {
			if strings.TrimSpace(row["storage_path"].String()) != "" {
				continue
			}
			source, sourceErr := s.findBotProfileMediaSource(ctx, row, profile.CreatedAt)
			fileID, fileName := "", strings.TrimSpace(row["name"].String())
			if sourceErr == nil {
				fileID, fileName, sourceErr = botMessageMediaFileID(source.RawJSON, row["media_type"].String())
			}
			if sourceErr != nil {
				source, fileID, sourceErr = s.findBotProfileMediaDirectSource(ctx, row)
			}
			if sourceErr != nil {
				return result, gerror.Wrapf(sourceErr, "定位Bot原始媒体失败 mediaId:%d", row["id"].Int64())
			}
			if source.BotID <= 0 || strings.TrimSpace(source.BotToken) == "" {
				return result, gerror.Newf("Bot原始媒体缺少Bot ID mediaId:%d", row["id"].Int64())
			}
			recoverable++
			if dryRun {
				continue
			}
			downloaded, downloadErr := s.downloadBotTelegramMediaWithToken(ctx, source.BotID, source.BotToken, collectMediaItem{Type: row["media_type"].String(), FileId: fileID})
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

func (s *sSysPublish) findBotProfileMediaDirectSource(ctx context.Context, media gdb.Record) (*botProfileMediaSource, string, error) {
	fileID := strings.TrimSpace(media["tg_file_id"].String())
	if fileID == "" || strings.HasPrefix(fileID, "copy:") {
		return nil, "", gerror.New("Bot媒体缺少可直接下载的File ID")
	}
	token, err := botTokenFromFileURL(media["file_url"].String())
	if err != nil {
		return nil, "", err
	}
	var source botProfileMediaSource
	if err = g.DB().Model("hg_youban_bot_bot").Ctx(ctx).
		Fields("id AS bot_id,bot_token").Where("bot_token", token).OrderDesc("id").Limit(1).Scan(&source); err != nil {
		return nil, "", gerror.Wrap(err, "读取历史Bot下载凭证失败")
	}
	if source.BotID <= 0 || strings.TrimSpace(source.BotToken) == "" {
		return nil, "", gerror.New("历史Bot下载凭证不存在")
	}
	return &source, fileID, nil
}

func botTokenFromFileURL(value string) (string, error) {
	const marker = "/file/bot"
	value = strings.TrimSpace(value)
	start := strings.Index(value, marker)
	if start < 0 {
		return "", gerror.New("Bot临时文件地址无效")
	}
	remainder := value[start+len(marker):]
	end := strings.IndexByte(remainder, '/')
	if end <= 0 {
		return "", gerror.New("Bot临时文件地址缺少Token")
	}
	token := strings.TrimSpace(remainder[:end])
	if !strings.Contains(token, ":") {
		return "", gerror.New("Bot临时文件地址Token无效")
	}
	return token, nil
}

type botProfileMediaSource struct {
	BotID    int64  `json:"bot_id"`
	BotToken string `json:"bot_token"`
	RawJSON  string `json:"raw_json"`
}

func (s *sSysPublish) findBotProfileMediaSource(ctx context.Context, media gdb.Record, profileCreatedAt *gtime.Time) (*botProfileMediaSource, error) {
	if strings.HasPrefix(strings.TrimSpace(media["tg_file_id"].String()), "copy:") {
		chatID, messageID, err := parseBotMediaCopyReference(media["tg_file_id"].String())
		if err != nil {
			return nil, err
		}
		var source botProfileMediaSource
		if err = g.DB().Model("hg_youban_bot_message m").Safe().Ctx(ctx).
			Fields("m.bot_id,m.raw_json,b.bot_token").
			LeftJoin("hg_youban_bot_bot b", "b.id=m.bot_id AND b.deleted_at IS NULL").
			Where("m.chat_id", chatID).Where("m.message_id", messageID).Scan(&source); err != nil {
			return nil, gerror.Wrap(err, "读取Bot原始媒体消息失败")
		}
		return &source, nil
	}
	if profileCreatedAt == nil || strings.TrimSpace(media["name"].String()) == "" {
		return nil, gerror.New("Bot媒体缺少可回溯的创建时间或文件名")
	}
	var sessions []struct {
		BotID       int64       `json:"bot_id"`
		ChatID      string      `json:"chat_id"`
		PayloadJSON string      `json:"payload_json"`
		CreatedAt   *gtime.Time `json:"created_at"`
		UpdatedAt   *gtime.Time `json:"updated_at"`
	}
	if err := g.DB().Model("hg_youban_bot_profile_session").Safe().Ctx(ctx).
		Fields("bot_id,chat_id,payload_json,created_at,updated_at").
		Where("tenant_id", media["tenant_id"].Int64()).Where("account_id", media["account_id"].Int64()).
		WhereBetween("created_at", profileCreatedAt.Add(-15*time.Minute), profileCreatedAt.Add(2*time.Minute)).
		OrderDesc("id").Limit(50).Scan(&sessions); err != nil {
		return nil, gerror.Wrap(err, "读取Bot资料创建会话失败")
	}
	name := strings.TrimSpace(media["name"].String())
	for _, session := range sessions {
		if !strings.Contains(session.PayloadJSON, name) || session.CreatedAt == nil || session.UpdatedAt == nil {
			continue
		}
		var messages []*botProfileMediaSource
		if err := g.DB().Model("hg_youban_bot_message m").Safe().Ctx(ctx).
			Fields("m.bot_id,m.raw_json,b.bot_token").
			LeftJoin("hg_youban_bot_bot b", "b.id=m.bot_id AND b.deleted_at IS NULL").
			Where("m.bot_id", session.BotID).Where("m.chat_id", session.ChatID).
			WhereBetween("m.created_at", session.CreatedAt.Add(-time.Minute), session.UpdatedAt.Add(time.Minute)).
			OrderAsc("m.message_id").Scan(&messages); err != nil {
			return nil, gerror.Wrap(err, "读取Bot会话媒体消息失败")
		}
		for _, message := range messages {
			_, fileName, err := botMessageMediaFileID(message.RawJSON, media["media_type"].String())
			if err == nil && strings.EqualFold(strings.TrimSpace(fileName), name) {
				return message, nil
			}
		}
	}
	return nil, gerror.Newf("未找到Bot原始媒体消息 name:%s", name)
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
