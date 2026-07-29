package sys

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

const telegramMediaGroupMaxItems = 10

func (s *sSysPublish) sendTelegramMediaSet(ctx context.Context, bot *tgbot.Bot, chatId string, purpose string, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	if len(media) == 0 {
		return nil, nil
	}
	s.prepareTelegramMediaItemsForSend(ctx, media)
	if len(media) == 1 {
		return s.sendTelegramSingleMedia(ctx, bot, chatId, purpose, caption, media[0])
	}
	if telegramMediaSetHasCopyRef(media) {
		if strings.TrimSpace(caption) == "" {
			return s.copyTelegramMediaSet(ctx, bot, chatId, purpose, caption, media)
		}
		media = telegramMediaSetWithoutTgFileId(media)
	}
	allMessages := make([]*telegramSentMessage, 0, len(media))
	for chunkIndex, chunk := range splitTelegramMediaItems(media, telegramMediaGroupMaxItems) {
		chunkCaption := ""
		if chunkIndex == 0 {
			chunkCaption = caption
		}
		group, mediaIds, assetHashes, closers, err := s.telegramInputMediaGroup(ctx, chunk, chunkCaption)
		if err != nil {
			return allMessages, err
		}
		if len(group) == 0 {
			continue
		}
		msgs, err := bot.SendMediaGroup(ctx, &tgbot.SendMediaGroupParams{
			ChatID: chatId,
			Media:  group,
		})
		closeTelegramMediaFiles(closers)
		if err != nil {
			return allMessages, err
		}
		allMessages = append(allMessages, telegramSentMessagesFromGroup(msgs, purpose, mediaIds, assetHashes)...)
	}
	return allMessages, nil
}

