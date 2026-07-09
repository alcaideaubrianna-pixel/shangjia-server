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
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const collectGroupedEventDelay = 3 * time.Second

func (s *sSysPublish) collectBotMessage(ctx context.Context, botId int64, msg *models.Message) {
	if msg == nil {
		return
	}
	if !s.collectGlobalEnabled(ctx) {
		return
	}
	sources, err := s.collectSourcesByBotMessage(ctx, botId, msg)
	if err != nil {
		g.Log().Warningf(ctx, "读取Bot采集源失败 bot:%d chat:%d err:%+v", botId, msg.Chat.ID, err)
		return
	}
	for _, source := range sources {
		eventId, grouped, err := s.saveCollectBotEvent(ctx, source, botId, msg)
		if err != nil {
			g.Log().Warningf(ctx, "保存Bot采集事件失败 source:%d msg:%d err:%+v", gconv.Int64(source["id"]), msg.ID, err)
			continue
		}
		if grouped {
			s.scheduleCollectGroupedEvent(eventId, gconv.Int64(source["tenant_id"]), gconv.Int64(source["account_id"]))
			continue
		}
		if err = s.processCollectEvent(ctx, eventId, gconv.Int64(source["tenant_id"]), gconv.Int64(source["account_id"])); err != nil {
			g.Log().Warningf(ctx, "处理Bot采集事件失败 event:%d err:%+v", eventId, err)
		}
	}
}

func (s *sSysPublish) collectSourcesByBotMessage(ctx context.Context, botId int64, msg *models.Message) ([]g.Map, error) {
	chatId := strconv.FormatInt(msg.Chat.ID, 10)
	mod := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where("source_type", sysin.CollectSourceTypeBot).
		Where("collect_enabled", 1).
		Where("status", 1).
		Where("source_chat_id", chatId).
		WhereNull("deleted_at")
	if botId > 0 {
		mod = mod.Where("bot_id", botId)
	}
	records, err := mod.All()
	if err != nil {
		return nil, err
	}
	rows := make([]g.Map, 0, len(records))
	for _, record := range records {
		rows = append(rows, record.Map())
	}
	return rows, nil
}

func (s *sSysPublish) saveCollectBotEvent(ctx context.Context, source g.Map, botId int64, msg *models.Message) (int64, bool, error) {
	chatId := strconv.FormatInt(msg.Chat.ID, 10)
	grouped := strings.TrimSpace(msg.MediaGroupID) != ""
	uniqueKey := fmt.Sprintf("bot:%d:%s:%d", botId, chatId, msg.ID)
	if grouped {
		uniqueKey = fmt.Sprintf("bot:%d:%s:group:%s", botId, chatId, msg.MediaGroupID)
	}
	record, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("source_unique_key", uniqueKey).One()
	if err != nil {
		return 0, grouped, err
	}
	rawText := strings.TrimSpace(telegramMessageText(msg))
	mediaItems := collectTelegramMediaItems(msg)
	mediaJson, mediaCount := collectMessageMediaJSON(mediaItems)
	now := gtime.Now()
	if !record.IsEmpty() {
		nextText := strings.TrimSpace(record["raw_text"].String())
		if nextText == "" {
			nextText = rawText
		}
		nextMediaJson, nextMediaCount := mergeCollectMediaJSON(record["media_json"].String(), mediaJson)
		nextSourceMessageId := record["source_message_id"].Int64()
		if int64(msg.ID) > 0 && (nextSourceMessageId <= 0 || int64(msg.ID) < nextSourceMessageId) {
			nextSourceMessageId = int64(msg.ID)
		}
		status := sysin.CollectEventStatusPending
		if grouped {
			status = sysin.CollectEventStatusGroupCollect
		}
		_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", record["id"].Int64()).Data(g.Map{
			"raw_text":          nextText,
			"media_count":       nextMediaCount,
			"media_json":        nextMediaJson,
			"source_message_id": nextSourceMessageId,
			"text_hash":         collectHash(nextText),
			"dedupe_key":        collectHash(fmt.Sprintf("%s:%s:%d", nextText, nextMediaJson, nextMediaCount)),
			"status":            status,
			"updated_at":        now,
		}).Update()
		if err != nil {
			return record["id"].Int64(), grouped, err
		}
		updated, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", record["id"].Int64()).One()
		if err != nil {
			return record["id"].Int64(), grouped, err
		}
		if err = s.upsertCollectEventMedia(ctx, updated, mediaItems); err != nil {
			return record["id"].Int64(), grouped, err
		}
		s.appendCollectEventLogForRecord(ctx, updated, "ingest", "updated", "Bot采集事件已合并媒体", "")
		return record["id"].Int64(), grouped, nil
	}
	receivedAt := now
	if msg.Date > 0 {
		receivedAt = gtime.NewFromTime(time.Unix(int64(msg.Date), 0))
	}
	status := sysin.CollectEventStatusPending
	if grouped {
		status = sysin.CollectEventStatusGroupCollect
	}
	eventId, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Data(g.Map{
		"tenant_id":         source["tenant_id"],
		"account_id":        source["account_id"],
		"source_id":         source["id"],
		"source_type":       sysin.CollectSourceTypeBot,
		"bot_id":            botId,
		"source_chat_id":    chatId,
		"source_message_id": msg.ID,
		"source_grouped_id": msg.MediaGroupID,
		"source_unique_key": uniqueKey,
		"raw_text":          rawText,
		"media_count":       mediaCount,
		"media_json":        mediaJson,
		"text_hash":         collectHash(rawText),
		"dedupe_key":        collectHash(fmt.Sprintf("%s:%s:%d", rawText, mediaJson, mediaCount)),
		"status":            status,
		"received_at":       receivedAt,
		"created_at":        now,
		"updated_at":        now,
	}).InsertAndGetId()
	if err != nil {
		return 0, grouped, err
	}
	created, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", eventId).One()
	if err != nil {
		return 0, grouped, err
	}
	if err = s.upsertCollectEventMedia(ctx, created, mediaItems); err != nil {
		return 0, grouped, err
	}
	s.appendCollectEventLogForRecord(ctx, created, "ingest", "created", "Bot采集事件已入库", "")
	_, _ = pdao.YoubanPublishCollectSource.Ctx(ctx).Where("id", source["id"]).Data(g.Map{
		"event_total":   gdb.Raw("event_total+1"),
		"last_event_at": now,
		"updated_at":    now,
	}).Update()
	return eventId, grouped, nil
}

