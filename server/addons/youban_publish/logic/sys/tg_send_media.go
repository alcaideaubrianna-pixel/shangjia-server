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
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"golang.org/x/sync/errgroup"
)

const telegramMediaGroupMaxItems = 10
const telegramMediaPrepareConcurrency = 4

func (s *sSysPublish) sendTelegramMediaSet(ctx context.Context, bot *tgbot.Bot, chatId string, purpose string, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	if len(media) == 0 {
		return nil, nil
	}
	s.prepareTelegramMediaItemsForSend(ctx, media)
	if strings.TrimSpace(caption) != "" && telegramMediaSetHasCopyRef(media) {
		media = telegramMediaSetWithoutTgFileId(media)
	}
	if len(media) == 1 {
		return s.sendTelegramSingleMedia(ctx, bot, chatId, purpose, caption, media[0])
	}
	if telegramMediaSetHasCopyRef(media) {
		if strings.TrimSpace(caption) == "" {
			return s.copyTelegramMediaSet(ctx, bot, chatId, purpose, caption, media)
		}
		captionMessage, err := bot.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:    chatId,
			Text:      caption,
			ParseMode: models.ParseModeHTML,
		})
		if err != nil {
			return nil, err
		}
		copied, ok, err := s.copyTelegramMediaGroup(ctx, bot, chatId, purpose, "", media)
		if err != nil {
			if captionMessage != nil {
				s.cleanupTelegramSentMessages(ctx, bot, chatId, []*telegramSentMessage{{MessageId: int64(captionMessage.ID), Purpose: purpose}}, "复制媒体组失败")
			}
			return nil, err
		}
		if !ok {
			return nil, gerror.New("多媒体组复制引用不完整，且本地媒体缓存不可用")
		}
		messages := make([]*telegramSentMessage, 0, len(copied)+1)
		if captionMessage != nil {
			messages = append(messages, &telegramSentMessage{MessageId: int64(captionMessage.ID), Purpose: purpose})
		}
		return append(messages, copied...), nil
	}
	allMessages := make([]*telegramSentMessage, 0, len(media))
	chunks := splitTelegramMediaItems(media, telegramMediaGroupMaxItems)
	for chunkIndex, chunk := range chunks {
		chunkCaption := ""
		if chunkIndex == 0 || purpose == "verify" {
			chunkCaption = caption
		}
		prepareStartedAt := time.Now()
		group, closers, err := s.telegramInputMediaGroup(ctx, chunk, chunkCaption)
		if err != nil {
			return allMessages, err
		}
		g.Log().Infof(ctx, "TG媒体组准备完成 purpose:%s chunk:%d/%d media:%d antiScan:%d duration:%s", purpose, chunkIndex+1, len(chunks), len(chunk), telegramAntiScanMediaCount(chunk), time.Since(prepareStartedAt).Round(time.Millisecond))
		if len(group) == 0 {
			continue
		}
		sendStartedAt := time.Now()
		msgs, err := bot.SendMediaGroup(ctx, &tgbot.SendMediaGroupParams{
			ChatID: chatId,
			Media:  group,
		})
		closeTelegramMediaFiles(closers)
		if err != nil && telegramMediaSetHasReusableFileId(chunk) && (isTelegramInvalidReusableFileError(err) || isTelegramPhotoTooLargeError(err)) {
			group, closers, err = s.telegramInputMediaGroup(ctx, telegramMediaSetWithoutTgFileId(chunk), chunkCaption)
			if err == nil {
				msgs, err = bot.SendMediaGroup(ctx, &tgbot.SendMediaGroupParams{ChatID: chatId, Media: group})
				closeTelegramMediaFiles(closers)
			}
		}
		if err != nil && isTelegramRequestEntityTooLargeError(err) {
			fallback, fallbackErr := s.sendTelegramOversizedMediaChunk(ctx, bot, chatId, purpose, chunkCaption, chunk)
			if fallbackErr == nil {
				allMessages = append(allMessages, fallback...)
				continue
			}
			err = fallbackErr
		}
		if err != nil {
			return allMessages, err
		}
		g.Log().Infof(ctx, "TG媒体组发送完成 purpose:%s chunk:%d/%d media:%d duration:%s", purpose, chunkIndex+1, len(chunks), len(chunk), time.Since(sendStartedAt).Round(time.Millisecond))
		allMessages = append(allMessages, telegramSentMessagesFromGroup(msgs, purpose, chunk)...)
	}
	return allMessages, nil
}

