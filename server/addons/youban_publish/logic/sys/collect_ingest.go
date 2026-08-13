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
