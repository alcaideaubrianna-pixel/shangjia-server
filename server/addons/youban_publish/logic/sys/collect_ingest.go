package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/library/cache"
)

const (
	collectGroupedEventDelay      = collectMaterialGroupingDelay
	collectSourceCacheVersionKey  = "youban_publish:collect:sources:version"
	collectSourceCacheTTL         = 30 * time.Second
	collectPublishChannelCacheKey = "youban_publish:collect:publish_channels"
)

// collectMessageFromAccountPublishChannel 仅拦截已确认由本系统发送的消息，
// 允许同一频道中的其他原始消息继续进入代理采集规则。
func (s *sSysPublish) collectMessageFromAccountPublishChannel(ctx context.Context, tenantId, tgAccountId int64, chatId string, messageID ...int64) (bool, error) {
	chatId = normalizeTelegramChannelChatID(chatId)
	if tenantId <= 0 || chatId == "" {
		return false, nil
	}
	if len(messageID) > 0 && messageID[0] > 0 {
		count, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
			Where("target_chat_id", chatId).Where("tg_message_id", messageID[0]).Count()
		if err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
		return false, nil
	}
	version := s.collectSourceCacheVersion(ctx)
	cacheKey := fmt.Sprintf("%s:%s:%d:%d", collectPublishChannelCacheKey, version, tenantId, tgAccountId)
	var chatIds []string
	if value, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !value.IsNil() {
		if json.Unmarshal([]byte(value.String()), &chatIds) == nil {
			return collectChatMatchesPublishChannel(chatId, chatIds), nil
		}
	}
	model := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("target_chat_id").
		Where("tenant_id", tenantId).
		Where("publish_direction", "up").
		Where("status", 1).
		WhereNull("deleted_at")
	if tgAccountId > 0 {
		model = model.Where("tg_account_id", tgAccountId)
	}
	records, err := model.All()
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
	return collectChatMatchesPublishChannel(chatId, chatIds), nil
}

func collectChatMatchesPublishChannel(chatId string, publishChatIds []string) bool {
	incoming := normalizeTelegramChannelChatID(chatId)
	for _, publishChatId := range publishChatIds {
		if incoming == normalizeTelegramChannelChatID(publishChatId) {
			return true
		}
	}
	return false
}

// Kept as a small compatibility helper for callers that already hold Telegram's numeric chat ID.
func botCollectChatMatchesPublishChannel(chatId int64, publishChatIds []string) bool {
	return collectChatMatchesPublishChannel(strconv.FormatInt(chatId, 10), publishChatIds)
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

type collectMediaItem struct {
	EventMediaId        int64  `json:"-"`
	Type                string `json:"type"`
	Purpose             string `json:"purpose,omitempty"`
	FileId              string `json:"fileId"`
	FileUniqueId        string `json:"fileUniqueId,omitempty"`
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

func (s *sSysPublish) scheduleCollectGroupedEvent(ctx context.Context, eventId int64, sourceId int64, sourceChatId string, tenantId int64, accountId int64, history bool) error {
	return s.enqueueCollectProcess(ctx, collectProcessQueuePayload{
		History: history, EventId: eventId, SourceId: sourceId, SourceChatId: sourceChatId, TenantId: tenantId, AccountId: accountId,
	}, collectGroupedEventDelay)
}

func (s *sSysPublish) stopCollectGroupedEventTimers() {}
