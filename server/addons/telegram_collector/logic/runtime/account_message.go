package runtime

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gotd/td/tg"

	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

type accountMessageTask struct {
	source  sysin.AccountRuntimeSource
	message *tg.Message
	chatID  string
}

func (w *accountWorker) bindAccountMessageHandlers(dispatcher tg.UpdateDispatcher) {
	dispatcher.OnNewChannelMessage(func(ctx context.Context, entities tg.Entities, update *tg.UpdateNewChannelMessage) error {
		message, ok := update.Message.(*tg.Message)
		if ok {
			w.handleAccountMessage(ctx, entities, message)
		}
		return nil
	})
	dispatcher.OnNewMessage(func(ctx context.Context, entities tg.Entities, update *tg.UpdateNewMessage) error {
		message, ok := update.Message.(*tg.Message)
		if ok {
			w.handleAccountMessage(ctx, entities, message)
		}
		return nil
	})
}

func (w *accountWorker) handleAccountMessage(ctx context.Context, entities tg.Entities, message *tg.Message) {
	if w == nil || message == nil || message.Out {
		return
	}
	chatIDs := accountMessageChatIDs(message)
	if len(chatIDs) == 0 {
		return
	}
	if observer, ok := w.session.(collectorservice.AccountRuntimeMessageObserver); ok {
		observer.HandleAccountRuntimeMessage(ctx, entities, message, chatIDs)
	}
	binding := w.bindingSnapshot()
	if binding == nil {
		return
	}
	for _, source := range matchAccountRuntimeSources(binding.Sources, chatIDs) {
		task := accountMessageTask{source: source, message: message, chatID: chatIDs[0]}
		select {
		case w.messages <- task:
		default:
			g.Log().Warningf(ctx, "Telegram账号采集事件缓冲区已满 accountId:%d sourceId:%d messageId:%d", binding.AccountID, source.SourceID, message.ID)
		}
	}
}

func (w *accountWorker) runAccountMessageLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-w.messages:
			event := accountMessageEvent(w.bindingSnapshot(), task)
			if event == nil {
				continue
			}
			if err := collectorservice.Collector().IngestAccountMessage(ctx, event); err != nil {
				g.Log().Errorf(ctx, "提交Telegram账号采集事件失败 accountId:%d sourceId:%d messageId:%d err:%+v", event.TgAccountID, event.SourceID, event.SourceMessageID, err)
			}
		}
	}
}

func accountMessageEvent(binding *sysin.AccountRuntimeBinding, task accountMessageTask) *sysin.AccountMessageEvent {
	if binding == nil || task.message == nil || task.source.SourceID <= 0 {
		return nil
	}
	return BuildAccountMessageEvent(binding.AccountID, task.source, task.message, task.chatID)
}

// BuildAccountMessageEvent is the single conversion boundary shared by
// realtime listeners and history-page collection.
func BuildAccountMessageEvent(tgAccountID int64, source sysin.AccountRuntimeSource, message *tg.Message, chatID string) *sysin.AccountMessageEvent {
	if tgAccountID <= 0 || source.SourceID <= 0 || message == nil || message.ID <= 0 {
		return nil
	}
	groupedID := accountMessageGroupedID(message)
	return &sysin.AccountMessageEvent{
		TenantID: source.TenantID, AccountID: source.AccountID, SourceID: source.SourceID,
		TgAccountID: tgAccountID, SourceChatID: chatID, SourceMessageID: int64(message.ID),
		SourceGroupedID: groupedID,
		SourceUniqueKey: fmt.Sprintf("account:%d:source:%d:%s:message:%d", tgAccountID, source.SourceID, chatID, message.ID),
		RawText:         strings.TrimSpace(message.Message), Media: accountMessageMedia(message, chatID),
		ReceivedAt: time.Unix(int64(message.Date), 0),
	}
}

