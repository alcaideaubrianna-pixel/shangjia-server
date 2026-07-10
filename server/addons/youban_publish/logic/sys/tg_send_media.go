package sys

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
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
	if telegramMediaSetHasCopyRef(media) {
		if strings.TrimSpace(caption) == "" {
			return s.copyTelegramMediaSet(ctx, bot, chatId, purpose, caption, media)
		}
		media = telegramMediaSetWithoutTgFileId(media)
	}
	group, mediaIds, assetHashes, closers, err := s.telegramInputMediaGroup(media, caption)
	defer closeTelegramMediaFiles(closers)
	if err != nil {
		return nil, err
	}
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
	if ref, ok := telegramCopyMediaRefFromFileId(media.TgFileId); ok {
		return s.copyTelegramSingleMedia(ctx, bot, chatId, purpose, caption, media, ref)
	}
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
			ChatID:            chatId,
			Video:             input,
			Thumbnail:         thumbnail,
			Caption:           caption,
			ParseMode:         telegramMediaParseMode(caption),
			SupportsStreaming: true,
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

func (s *sSysPublish) copyTelegramMediaSet(ctx context.Context, bot *tgbot.Bot, chatId string, purpose string, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	if len(media) == 0 {
		return nil, nil
	}
	if len(media) == 1 {
		ref, ok := telegramCopyMediaRefFromFileId(media[0].TgFileId)
		if !ok {
			return s.sendTelegramSingleMedia(ctx, bot, chatId, purpose, caption, media[0])
		}
		return s.copyTelegramSingleMedia(ctx, bot, chatId, purpose, caption, media[0], ref)
	}
	copied, ok, err := s.copyTelegramMediaGroup(ctx, bot, chatId, purpose, caption, media)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, gerror.New("多媒体组备份引用不完整，禁止逐条发送以避免打散原消息组")
	}
	return copied, nil
}

func (s *sSysPublish) copyTelegramMediaGroup(ctx context.Context, bot *tgbot.Bot, chatId string, purpose string, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, bool, error) {
	if len(media) <= 1 {
		return nil, false, nil
	}
	if strings.TrimSpace(caption) != "" {
		return nil, false, nil
	}
	fromChatId, messageIds, ok := telegramCopyMediaGroupRefs(media)
	if !ok {
		return nil, false, nil
	}
	copied, err := bot.CopyMessages(ctx, &tgbot.CopyMessagesParams{
		ChatID:        chatId,
		FromChatID:    fromChatId,
		MessageIDs:    messageIds,
		RemoveCaption: true,
	})
	if err != nil {
		return nil, true, err
	}
	messages := telegramSentMessagesFromCopiedIDs(copied, purpose, media)
	if len(messages) != len(media) {
		s.cleanupTelegramSentMessages(ctx, bot, chatId, messages, "复制媒体组返回数量不完整")
		return nil, true, gerror.Newf("复制媒体组返回数量不完整，期望:%d 实际:%d", len(media), len(messages))
	}
	return messages, true, nil
}

func telegramSentMessagesFromCopiedIDs(copied []models.MessageID, purpose string, media []*telegramMediaItem) []*telegramSentMessage {
	messages := make([]*telegramSentMessage, 0, len(copied))
	for index, msg := range copied {
		if msg.ID <= 0 {
			continue
		}
		if index >= len(media) {
			continue
		}
		item := media[index]
		messages = append(messages, &telegramSentMessage{
			MessageId: int64(msg.ID),
			Purpose:   purpose,
			MediaId:   item.Id,
			TgFileId:  item.TgFileId,
			AssetHash: item.AssetHash,
		})
	}
	return messages
}

func (s *sSysPublish) copyTelegramSingleMedia(ctx context.Context, bot *tgbot.Bot, chatId string, purpose string, caption string, media *telegramMediaItem, ref telegramCopyMediaRef) ([]*telegramSentMessage, error) {
	if strings.TrimSpace(caption) == "" {
		return s.copyTelegramSingleMediaWithoutCaption(ctx, bot, chatId, purpose, media, ref)
	}
	msg, err := bot.CopyMessage(ctx, &tgbot.CopyMessageParams{
		ChatID:     chatId,
		FromChatID: ref.ChatId,
		MessageID:  ref.MessageId,
		Caption:    caption,
		ParseMode:  telegramMediaParseMode(caption),
	})
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, nil
	}
	return []*telegramSentMessage{{
		MessageId: int64(msg.ID),
		Purpose:   purpose,
		MediaId:   media.Id,
		TgFileId:  media.TgFileId,
		AssetHash: media.AssetHash,
	}}, nil
}

func (s *sSysPublish) copyTelegramSingleMediaWithoutCaption(ctx context.Context, bot *tgbot.Bot, chatId string, purpose string, media *telegramMediaItem, ref telegramCopyMediaRef) ([]*telegramSentMessage, error) {
	copied, err := bot.CopyMessages(ctx, &tgbot.CopyMessagesParams{
		ChatID:        chatId,
		FromChatID:    ref.ChatId,
		MessageIDs:    []int{ref.MessageId},
		RemoveCaption: true,
	})
	if err != nil {
		return nil, err
	}
	if len(copied) == 0 || copied[0].ID <= 0 {
		return nil, nil
	}
	return []*telegramSentMessage{{
		MessageId: int64(copied[0].ID),
		Purpose:   purpose,
		MediaId:   media.Id,
		TgFileId:  media.TgFileId,
		AssetHash: media.AssetHash,
	}}, nil
}

