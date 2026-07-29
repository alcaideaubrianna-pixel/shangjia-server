package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

const (
	collectGroupedEventDelay     = collectMaterialGroupingDelay
	collectSourceCacheVersionKey = "youban_publish:collect:sources:version"
	collectSourceCacheTTL        = 30 * time.Second
)

type collectBotSourceCacheItem struct {
	Id          int64  `json:"id"`
	TenantId    int64  `json:"tenantId"`
	AccountId   int64  `json:"accountId"`
	SourceTitle string `json:"sourceTitle"`
}

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
		message := botCollectMessage(botId, source, msg)
		_, err := s.ingestAndProcessCollectMessage(ctx, message)
		if err != nil {
			g.Log().Errorf(ctx, "处理Bot采集事件失败 source:%d msg:%d err:%+v", gconv.Int64(source["id"]), msg.ID, err)
			continue
		}
	}
}

func (s *sSysPublish) collectSourcesByBotMessage(ctx context.Context, botId int64, msg *models.Message) ([]g.Map, error) {
	chatId := strconv.FormatInt(msg.Chat.ID, 10)
	version := s.collectSourceCacheVersion(ctx)
	cacheKey := fmt.Sprintf("youban_publish:collect:sources:%s:%d:%s", version, botId, chatId)
	if value, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !value.IsNil() {
		var items []collectBotSourceCacheItem
		if json.Unmarshal([]byte(value.String()), &items) == nil {
			return collectBotSourceMaps(items), nil
		}
	}
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
	items := make([]collectBotSourceCacheItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, collectBotSourceCacheItem{Id: gconv.Int64(row["id"]), TenantId: gconv.Int64(row["tenant_id"]), AccountId: gconv.Int64(row["account_id"]), SourceTitle: gconv.String(row["title"])})
	}
	if data, marshalErr := json.Marshal(items); marshalErr == nil {
		_ = cache.Instance().Set(ctx, cacheKey, string(data), collectSourceCacheTTL)
	}
	return rows, nil
}

func collectBotSourceMaps(items []collectBotSourceCacheItem) []g.Map {
	rows := make([]g.Map, 0, len(items))
	for _, item := range items {
		rows = append(rows, g.Map{"id": item.Id, "tenant_id": item.TenantId, "account_id": item.AccountId, "title": item.SourceTitle})
	}
	return rows
}

func (s *sSysPublish) collectSourceCacheVersion(ctx context.Context) string {
	value, err := cache.Instance().Get(ctx, collectSourceCacheVersionKey)
	if err == nil && !value.IsNil() && value.String() != "" {
		return value.String()
	}
	version := strconv.FormatInt(time.Now().UnixNano(), 10)
	_ = cache.Instance().Set(ctx, collectSourceCacheVersionKey, version, 24*time.Hour)
	return version
}

func (s *sSysPublish) refreshCollectSourceCache(ctx context.Context) {
	_ = cache.Instance().Set(ctx, collectSourceCacheVersionKey, strconv.FormatInt(time.Now().UnixNano(), 10), 24*time.Hour)
}

func botCollectMessage(botId int64, source g.Map, msg *models.Message) *CollectMessage {
	chatId := strconv.FormatInt(msg.Chat.ID, 10)
	now := gtime.Now()
	receivedAt := now
	if msg.Date > 0 {
		receivedAt = gtime.NewFromTime(time.Unix(int64(msg.Date), 0))
	}
	uniqueKey := "bot:" + strconv.FormatInt(botId, 10) + ":source:" + strconv.FormatInt(gconv.Int64(source["id"]), 10) + ":" + chatId + ":" + strconv.Itoa(msg.ID)
	if groupedId := strings.TrimSpace(msg.MediaGroupID); groupedId != "" {
		uniqueKey = "bot:" + strconv.FormatInt(botId, 10) + ":source:" + strconv.FormatInt(gconv.Int64(source["id"]), 10) + ":" + chatId + ":group:" + groupedId
	}
	return &CollectMessage{
		TenantId:        gconv.Int64(source["tenant_id"]),
		AccountId:       gconv.Int64(source["account_id"]),
		SourceId:        gconv.Int64(source["id"]),
		SourceType:      sysin.CollectSourceTypeBot,
		BotId:           botId,
		SourceChatId:    chatId,
		SourceMessageId: int64(msg.ID),
		SourceGroupedId: strings.TrimSpace(msg.MediaGroupID),
		SourceUniqueKey: uniqueKey,
		RawText:         telegramMessageText(msg),
		Media:           collectTelegramMediaItems(msg),
		ReceivedAt:      receivedAt,
	}
}

type collectMediaItem struct {
	Type        string `json:"type"`
	Purpose     string `json:"purpose,omitempty"`
	FileId      string `json:"fileId"`
	FileUrl     string `json:"fileUrl,omitempty"`
	StoragePath string `json:"storagePath,omitempty"`
	PosterUrl   string `json:"posterUrl,omitempty"`
	FileMd5     string `json:"fileMd5,omitempty"`
	FilePhash   string `json:"filePhash,omitempty"`
	MetaJson    string `json:"metaJson,omitempty"`
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
		key := strings.TrimSpace(item.Purpose) + ":" + item.Type + ":" + sourceKey
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, item)
	}
	data, _ := json.Marshal(merged)
	return string(data), len(merged)
}

func collectMediaJSONWithPurpose(mediaJSON string, purpose string) string {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return mediaJSON
	}
	var items []collectMediaItem
	if err := json.Unmarshal([]byte(mediaJSON), &items); err != nil {
		return mediaJSON
	}
	filtered := make([]collectMediaItem, 0, len(items))
	for index := range items {
		itemPurpose := strings.TrimSpace(items[index].Purpose)
		if itemPurpose != "" && !strings.EqualFold(itemPurpose, purpose) {
			continue
		}
		if itemPurpose == "" {
			items[index].Purpose = purpose
		}
		filtered = append(filtered, items[index])
	}
	data, err := json.Marshal(filtered)
	if err != nil {
		return mediaJSON
	}
	return string(data)
}

func (s *sSysPublish) scheduleCollectGroupedEvent(eventId int64, sourceId int64, tenantId int64, accountId int64) {
	if err := s.enqueueCollectProcess(context.Background(), collectProcessQueuePayload{
		EventId:   eventId,
		SourceId:  sourceId,
		TenantId:  tenantId,
		AccountId: accountId,
	}, collectGroupedEventDelay); err != nil {
		g.Log().Warningf(context.Background(), "投递采集媒体组延迟处理失败 event:%d err:%+v", eventId, err)
	}
}

func (s *sSysPublish) stopCollectGroupedEventTimers() {
}