type collectMediaItem struct {
	Type        string `json:"type"`
	FileId      string `json:"fileId"`
	FileUrl     string `json:"fileUrl,omitempty"`
	StoragePath string `json:"storagePath,omitempty"`
	PosterUrl   string `json:"posterUrl,omitempty"`
	MetaJson    string `json:"metaJson,omitempty"`
}

func collectTelegramMedia(msg *models.Message) (int, string) {
	items := collectTelegramMediaItems(msg)
	data, _ := json.Marshal(items)
	return len(items), string(data)
}

func collectTelegramMediaItems(msg *models.Message) []collectMediaItem {
	items := make([]collectMediaItem, 0, 2)
	if len(msg.Photo) > 0 {
		photo := msg.Photo[len(msg.Photo)-1]
		items = append(items, collectMediaItem{Type: "photo", FileId: photo.FileID})
	}
	if msg.Video != nil {
		items = append(items, collectMediaItem{Type: "video", FileId: msg.Video.FileID})
	}
	if msg.Document != nil {
		items = append(items, collectMediaItem{Type: "document", FileId: msg.Document.FileID})
	}
	return items
}

func mergeCollectMediaJSON(existing string, next string) (string, int) {
	items := make([]collectMediaItem, 0)
	_ = json.Unmarshal([]byte(existing), &items)
	var nextItems []collectMediaItem
	_ = json.Unmarshal([]byte(next), &nextItems)
	seen := map[string]struct{}{}
	merged := make([]collectMediaItem, 0, len(items)+len(nextItems))
	for _, item := range append(items, nextItems...) {
		sourceKey := collectMediaSourceKey(item)
		if sourceKey == "" {
			continue
		}
		key := item.Type + ":" + sourceKey
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, item)
	}
	data, _ := json.Marshal(merged)
	return string(data), len(merged)
}

func (s *sSysPublish) scheduleCollectGroupedEvent(eventId int64, tenantId int64, accountId int64) {
	if err := s.enqueueCollectProcess(context.Background(), collectProcessQueuePayload{
		EventId:   eventId,
		TenantId:  tenantId,
		AccountId: accountId,
	}, collectGroupedEventDelay); err != nil {
		g.Log().Warningf(context.Background(), "投递采集媒体组延迟处理失败 event:%d err:%+v", eventId, err)
	}
}

func (s *sSysPublish) stopCollectGroupedEventTimers() {
}