func (s *sSysPublish) sendTelegramOversizedMediaChunk(ctx context.Context, bot *tgbot.Bot, chatId string, purpose string, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	if len(media) <= 1 {
		return s.sendTelegramSingleMedia(ctx, bot, chatId, purpose, caption, media[0])
	}
	middle := len(media) / 2
	first, err := s.sendTelegramMediaSet(ctx, bot, chatId, purpose, caption, media[:middle])
	if err != nil {
		return nil, err
	}
	second, err := s.sendTelegramMediaSet(ctx, bot, chatId, purpose, "", media[middle:])
	if err != nil {
		return first, err
	}
	return append(first, second...), nil
}

func (s *sSysPublish) sendTelegramSingleMedia(ctx context.Context, bot *tgbot.Bot, chatId string, purpose string, caption string, media *telegramMediaItem) ([]*telegramSentMessage, error) {
	s.prepareTelegramMediaItemForSend(ctx, media)
	if ref, ok := telegramCopyMediaRefFromFileId(media.TgFileId); ok {
		messages, err := s.copyTelegramSingleMedia(ctx, bot, chatId, purpose, caption, media, ref)
		if err != nil && isTelegramPhotoTooLargeError(err) {
			cloned := *media
			cloned.TgFileId = ""
			cloned.TgThumbFileId = ""
			return s.sendTelegramSingleMedia(ctx, bot, chatId, purpose, caption, &cloned)
		}
		return messages, err
	}
	prepareStartedAt := time.Now()
	input, closer, err := telegramSingleMediaInputFile(ctx, media)
	if err != nil {
		return nil, err
	}
	if closer != nil {
		defer closer.Close()
	}
	switch media.MediaType {
	case "video":
		reuseWithCover := telegramVideoUsesReusableFileIdWithCover(media)
		var thumbnail models.InputFile
		var cover models.InputFile
		var thumbnailCloser io.Closer
		if reuseWithCover {
			cover, thumbnailCloser, err = telegramVideoPreview(ctx, media, "cover")
		} else if !telegramMediaUsesReusableFileId(media) {
			thumbnail, thumbnailCloser, err = telegramVideoPreview(ctx, media, "thumbnail")
		}
		if err != nil {
			return nil, err
		}
		if thumbnailCloser != nil {
			defer thumbnailCloser.Close()
		}
		g.Log().Infof(ctx, "TG单媒体准备完成 purpose:%s mediaId:%d type:%s antiScan:%t duration:%s", purpose, media.Id, media.MediaType, media.AntiScanEnabled, time.Since(prepareStartedAt).Round(time.Millisecond))
		videoMeta := telegramVideoMeta{}
		if !reuseWithCover {
			videoMeta = s.telegramVideoMeta(ctx, media)
		}
		params := &tgbot.SendVideoParams{
			ChatID:            chatId,
			Video:             input,
			Thumbnail:         thumbnail,
			Cover:             cover,
			Caption:           caption,
			ParseMode:         telegramMediaParseMode(caption),
			SupportsStreaming: true,
		}
		applyTelegramSendVideoMeta(params, videoMeta)
		sendStartedAt := time.Now()
		msg, err := bot.SendVideo(ctx, params)
		if err != nil && strings.TrimSpace(media.TgFileId) != "" && (isTelegramInvalidReusableFileError(err) || isTelegramPhotoTooLargeError(err)) {
			cloned := *media
			cloned.TgFileId = ""
			cloned.TgThumbFileId = ""
			return s.sendTelegramSingleMedia(ctx, bot, chatId, purpose, caption, &cloned)
		}
		if err != nil {
			if isTelegramPhotoTooLargeError(err) {
				cloned := *media
				cloned.TgFileId = ""
				cloned.TgThumbFileId = ""
				return s.sendTelegramSingleMedia(ctx, bot, chatId, purpose, caption, &cloned)
			}
			return nil, err
		}
		g.Log().Infof(ctx, "TG单媒体发送完成 purpose:%s mediaId:%d type:%s duration:%s", purpose, media.Id, media.MediaType, time.Since(sendStartedAt).Round(time.Millisecond))
		return telegramSentMessagesFromSingle(msg, purpose, media)
	default:
		g.Log().Infof(ctx, "TG单媒体准备完成 purpose:%s mediaId:%d type:%s antiScan:%t duration:%s", purpose, media.Id, media.MediaType, media.AntiScanEnabled, time.Since(prepareStartedAt).Round(time.Millisecond))
		sendStartedAt := time.Now()
		msg, err := bot.SendPhoto(ctx, &tgbot.SendPhotoParams{
			ChatID:    chatId,
			Photo:     input,
			Caption:   caption,
			ParseMode: telegramMediaParseMode(caption),
		})
		if err != nil && strings.TrimSpace(media.TgFileId) != "" && isTelegramInvalidReusableFileError(err) {
			cloned := *media
			cloned.TgFileId = ""
			return s.sendTelegramSingleMedia(ctx, bot, chatId, purpose, caption, &cloned)
		}
		if err != nil {
			return nil, err
		}
		g.Log().Infof(ctx, "TG单媒体发送完成 purpose:%s mediaId:%d type:%s duration:%s", purpose, media.Id, media.MediaType, time.Since(sendStartedAt).Round(time.Millisecond))
		return telegramSentMessagesFromSingle(msg, purpose, media)
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
			MessageId:        int64(msg.ID),
			Purpose:          purpose,
			MediaId:          item.Id,
			TgFileId:         item.TgFileId,
			AssetHash:        item.AssetHash,
			ProtectedHashKey: item.ProtectedHashKey,
			ProtectedPHash:   item.ProtectedPHash,
			ProtectedDHash:   item.ProtectedDHash,
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

func (s *sSysPublish) telegramInputMediaGroup(ctx context.Context, media []*telegramMediaItem, caption string) ([]models.InputMedia, []io.Closer, error) {
	prepared := make([]telegramPreparedInputMedia, len(media))
	prepareGroup, prepareCtx := errgroup.WithContext(ctx)
	prepareGroup.SetLimit(telegramMediaPrepareConcurrency)
	for index, item := range media {
		index, item := index, item
		prepareGroup.Go(func() error {
			itemCaption := ""
			if index == 0 {
				itemCaption = caption
			}
			input, closers, err := s.telegramInputMediaItem(prepareCtx, item, itemCaption)
			if err != nil {
				return err
			}
			prepared[index] = telegramPreparedInputMedia{input: input, closers: closers}
			return nil
		})
	}
	if err := prepareGroup.Wait(); err != nil {
		closeTelegramPreparedInputMedia(prepared)
		return nil, nil, err
	}
	group := make([]models.InputMedia, 0, len(prepared))
	closers := make([]io.Closer, 0, len(prepared)*2)
	for _, item := range prepared {
		group = append(group, item.input)
		closers = append(closers, item.closers...)
	}
	return group, closers, nil
}

type telegramPreparedInputMedia struct {
	input   models.InputMedia
	closers []io.Closer
}

func (s *sSysPublish) telegramInputMediaItem(ctx context.Context, item *telegramMediaItem, caption string) (models.InputMedia, []io.Closer, error) {
	source, attachment, closer, err := telegramInputMediaSource(ctx, item)
	if err != nil {
		return nil, nil, err
	}
	closers := make([]io.Closer, 0, 2)
	if closer != nil {
		closers = append(closers, closer)
	}
	if source == "" {
		closeTelegramMediaFiles(closers)
		return nil, nil, gerror.New("媒体文件地址为空")
	}
	if item.MediaType != "video" {
		return &models.InputMediaPhoto{Media: source, Caption: caption, ParseMode: telegramMediaParseMode(caption), MediaAttachment: attachment}, closers, nil
	}
	var thumbnail models.InputFile
	if !telegramMediaUsesReusableFileId(item) {
		thumbnail, closer, err = telegramVideoThumbnail(ctx, item)
		if err != nil {
			closeTelegramMediaFiles(closers)
			return nil, nil, err
		}
		if closer != nil {
			closers = append(closers, closer)
		}
	}
	video := &models.InputMediaVideo{Media: source, Thumbnail: thumbnail, Caption: caption, ParseMode: telegramMediaParseMode(caption), SupportsStreaming: true, MediaAttachment: attachment}
	applyTelegramInputMediaVideoMeta(video, s.telegramVideoMeta(ctx, item))
	return video, closers, nil
}

func closeTelegramPreparedInputMedia(prepared []telegramPreparedInputMedia) {
	for _, item := range prepared {
		closeTelegramMediaFiles(item.closers)
	}
}

func telegramAntiScanMediaCount(media []*telegramMediaItem) int {
	count := 0
	for _, item := range media {
		if item != nil && item.AntiScanEnabled {
			count++
		}
	}
	return count
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

func telegramSingleMediaInputFile(ctx context.Context, media *telegramMediaItem) (models.InputFile, io.Closer, error) {
	if telegramVideoUsesReusableFileIdWithCover(media) {
		return &models.InputFileString{Data: strings.TrimSpace(media.TgFileId)}, nil, nil
	}
	return telegramInputFile(ctx, media)
}

func telegramVideoUsesReusableFileIdWithCover(media *telegramMediaItem) bool {
	if media == nil || media.MediaType != "video" || !media.AntiScanEnabled {
		return false
	}
	fileId := strings.TrimSpace(media.TgFileId)
	if fileId == "" {
		return false
	}
	_, copyRef := telegramCopyMediaRefFromFileId(fileId)
	return !copyRef
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

func telegramMediaSetHasReusableFileId(media []*telegramMediaItem) bool {
	for _, item := range media {
		if item != nil && strings.TrimSpace(item.TgFileId) != "" && !telegramMediaRequiresSanitizedUpload(item) {
			return true
		}
	}
	return false
}

func telegramMediaUsesReusableFileId(media *telegramMediaItem) bool {
	if media == nil || telegramMediaRequiresSanitizedUpload(media) {
		return false
	}
	fileId := strings.TrimSpace(media.TgFileId)
	if fileId == "" {
		return false
	}
	_, copyRef := telegramCopyMediaRefFromFileId(fileId)
	return !copyRef
}

func isTelegramInvalidReusableFileError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, part := range []string{"wrong file identifier", "file_id_invalid", "file reference expired", "file_reference_expired", "file_reference_invalid"} {
		if strings.Contains(message, part) {
			return true
		}
	}
	return false
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
	return media != nil && media.AntiScanEnabled
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

func telegramVideoThumbnail(ctx context.Context, media *telegramMediaItem) (models.InputFile, io.Closer, error) {
	return telegramVideoPreview(ctx, media, "thumbnail")
}

func telegramVideoPreview(ctx context.Context, media *telegramMediaItem, kind string) (models.InputFile, io.Closer, error) {
	if media == nil || media.MediaType != "video" {
		return nil, nil, nil
	}
	thumbPath, cleanupThumb, err := cachedTelegramVideoPosterFile(ctx, media)
	if err != nil {
		if cleanupThumb != nil {
			cleanupThumb()
		}
		return nil, nil, err
	}
	if strings.TrimSpace(thumbPath) == "" {
		return nil, nil, nil
	}
	if media.AntiScanEnabled {
		protectedPath, protectedCleanup, protectErr := prepareTelegramAntiScanUploadFile(ctx, media, thumbPath, cleanupThumb, kind)
		if protectErr != nil {
			return nil, nil, gerror.Wrap(protectErr, "处理TG视频缩略图防扫图失败")
		}
		thumbPath = protectedPath
		cleanupThumb = protectedCleanup
	}
	file, err := os.Open(thumbPath)
	if err != nil {
		if cleanupThumb != nil {
			cleanupThumb()
		}
		return nil, nil, gerror.Wrap(err, "打开TG视频缩略图失败")
	}
	closer := closeWithCleanup(file, cleanupThumb)
	return &models.InputFileUpload{Filename: filepath.Base(thumbPath), Data: file}, closer, nil
}

func cachedTelegramVideoPosterFile(ctx context.Context, media *telegramMediaItem) (string, func(), error) {
	if media == nil {
		return "", nil, nil
	}
	if path := strings.TrimSpace(media.PosterStoragePath); path != "" {
		localPath := resolveTelegramLocalPath(path)
		if fileExists(localPath) {
			return localPath, nil, nil
		}
	}
	if path := localTelegramFileURLPath(media.PosterUrl); path != "" {
		localPath := resolveTelegramLocalPath(path)
		if fileExists(localPath) {
			return localPath, nil, nil
		}
	}
	if source := strings.TrimSpace(media.PosterUrl); strings.HasPrefix(strings.ToLower(source), "http") && !isLocalTelegramURL(source) {
		path, err := cachedRemoteMediaFile(ctx, mediaFileCacheKey(media, source), source, mediaFileCacheExt(&telegramMediaItem{MediaType: "image", FileUrl: source}, source))
		return path, nil, err
	}
	videoPath, cleanup, err := cachedTelegramMediaFile(ctx, media)
	if err != nil || strings.TrimSpace(videoPath) == "" {
		return "", cleanup, err
	}
	thumbPath, err := generateTelegramVideoThumbnail(ctx, videoPath)
	if cleanup != nil {
		cleanup()
	}
	if err != nil {
		return "", nil, gerror.Wrapf(err, "生成TG视频预览图失败 mediaId:%d path:%s", media.Id, videoPath)
	}
	return thumbPath, func() { _ = os.Remove(thumbPath) }, nil
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

func telegramSentMessagesFromGroup(msgs []*models.Message, purpose string, media []*telegramMediaItem) []*telegramSentMessage {
	list := make([]*telegramSentMessage, 0, len(msgs))
	for i, msg := range msgs {
		if msg == nil {
			continue
		}
		var mediaItem *telegramMediaItem
		if i < len(media) {
			mediaItem = media[i]
		}
		if mediaItem == nil {
			continue
		}
		list = append(list, &telegramSentMessage{
			MessageId:        int64(msg.ID),
			MediaGroupId:     msg.MediaGroupID,
			Purpose:          purpose,
			MediaId:          mediaItem.Id,
			TgFileId:         telegramMessageFileId(msg),
			AssetHash:        mediaItem.AssetHash,
			ProtectedHashKey: mediaItem.ProtectedHashKey,
			ProtectedPHash:   mediaItem.ProtectedPHash,
			ProtectedDHash:   mediaItem.ProtectedDHash,
		})
	}
	return list
}

func telegramSentMessagesFromSingle(msg *models.Message, purpose string, media *telegramMediaItem) ([]*telegramSentMessage, error) {
	if msg == nil {
		return nil, nil
	}
	return []*telegramSentMessage{{
		MessageId:        int64(msg.ID),
		MediaGroupId:     msg.MediaGroupID,
		Purpose:          purpose,
		MediaId:          media.Id,
		TgFileId:         telegramMessageFileId(msg),
		AssetHash:        media.AssetHash,
		ProtectedHashKey: media.ProtectedHashKey,
		ProtectedPHash:   media.ProtectedPHash,
		ProtectedDHash:   media.ProtectedDHash,
	}}, nil
}