func accountMessageChatIDs(message *tg.Message) []string {
	if message == nil || message.PeerID == nil {
		return nil
	}
	ids := make([]string, 0, 2)
	switch peer := message.PeerID.(type) {
	case *tg.PeerChannel:
		ids = append(ids, strconv.FormatInt(peer.ChannelID, 10), "-100"+strconv.FormatInt(peer.ChannelID, 10))
	case *tg.PeerChat:
		ids = append(ids, strconv.FormatInt(peer.ChatID, 10), "-"+strconv.FormatInt(peer.ChatID, 10))
	case *tg.PeerUser:
		ids = append(ids, strconv.FormatInt(peer.UserID, 10))
	}
	return uniqueAccountMessageStrings(ids)
}

func matchAccountRuntimeSources(sources []sysin.AccountRuntimeSource, chatIDs []string) []sysin.AccountRuntimeSource {
	allowed := make(map[string]struct{}, len(chatIDs))
	for _, chatID := range chatIDs {
		if chatID = strings.TrimSpace(chatID); chatID != "" {
			allowed[chatID] = struct{}{}
		}
	}
	result := make([]sysin.AccountRuntimeSource, 0)
	for _, source := range sources {
		if _, ok := allowed[strings.TrimSpace(source.ChatID)]; ok {
			result = append(result, source)
		}
	}
	return result
}

func accountMessageGroupedID(message *tg.Message) string {
	if message == nil {
		return ""
	}
	groupedID, ok := message.GetGroupedID()
	if !ok || groupedID == 0 {
		return ""
	}
	return strconv.FormatInt(groupedID, 10)
}

func accountMessageMedia(message *tg.Message, chatID string) []sysin.CollectorMediaItem {
	if message == nil {
		return nil
	}
	media, ok := message.GetMedia()
	if !ok || media == nil {
		return nil
	}
	item := sysin.CollectorMediaItem{Type: "media", FileID: fmt.Sprintf("gotd:%s:%d", chatID, message.ID)}
	switch value := media.(type) {
	case *tg.MessageMediaPhoto:
		item.Type = sysin.MediaKindPhoto
		photo, photoOK := value.Photo.(*tg.Photo)
		if !photoOK {
			return []sysin.CollectorMediaItem{item}
		}
		item.SourceKind = sysin.MediaKindPhoto
		item.SourceMediaID = photo.ID
		item.SourceAccessHash = photo.AccessHash
		item.SourceFileReference = append([]byte(nil), photo.FileReference...)
		item.SourceThumbSize = accountLargestPhotoSizeType(photo)
		item.SourceDCID = photo.DCID
	case *tg.MessageMediaDocument:
		item.Type = sysin.MediaKindFile
		if value.Video || value.Round {
			item.Type = sysin.MediaKindVideo
		}
		document, documentOK := value.Document.(*tg.Document)
		if !documentOK {
			return []sysin.CollectorMediaItem{item}
		}
		item.SourceKind = "document"
		item.SourceMediaID = document.ID
		item.SourceAccessHash = document.AccessHash
		item.SourceFileReference = append([]byte(nil), document.FileReference...)
		item.SourceMimeType = document.MimeType
		item.SourceDCID = document.DCID
		item.SourceSize = document.Size
	default:
		return nil
	}
	return []sysin.CollectorMediaItem{item}
}

func accountLargestPhotoSizeType(photo *tg.Photo) string {
	if photo == nil || len(photo.Sizes) == 0 {
		return "y"
	}
	bestType := ""
	bestScore := -1
	for _, item := range photo.Sizes {
		sizeType, score := accountPhotoSizeScore(item)
		if sizeType == "" || score < bestScore {
			continue
		}
		bestType, bestScore = sizeType, score
	}
	if bestType == "" {
		return "y"
	}
	return bestType
}

func accountPhotoSizeScore(item tg.PhotoSizeClass) (string, int) {
	switch size := item.(type) {
	case *tg.PhotoSize:
		return strings.TrimSpace(size.Type), maxInt(size.W*size.H, size.Size)
	case *tg.PhotoSizeProgressive:
		score := size.W * size.H
		for _, value := range size.Sizes {
			score = maxInt(score, value)
		}
		return strings.TrimSpace(size.Type), score
	case *tg.PhotoCachedSize:
		return strings.TrimSpace(size.Type), maxInt(size.W*size.H, len(size.Bytes))
	default:
		return "", -1
	}
}

func uniqueAccountMessageStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}
