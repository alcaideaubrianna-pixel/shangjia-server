package sys

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
)

func (s *sSysPublish) sendTelegramMediaSet(ctx context.Context, bot *tgbot.Bot, chatId string, purpose string, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	if len(media) == 0 {
		return nil, nil
	}
	if len(media) == 1 {
		return s.sendTelegramSingleMedia(ctx, bot, chatId, purpose, caption, media[0])
	}
	group, mediaIds, assetHashes, closers := s.telegramInputMediaGroup(media, caption)
	defer closeTelegramMediaFiles(closers)
	if len(group) == 0 {
		return nil, nil
	}
	msgs, err := bot.SendMediaGroup(ctx, &tgbot.SendMediaGroupParams{
		ChatID: chatId,
		Media:  group,
	})
	if err != nil {
		return nil, err
	}
	return telegramSentMessagesFromGroup(msgs, purpose, mediaIds, assetHashes), nil
}

func (s *sSysPublish) sendTelegramSingleMedia(ctx context.Context, bot *tgbot.Bot, chatId string, purpose string, caption string, media *telegramMediaItem) ([]*telegramSentMessage, error) {
	input, closer, err := telegramInputFile(media)
	if err != nil {
		return nil, err
	}
	if closer != nil {
		defer closer.Close()
	}
	switch media.MediaType {
	case "video":
		thumbnail, thumbnailCloser, err := telegramVideoThumbnail(media)
		if err != nil {
			return nil, err
		}
		if thumbnailCloser != nil {
			defer thumbnailCloser.Close()
		}
		msg, err := bot.SendVideo(ctx, &tgbot.SendVideoParams{
			ChatID:    chatId,
			Video:     input,
			Thumbnail: thumbnail,
			Caption:   caption,
			ParseMode: telegramMediaParseMode(caption),
		})
		if err != nil {
			return nil, err
		}
		return telegramSentMessagesFromSingle(msg, purpose, media.Id, media.AssetHash)
	default:
		msg, err := bot.SendPhoto(ctx, &tgbot.SendPhotoParams{
			ChatID:    chatId,
			Photo:     input,
			Caption:   caption,
			ParseMode: telegramMediaParseMode(caption),
		})
		if err != nil {
			return nil, err
		}
		return telegramSentMessagesFromSingle(msg, purpose, media.Id, media.AssetHash)
	}
}

func (s *sSysPublish) telegramInputMediaGroup(media []*telegramMediaItem, caption string) ([]models.InputMedia, []int64, []string, []io.Closer) {
	group := make([]models.InputMedia, 0, len(media))
	mediaIds := make([]int64, 0, len(media))
	assetHashes := make([]string, 0, len(media))
	closers := make([]io.Closer, 0, len(media))
	for _, item := range media {
		source, attachment, closer, err := telegramInputMediaSource(item)
		if err != nil || source == "" {
			continue
		}
		if closer != nil {
			closers = append(closers, closer)
		}
		itemCaption := ""
		if len(group) == 0 {
			itemCaption = caption
		}
		if item.MediaType == "video" {
			group = append(group, &models.InputMediaVideo{Media: source, Caption: itemCaption, ParseMode: telegramMediaParseMode(itemCaption), Thumbnail: telegramVideoGroupThumbnail(item), MediaAttachment: attachment})
		} else {
			group = append(group, &models.InputMediaPhoto{Media: source, Caption: itemCaption, ParseMode: telegramMediaParseMode(itemCaption), MediaAttachment: attachment})
		}
		mediaIds = append(mediaIds, item.Id)
		assetHashes = append(assetHashes, item.AssetHash)
	}
	return group, mediaIds, assetHashes, closers
}

func telegramMediaParseMode(caption string) models.ParseMode {
	if strings.TrimSpace(caption) == "" {
		return ""
	}
	return models.ParseModeHTML
}

func telegramInputFile(media *telegramMediaItem) (models.InputFile, io.Closer, error) {
	if media == nil {
		return nil, nil, gerror.New("媒体文件为空")
	}
	if source := strings.TrimSpace(media.TgFileId); source != "" {
		return &models.InputFileString{Data: source}, nil, nil
	}
	path := strings.TrimSpace(media.StoragePath)
	if path != "" {
		localPath := resolveTelegramLocalPath(path)
		file, err := os.Open(localPath)
		if err == nil {
			return &models.InputFileUpload{Filename: filepath.Base(localPath), Data: file}, file, nil
		}
		if strings.TrimSpace(media.FileUrl) == "" || isLocalTelegramURL(media.FileUrl) {
			return nil, nil, gerror.Wrapf(err, "打开本地媒体文件失败:%s", localPath)
		}
	}
	if source := strings.TrimSpace(media.FileUrl); source != "" {
		return &models.InputFileString{Data: source}, nil, nil
	}
	return nil, nil, gerror.New("媒体文件地址为空")
}

