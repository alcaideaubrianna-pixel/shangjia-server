package sys

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/tg"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (w *accountCollectWorker) bindGotdHandlers(dispatcher tg.UpdateDispatcher) {
	dispatcher.OnNewChannelMessage(func(ctx context.Context, entities tg.Entities, update *tg.UpdateNewChannelMessage) error {
		msg, ok := update.Message.(*tg.Message)
		if !ok {
			return nil
		}
		w.handleGotdMessage(ctx, entities, msg)
		return nil
	})
	dispatcher.OnNewMessage(func(ctx context.Context, entities tg.Entities, update *tg.UpdateNewMessage) error {
		msg, ok := update.Message.(*tg.Message)
		if !ok {
			return nil
		}
		w.handleGotdMessage(ctx, entities, msg)
		return nil
	})
}

func (w *accountCollectWorker) handleGotdMessage(ctx context.Context, entities tg.Entities, msg *tg.Message) {
	if w == nil || w.service == nil || msg == nil {
		return
	}
	if msg.Out {
		return
	}
	chatIds := gotdMessageChatIds(msg)
	if len(chatIds) == 0 {
		return
	}
	if gotdMessageGroupedId(msg) != "" {
		w.bufferListenerGroupedMessage(ctx, entities, msg, chatIds)
	} else {
		w.handleListenerMessage(ctx, entities, msg, chatIds)
	}
	sources, _ := w.configSnapshot()
	sources = matchAccountCollectSources(sources, chatIds)
	if len(sources) == 0 {
		return
	}
	for _, source := range sources {
		task := accountCollectMessageTask{source: source, msg: msg, chatId: chatIds[0]}
		select {
		case w.messages <- task:
		default:
			g.Log().Warningf(ctx, "账号采集消息队列已满，跳过消息 tgAccountId:%d source:%d msg:%d", w.tgAccountId, source.Id, msg.ID)
		}
	}
}

func (w *accountCollectWorker) ingestGotdMessage(ctx context.Context, source accountCollectSourceRuntime, msg *tg.Message, chatId string) {
	message := gotdCollectMessage(w.tgAccountId, source, msg, chatId)
	_, err := w.service.ingestAndProcessCollectMessage(ctx, message)
	if err != nil {
		g.Log().Errorf(ctx, "处理账号采集事件失败 source:%d msg:%d err:%+v", source.Id, msg.ID, err)
		return
	}
}

func gotdCollectMessage(tgAccountId int64, source accountCollectSourceRuntime, msg *tg.Message, chatId string) *CollectMessage {
	groupedId := gotdMessageGroupedId(msg)
	uniqueKey := fmt.Sprintf("account:%d:source:%d:%s:%d", tgAccountId, source.Id, chatId, msg.ID)
	if groupedId != "" {
		uniqueKey = fmt.Sprintf("account:%d:source:%d:%s:group:%s", tgAccountId, source.Id, chatId, groupedId)
	}
	return &CollectMessage{
		TenantId:        source.TenantId,
		AccountId:       source.AccountId,
		SourceId:        source.Id,
		SourceType:      sysin.CollectSourceTypeAccount,
		TgAccountId:     tgAccountId,
		SourceChatId:    chatId,
		SourceMessageId: int64(msg.ID),
		SourceGroupedId: groupedId,
		SourceUniqueKey: uniqueKey,
		RawText:         strings.TrimSpace(msg.Message),
		Media:           gotdCollectMedia(msg, chatId),
		ReceivedAt:      gtime.NewFromTime(time.Unix(int64(msg.Date), 0)),
	}
}

func gotdMessageChatIds(msg *tg.Message) []string {
	if msg == nil || msg.PeerID == nil {
		return nil
	}
	ids := make([]string, 0, 3)
	switch peer := msg.PeerID.(type) {
	case *tg.PeerChannel:
		ids = append(ids, strconv.FormatInt(peer.ChannelID, 10), "-100"+strconv.FormatInt(peer.ChannelID, 10))
	case *tg.PeerChat:
		ids = append(ids, strconv.FormatInt(peer.ChatID, 10), "-"+strconv.FormatInt(peer.ChatID, 10))
	case *tg.PeerUser:
		ids = append(ids, strconv.FormatInt(peer.UserID, 10))
	}
	return uniqueStrings(ids)
}

