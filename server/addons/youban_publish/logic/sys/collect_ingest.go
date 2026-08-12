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
	collectGroupedEventDelay      = collectMaterialGroupingDelay
	collectSourceCacheVersionKey  = "youban_publish:collect:sources:version"
	collectSourceCacheTTL         = 30 * time.Second
	collectSourceMissLogTTL       = time.Minute
	collectPublishChannelCacheKey = "youban_publish:collect:publish_channels"
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
		s.logCollectBotSkip(ctx, botId, msg, "采集总开关已关闭")
		return
	}
	sources, err := s.collectSourcesByBotMessage(ctx, botId, msg)
	if err != nil {
		g.Log().Warningf(ctx, "读取Bot采集源失败 bot:%d chat:%d err:%+v", botId, msg.Chat.ID, err)
		return
	}
	if len(sources) == 0 {
		s.logCollectBotSkip(ctx, botId, msg, "没有匹配到启用中的Bot采集源，请检查bot_id、采集开关、状态和会员有效期")
		return
	}
	for _, source := range sources {
		blocked, checkErr := s.botCollectMessageFromPublishChannel(ctx, gconv.Int64(source["tenant_id"]), msg.Chat.ID)
		if checkErr != nil {
			g.Log().Warningf(ctx, "检查Bot采集上架频道过滤失败，已安全丢弃消息 source:%d chat:%d message:%d err:%+v", gconv.Int64(source["id"]), msg.Chat.ID, msg.ID, checkErr)
			continue
		}
		if blocked {
			g.Log().Infof(ctx, "Bot采集忽略当前账号上架频道消息 source:%d chat:%d message:%d", gconv.Int64(source["id"]), msg.Chat.ID, msg.ID)
			continue
		}
		message := botCollectMessage(botId, source, msg)
		_, err := s.ingestAndProcessCollectMessage(ctx, message)
		if err != nil {
			g.Log().Errorf(ctx, "处理Bot采集事件失败 source:%d msg:%d err:%+v", gconv.Int64(source["id"]), msg.ID, err)
			continue
		}
	}
}

func (s *sSysPublish) botCollectMessageFromPublishChannel(ctx context.Context, tenantId int64, chatId int64) (bool, error) {
	if tenantId <= 0 || chatId == 0 {
		return false, nil
	}
	version := s.collectSourceCacheVersion(ctx)
	cacheKey := fmt.Sprintf("%s:%s:%d", collectPublishChannelCacheKey, version, tenantId)
	var chatIds []string
	if value, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !value.IsNil() {
		if json.Unmarshal([]byte(value.String()), &chatIds) == nil {
			return botCollectChatMatchesPublishChannel(chatId, chatIds), nil
		}
	}
	records, err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("target_chat_id").
		Where("tenant_id", tenantId).
		Where("publish_direction", "up").
		WhereNull("deleted_at").
		All()
	if err != nil {
		return false, err
	}
	chatIds = make([]string, 0, len(records))
	for _, record := range records {
		targetChatId := normalizeTelegramChannelChatID(record["target_chat_id"].String())
		if targetChatId != "" {
			chatIds = append(chatIds, targetChatId)
		}
	}
	if data, marshalErr := json.Marshal(chatIds); marshalErr == nil {
		_ = cache.Instance().Set(ctx, cacheKey, string(data), collectSourceCacheTTL)
	}
	return botCollectChatMatchesPublishChannel(chatId, chatIds), nil
}

func botCollectChatMatchesPublishChannel(chatId int64, publishChatIds []string) bool {
	incoming := normalizeTelegramChannelChatID(strconv.FormatInt(chatId, 10))
	for _, publishChatId := range publishChatIds {
		if incoming == normalizeTelegramChannelChatID(publishChatId) {
			return true
		}
	}
	return false
}

func (s *sSysPublish) logCollectBotSkip(ctx context.Context, botId int64, msg *models.Message, reason string) {
	if msg == nil {
		return
	}
	key := fmt.Sprintf("youban_publish:collect:source-miss:%d", botId)
	if value, err := cache.Instance().Get(ctx, key); err == nil && !value.IsNil() {
		return
	}
	_ = cache.Instance().Set(ctx, key, 1, collectSourceMissLogTTL)
	g.Log().Warningf(ctx, "Bot采集跳过消息 bot:%d chat:%d message:%d reason:%s", botId, msg.Chat.ID, msg.ID, reason)
}

func (s *sSysPublish) collectSourcesByBotMessage(ctx context.Context, botId int64, msg *models.Message) ([]g.Map, error) {
	if err := ensureTenantVipTables(ctx); err != nil {
		return nil, err
	}
	version := s.collectSourceCacheVersion(ctx)
	cacheKey := fmt.Sprintf("youban_publish:collect:sources:%s:%d", version, botId)
	if value, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !value.IsNil() {
		var items []collectBotSourceCacheItem
		if json.Unmarshal([]byte(value.String()), &items) == nil {
			return collectBotSourceMaps(items), nil
		}
	}
	mod := pdao.YoubanPublishCollectSource.DB().Model(pdao.YoubanPublishCollectSource.Table()+" s").Safe().Ctx(ctx).
		InnerJoin(pdao.YoubanPublishTenantVip.Table()+" vip", "vip.tenant_id=s.tenant_id AND vip.status=1 AND vip.level>0 AND vip.deleted_at IS NULL").
		Where("s.source_type", sysin.CollectSourceTypeBot).
		Where("s.collect_enabled", 1).
		Where("s.status", 1).
		Where("(vip.expired_at IS NULL OR vip.expired_at>?)", gtime.Now()).
		WhereNull("s.deleted_at").
		Fields("s.*")
	if botId > 0 {
		mod = mod.Where("s.bot_id", botId)
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
	EventMediaId        int64  `json:"-"`
	Type                string `json:"type"`
	Purpose             string `json:"purpose,omitempty"`
	FileId              string `json:"fileId"`
	FileUrl             string `json:"fileUrl,omitempty"`
	StoragePath         string `json:"storagePath,omitempty"`
	PosterUrl           string `json:"posterUrl,omitempty"`
	FileMd5             string `json:"fileMd5,omitempty"`
	FilePhash           string `json:"filePhash,omitempty"`
	SourceKind          string `json:"sourceKind,omitempty"`
	SourceMediaId       int64  `json:"sourceMediaId,omitempty"`
	SourceAccessHash    int64  `json:"sourceAccessHash,omitempty"`
	SourceFileReference []byte `json:"sourceFileReference,omitempty"`
	SourceThumbSize     string `json:"sourceThumbSize,omitempty"`
	SourceMimeType      string `json:"sourceMimeType,omitempty"`
	SourceDCId          int    `json:"sourceDcId,omitempty"`
	SourceSize          int64  `json:"sourceSize,omitempty"`
	DebugMetaJson       string `json:"debugMetaJson,omitempty"`
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