func (s *sSysPublish) sendTelegramSingleMedia(ctx context.Context, bot *tgbot.Bot, chatId string, purpose string, caption string, media *telegramMediaItem) ([]*telegramSentMessage, error) {
	s.prepareTelegramMediaItemForSend(ctx, media)
	if ref, ok := telegramCopyMediaRefFromFileId(media.TgFileId); ok {
		return s.copyTelegramSingleMedia(ctx, bot, chatId, purpose, caption, media, ref)
	}
	input, closer, err := telegramInputFile(ctx, media)
	if err != nil {
		return nil, err
	}
	if closer != nil {
		defer closer.Close()
	}
	switch media.MediaType {
	case "video":
		thumbnail, thumbnailCloser, err := telegramVideoThumbnail(ctx, media)
		if err != nil {
			return nil, err
		}
		if thumbnailCloser != nil {
			defer thumbnailCloser.Close()
		}
		videoMeta := s.telegramVideoMeta(ctx, media)
		params := &tgbot.SendVideoParams{
			ChatID:            chatId,
			Video:             input,
			Thumbnail:         thumbnail,
			Caption:           caption,
			ParseMode:         telegramMediaParseMode(caption),
			SupportsStreaming: true,
		}
		applyTelegramSendVideoMeta(params, videoMeta)
		msg, err := bot.SendVideo(ctx, params)
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
		return copied, err
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
	allMessages := make([]*telegramSentMessage, 0, len(media))
	for _, chunk := range splitTelegramMediaItems(media, telegramMediaGroupMaxItems) {
		fromChatId, messageIds, ok := telegramCopyMediaGroupRefs(chunk)
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
		messages := telegramSentMessagesFromCopiedIDs(copied, purpose, chunk)
		if len(messages) != len(chunk) {
			s.cleanupTelegramSentMessages(ctx, bot, chatId, append(allMessages, messages...), "复制媒体组返回数量不完整")
			return nil, true, gerror.Newf("复制媒体组返回数量不完整，期望:%d 实际:%d", len(chunk), len(messages))
		}
		allMessages = append(allMessages, messages...)
	}
	return allMessages, true, nil
}

func splitTelegramMediaItems(media []*telegramMediaItem, maxItems int) [][]*telegramMediaItem {
	if maxItems <= 0 {
		maxItems = telegramMediaGroupMaxItems
	}
	chunks := make([][]*telegramMediaItem, 0, (len(media)+maxItems-1)/maxItems)
	for start := 0; start < len(media); start += maxItems {
		end := start + maxItems
		if end > len(media) {
			end = len(media)
		}
		chunks = append(chunks, media[start:end])
	}
	return chunks
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

func (s *sSysPublish) telegramInputMediaGroup(ctx context.Context, media []*telegramMediaItem, caption string) ([]models.InputMedia, []int64, []string, []io.Closer, error) {
	group := make([]models.InputMedia, 0, len(media))
	mediaIds := make([]int64, 0, len(media))
	assetHashes := make([]string, 0, len(media))
	closers := make([]io.Closer, 0, len(media))
	for _, item := range media {
		source, attachment, closer, err := telegramInputMediaSource(ctx, item)
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
			thumbnail, thumbnailCloser, err := telegramVideoThumbnail(ctx, item)
			if err != nil {
				closeTelegramMediaFiles(closers)
				return nil, nil, nil, nil, err
			}
			if thumbnailCloser != nil {
				closers = append(closers, thumbnailCloser)
			}
			video := &models.InputMediaVideo{Media: source, Thumbnail: thumbnail, Caption: itemCaption, ParseMode: telegramMediaParseMode(itemCaption), SupportsStreaming: true, MediaAttachment: attachment}
			applyTelegramInputMediaVideoMeta(video, s.telegramVideoMeta(ctx, item))
			group = append(group, video)
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

func telegramInputFile(ctx context.Context, media *telegramMediaItem) (models.InputFile, io.Closer, error) {
	if media == nil {
		return nil, nil, gerror.New("媒体文件为空")
	}
	if source := strings.TrimSpace(media.TgFileId); source != "" && !telegramMediaRequiresSanitizedUpload(media) {
		return &models.InputFileString{Data: source}, nil, nil
	}
	path, cleanup, err := cachedTelegramMediaFile(ctx, media)
	if err != nil {
		return nil, nil, err
	}
	path, cleanup, err = prepareTelegramMediaUploadFile(ctx, media, path, cleanup)
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(path) == "" {
		return nil, nil, gerror.New("媒体文件地址为空")
	}
	file, err := os.Open(path)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, nil, gerror.Wrapf(err, "打开媒体文件失败:%s", path)
	}
	return &models.InputFileUpload{Filename: telegramUploadFilename(media, path), Data: file}, closeWithCleanup(file, cleanup), nil
}

func telegramMediaSetHasCopyRef(media []*telegramMediaItem) bool {
	for _, item := range media {
		if item == nil {
			continue
		}
		if telegramMediaRequiresSanitizedUpload(item) {
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
		if telegramMediaRequiresSanitizedUpload(item) {
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

func telegramInputMediaSource(ctx context.Context, media *telegramMediaItem) (string, io.Reader, io.Closer, error) {
	if media == nil {
		return "", nil, nil, gerror.New("媒体文件为空")
	}
	if source := strings.TrimSpace(media.TgFileId); source != "" && !telegramMediaRequiresSanitizedUpload(media) {
		return source, nil, nil, nil
	}
	path, cleanup, err := cachedTelegramMediaFile(ctx, media)
	if err != nil {
		return "", nil, nil, err
	}
	path, cleanup, err = prepareTelegramMediaUploadFile(ctx, media, path, cleanup)
	if err != nil {
		return "", nil, nil, err
	}
	if strings.TrimSpace(path) == "" {
		return "", nil, nil, gerror.New("媒体文件地址为空")
	}
	file, err := os.Open(path)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return "", nil, nil, gerror.Wrapf(err, "打开媒体文件失败:%s", path)
	}
	attachName := fmt.Sprintf("media_%d_%s", media.Id, telegramUploadFilename(media, path))
	return "attach://" + attachName, file, closeWithCleanup(file, cleanup), nil
}

func telegramMediaRequiresSanitizedUpload(media *telegramMediaItem) bool {
	if media == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(media.MediaType)) {
	case "image", "photo", "video":
		return true
	default:
		return false
	}
}

type fileCleanupCloser struct {
	*os.File
	cleanup func()
}

func closeWithCleanup(file *os.File, cleanup func()) io.Closer {
	if cleanup == nil {
		return file
	}
	return &fileCleanupCloser{File: file, cleanup: cleanup}
}

func (c *fileCleanupCloser) Close() error {
	if c == nil || c.File == nil {
		if c != nil && c.cleanup != nil {
			c.cleanup()
		}
		return nil
	}
	err := c.File.Close()
	if c.cleanup != nil {
		c.cleanup()
	}
	return err
}

type telegramRemoteMediaFile struct {
	*os.File
}

func (f *telegramRemoteMediaFile) Close() error {
	if f == nil || f.File == nil {
		return nil
	}
	name := f.Name()
	err := f.File.Close()
	_ = os.Remove(name)
	return err
}

func downloadTelegramRemoteMedia(source string) (*telegramRemoteMediaFile, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, gerror.New("远程媒体地址为空")
	}
	req, err := http.NewRequest(http.MethodGet, source, nil)
	if err != nil {
		return nil, gerror.Wrap(err, "创建远程媒体下载请求失败")
	}
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(req)
	if err != nil {
		return nil, gerror.Wrap(err, "下载远程媒体失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, gerror.Newf("下载远程媒体失败：HTTP %d", resp.StatusCode)
	}
	ext := filepath.Ext(strings.Split(source, "?")[0])
	if ext == "" {
		ext = ".media"
	}
	file, err := os.CreateTemp("", "ybp-tg-media-*"+ext)
	if err != nil {
		return nil, gerror.Wrap(err, "创建远程媒体临时文件失败")
	}
	if _, err = io.Copy(file, resp.Body); err != nil {
		name := file.Name()
		_ = file.Close()
		_ = os.Remove(name)
		return nil, gerror.Wrap(err, "保存远程媒体临时文件失败")
	}
	if _, err = file.Seek(0, 0); err != nil {
		name := file.Name()
		_ = file.Close()
		_ = os.Remove(name)
		return nil, gerror.Wrap(err, "读取远程媒体临时文件失败")
	}
	return &telegramRemoteMediaFile{File: file}, nil
}

func telegramVideoThumbnail(ctx context.Context, media *telegramMediaItem) (models.InputFile, io.Closer, error) {
	if media == nil || media.MediaType != "video" {
		return nil, nil, nil
	}
	videoPath, cleanup, err := cachedTelegramMediaFile(ctx, media)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(videoPath) == "" {
		return nil, nil, nil
	}
	thumbPath, err := generateTelegramVideoThumbnail(ctx, videoPath)
	if err != nil {
		g.Log().Warningf(ctx, "生成TG视频缩略图失败，跳过自定义缩略图 mediaId:%d path:%s err:%+v", media.Id, videoPath, err)
		return nil, nil, nil
	}
	file, err := os.Open(thumbPath)
	if err != nil {
		_ = os.Remove(thumbPath)
		return nil, nil, gerror.Wrap(err, "打开TG视频缩略图失败")
	}
	closer := closeWithCleanup(file, func() { _ = os.Remove(thumbPath) })
	return &models.InputFileUpload{Filename: filepath.Base(thumbPath), Data: file}, closer, nil
}

func generateTelegramVideoThumbnail(ctx context.Context, videoPath string) (string, error) {
	return generateVideoPosterPath(ctx, videoPath)
}

func fileNonEmpty(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
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