func telegramInputMediaSource(media *telegramMediaItem) (string, io.Reader, io.Closer, error) {
	if media == nil {
		return "", nil, nil, gerror.New("媒体文件为空")
	}
	if source := strings.TrimSpace(media.TgFileId); source != "" {
		return source, nil, nil, nil
	}
	path := strings.TrimSpace(media.StoragePath)
	if path != "" {
		localPath := resolveTelegramLocalPath(path)
		file, err := os.Open(localPath)
		if err == nil {
			attachName := fmt.Sprintf("media_%d_%s", media.Id, filepath.Base(localPath))
			return "attach://" + attachName, file, file, nil
		}
		if strings.TrimSpace(media.FileUrl) == "" || isLocalTelegramURL(media.FileUrl) {
			return "", nil, nil, gerror.Wrapf(err, "打开本地媒体文件失败:%s", localPath)
		}
	}
	if source := strings.TrimSpace(media.FileUrl); source != "" {
		return source, nil, nil, nil
	}
	return "", nil, nil, gerror.New("媒体文件地址为空")
}

func telegramVideoThumbnail(media *telegramMediaItem) (models.InputFile, io.Closer, error) {
	if media == nil || media.MediaType != "video" {
		return nil, nil, nil
	}
	if source := strings.TrimSpace(media.TgThumbFileId); source != "" {
		return &models.InputFileString{Data: source}, nil, nil
	}
	if path := strings.TrimSpace(media.PosterStoragePath); path != "" {
		localPath := resolveTelegramLocalPath(path)
		file, err := os.Open(localPath)
		if err == nil {
			return &models.InputFileUpload{Filename: filepath.Base(localPath), Data: file}, file, nil
		}
		if strings.TrimSpace(media.PosterUrl) == "" || isLocalTelegramURL(media.PosterUrl) {
			return nil, nil, gerror.Wrapf(err, "打开本地视频封面失败:%s", localPath)
		}
	}
	if source := strings.TrimSpace(media.PosterUrl); source != "" {
		return &models.InputFileString{Data: source}, nil, nil
	}
	return nil, nil, nil
}

func telegramVideoGroupThumbnail(media *telegramMediaItem) models.InputFile {
	if media == nil || media.MediaType != "video" {
		return nil
	}
	if source := strings.TrimSpace(media.TgThumbFileId); source != "" {
		return &models.InputFileString{Data: source}
	}
	if source := strings.TrimSpace(media.PosterUrl); source != "" {
		return &models.InputFileString{Data: source}
	}
	return nil
}

func resolveTelegramLocalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) && fileExists(path) {
		return path
	}
	trimmed := strings.TrimLeft(path, "/")
	candidates := []string{
		path,
		trimmed,
		filepath.Join("resource/public", trimmed),
		filepath.Join("storage", trimmed),
	}
	for _, item := range candidates {
		if fileExists(item) {
			return item
		}
	}
	return filepath.Join("resource/public", trimmed)
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isLocalTelegramURL(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	return strings.Contains(value, "://localhost") ||
		strings.Contains(value, "://127.") ||
		strings.Contains(value, "://0.0.0.0") ||
		strings.Contains(value, "://[::1]")
}

func closeTelegramMediaFiles(closers []io.Closer) {
	for _, closer := range closers {
		if closer != nil {
			_ = closer.Close()
		}
	}
}

func telegramSentMessagesFromGroup(msgs []*models.Message, purpose string, mediaIds []int64, assetHashes []string) []*telegramSentMessage {
	list := make([]*telegramSentMessage, 0, len(msgs))
	for i, msg := range msgs {
		if msg == nil {
			continue
		}
		mediaId := int64(0)
		if i < len(mediaIds) {
			mediaId = mediaIds[i]
		}
		assetHash := ""
		if i < len(assetHashes) {
			assetHash = assetHashes[i]
		}
		list = append(list, &telegramSentMessage{
			MessageId:    int64(msg.ID),
			MediaGroupId: msg.MediaGroupID,
			Purpose:      purpose,
			MediaId:      mediaId,
			TgFileId:     telegramMessageFileId(msg),
			AssetHash:    assetHash,
		})
	}
	return list
}

func telegramSentMessagesFromSingle(msg *models.Message, purpose string, mediaId int64, assetHash string) ([]*telegramSentMessage, error) {
	if msg == nil {
		return nil, nil
	}
	return []*telegramSentMessage{{
		MessageId:    int64(msg.ID),
		MediaGroupId: msg.MediaGroupID,
		Purpose:      purpose,
		MediaId:      mediaId,
		TgFileId:     telegramMessageFileId(msg),
		AssetHash:    assetHash,
	}}, nil
}