func matchAccountCollectSources(sources []accountCollectSourceRuntime, chatIds []string) []accountCollectSourceRuntime {
	allowed := make(map[string]struct{}, len(chatIds))
	for _, chatId := range chatIds {
		if chatId = strings.TrimSpace(chatId); chatId != "" {
			allowed[chatId] = struct{}{}
		}
	}
	matches := make([]accountCollectSourceRuntime, 0)
	for _, source := range sources {
		if _, ok := allowed[strings.TrimSpace(source.SourceChatId)]; ok {
			matches = append(matches, source)
		}
	}
	return matches
}

func gotdMessageGroupedId(msg *tg.Message) string {
	if msg == nil {
		return ""
	}
	groupedId, ok := msg.GetGroupedID()
	if !ok || groupedId == 0 {
		return ""
	}
	return strconv.FormatInt(groupedId, 10)
}

func gotdCollectMedia(msg *tg.Message, chatId string) []collectMediaItem {
	if msg == nil {
		return nil
	}
	media, ok := msg.GetMedia()
	if !ok || media == nil {
		return nil
	}
	mediaType := "media"
	var meta *gotdCollectMediaMeta
	switch item := media.(type) {
	case *tg.MessageMediaPhoto:
		mediaType = "photo"
		if photo, ok := item.Photo.(*tg.Photo); ok {
			meta = &gotdCollectMediaMeta{
				Kind:          "photo",
				Id:            photo.ID,
				AccessHash:    photo.AccessHash,
				FileReference: photo.FileReference,
				ThumbSize:     gotdLargestPhotoSizeType(photo),
				DCID:          photo.DCID,
			}
		}
	case *tg.MessageMediaDocument:
		if item.Video || item.Round {
			mediaType = "video"
		} else {
			mediaType = "document"
		}
		if doc, ok := item.Document.(*tg.Document); ok {
			meta = &gotdCollectMediaMeta{
				Kind:          "document",
				Id:            doc.ID,
				AccessHash:    doc.AccessHash,
				FileReference: doc.FileReference,
				MimeType:      doc.MimeType,
				DCID:          doc.DCID,
				Size:          doc.Size,
			}
		}
	default:
		return nil
	}
	result := collectMediaItem{Type: mediaType, FileId: fmt.Sprintf("gotd:%s:%d", chatId, msg.ID)}
	if meta != nil {
		result.SourceKind = meta.Kind
		result.SourceMediaId = meta.Id
		result.SourceAccessHash = meta.AccessHash
		result.SourceFileReference = append([]byte(nil), meta.FileReference...)
		result.SourceThumbSize = meta.ThumbSize
		result.SourceMimeType = meta.MimeType
		result.SourceDCId = meta.DCID
		result.SourceSize = meta.Size
	}
	return []collectMediaItem{result}
}

func gotdLargestPhotoSizeType(photo *tg.Photo) string {
	if photo == nil || len(photo.Sizes) == 0 {
		return "y"
	}
	bestType := ""
	bestScore := -1
	for _, item := range photo.Sizes {
		sizeType := ""
		score := -1
		switch size := item.(type) {
		case *tg.PhotoSize:
			sizeType = strings.TrimSpace(size.Type)
			score = size.W * size.H
			if size.Size > score {
				score = size.Size
			}
		case *tg.PhotoSizeProgressive:
			sizeType = strings.TrimSpace(size.Type)
			score = size.W * size.H
			for _, value := range size.Sizes {
				if value > score {
					score = value
				}
			}
		case *tg.PhotoCachedSize:
			sizeType = strings.TrimSpace(size.Type)
			score = size.W * size.H
			if len(size.Bytes) > score {
				score = len(size.Bytes)
			}
		}
		if sizeType == "" || score < bestScore {
			continue
		}
		bestType = sizeType
		bestScore = score
	}
	if bestType == "" {
		return "y"
	}
	return bestType
}

type gotdCollectMediaMeta struct {
	Kind          string `json:"kind"`
	Id            int64  `json:"id"`
	AccessHash    int64  `json:"accessHash"`
	FileReference []byte `json:"fileReference"`
	ThumbSize     string `json:"thumbSize,omitempty"`
	MimeType      string `json:"mimeType,omitempty"`
	DCID          int    `json:"dcId,omitempty"`
	Size          int64  `json:"size,omitempty"`
}

func collectMessageHasGotdMedia(message *CollectMessage) bool {
	if message == nil {
		return false
	}
	for _, item := range message.Media {
		if strings.HasPrefix(strings.TrimSpace(item.FileId), "gotd:") {
			return true
		}
	}
	return false
}