func (s *sSysPublish) telegramInputMediaGroup(media []*telegramMediaItem, caption string) ([]models.InputMedia, []int64, []string, []io.Closer, error) {
	group := make([]models.InputMedia, 0, len(media))
	mediaIds := make([]int64, 0, len(media))
	assetHashes := make([]string, 0, len(media))
	closers := make([]io.Closer, 0, len(media))
	for _, item := range media {
		source, attachment, closer, err := telegramInputMediaSource(item)
		if err != nil {
			closeTelegramMediaFiles(closers)
			return nil, nil, nil, nil, err
		}
		if source == "" {
			closeTelegramMediaFiles(closers)
			return nil, nil, nil, nil, gerror.New("媒体文件地址为空")
		}
		if closer != nil {
			closers = append(closers, closer)
		}
		itemCaption := ""
		if len(group) == 0 {
			itemCaption = caption
		}
		if item.MediaType == "video" {
			group = append(group, &models.InputMediaVideo{Media: source, Caption: itemCaption, ParseMode: telegramMediaParseMode(itemCaption), Thumbnail: telegramVideoGroupThumbnail(item), SupportsStreaming: true, MediaAttachment: attachment})
		} else {
			group = append(group, &models.InputMediaPhoto{Media: source, Caption: itemCaption, ParseMode: telegramMediaParseMode(itemCaption), MediaAttachment: attachment})
		}
		mediaIds = append(mediaIds, item.Id)
		assetHashes = append(assetHashes, item.AssetHash)
	}
	return group, mediaIds, assetHashes, closers, nil
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
			return &models.InputFileUpload{Filename: telegramUploadFilename(media, localPath), Data: file}, file, nil
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

func telegramMediaSetHasCopyRef(media []*telegramMediaItem) bool {
	for _, item := range media {
		if item == nil {
			continue
		}
		if _, ok := telegramCopyMediaRefFromFileId(item.TgFileId); ok {
			return true
		}
	}
	return false
}

func telegramMediaSetWithoutTgFileId(media []*telegramMediaItem) []*telegramMediaItem {
	list := make([]*telegramMediaItem, 0, len(media))
	for _, item := range media {
		if item == nil {
			list = append(list, item)
			continue
		}
		cloned := *item
		cloned.TgFileId = ""
		cloned.TgThumbFileId = ""
		list = append(list, &cloned)
	}
	return list
}

func telegramCopyMediaGroupRefs(media []*telegramMediaItem) (string, []int, bool) {
	fromChatId := ""
	messageIds := make([]int, 0, len(media))
	for _, item := range media {
		if item == nil {
			return "", nil, false
		}
		ref, ok := telegramCopyMediaRefFromFileId(item.TgFileId)
		if !ok {
			return "", nil, false
		}
		if fromChatId == "" {
			fromChatId = ref.ChatId
		}
		if fromChatId != ref.ChatId {
			return "", nil, false
		}
		messageIds = append(messageIds, ref.MessageId)
	}
	return fromChatId, messageIds, len(messageIds) == len(media)
}

func telegramCopyMediaFileId(chatId string, messageId int) string {
	return fmt.Sprintf("copy:%s:%d", normalizeTelegramChannelChatID(chatId), messageId)
}

func telegramCopyMediaRefFromFileId(fileId string) (telegramCopyMediaRef, bool) {
	fileId = strings.TrimSpace(fileId)
	if !strings.HasPrefix(fileId, "copy:") {
		return telegramCopyMediaRef{}, false
	}
	raw := strings.TrimPrefix(fileId, "copy:")
	index := strings.LastIndex(raw, ":")
	if index <= 0 || index >= len(raw)-1 {
		return telegramCopyMediaRef{}, false
	}
	messageId, err := strconv.Atoi(raw[index+1:])
	if err != nil || messageId <= 0 {
		return telegramCopyMediaRef{}, false
	}
	chatId := normalizeTelegramChannelChatID(raw[:index])
	if strings.TrimSpace(chatId) == "" {
		return telegramCopyMediaRef{}, false
	}
	return telegramCopyMediaRef{ChatId: chatId, MessageId: messageId}, true
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
			attachName := fmt.Sprintf("media_%d_%s", media.Id, telegramUploadFilename(media, localPath))
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

func telegramUploadFilename(media *telegramMediaItem, localPath string) string {
	name := filepath.Base(localPath)
	if media != nil && media.MediaType == "video" && strings.EqualFold(filepath.Ext(name), ".m4v") {
		return strings.TrimSuffix(name, filepath.Ext(name)) + ".mp4"
	}
	return name
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

func localTelegramFileURLPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "data:") {
		return ""
	}
	path := raw
	if parsed, err := url.Parse(raw); err == nil {
		if parsed.Scheme != "" && parsed.Scheme != "http" && parsed.Scheme != "https" {
			return ""
		}
		if parsed.Path != "" {
			path = parsed.Path
		}
	}
	path = strings.TrimLeft(path, "/")
	if path == "" {
		return ""
	}
	for _, prefix := range []string{"resource/public/", "public/"} {
		if strings.HasPrefix(path, prefix) {
			return path
		}
	}
	if strings.HasPrefix(path, "attachment/") || strings.HasPrefix(path, "upload/") || strings.HasPrefix(path, "uploads/") {
		return path
	}
	return ""
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
