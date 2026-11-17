package sys

import (
	"context"
	"strconv"
	"strings"
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

// collectMessageFromAccountPublishChannel 拦截所有已配置为上架目标的频道消息，
// 防止上架频道的消息再次进入采集链路形成死循环。
func (s *sSysPublish) collectMessageFromAccountPublishChannel(ctx context.Context, tenantId, tgAccountId int64, chatId string, _ ...int64) (bool, error) {
	chatIDs := collectPublishChatLookupIDs(chatId)
	if tenantId <= 0 || len(chatIDs) == 0 {
		return false, nil
	}
	// Do this lookup synchronously at the ingestion boundary. A long-lived
	// cached target list can outlive a channel reconfiguration and let messages
	// from a publish target re-enter collection, creating an A -> A loop.
	// Telegram represents channel IDs both as N and -100N, so compare both
	// canonical forms without stripping arbitrary "100" substrings.
	model := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id").
		Where("tenant_id", tenantId).
		Where("publish_direction", "up").
		Where("status", 1).
		// Keep soft-deleted publish targets in the loop guard.  Telegram can
		// still deliver messages from a channel after its web configuration is
		// removed, and allowing that historical target back into collection
		// creates the same A -> A loop we are trying to prevent.
		WhereIn("target_chat_id", chatIDs)
	// 上架目标是租户级频道边界，不能按采集协议账号过滤；同一目标可能由
	// Bot、多个协议账号或多个账户共用，任何入口都必须拦截，避免自采集循环。
	target, err := model.Limit(1).One()
	if err != nil {
		return false, err
	}
	return !target.IsEmpty(), nil
}

func collectPublishChatLookupIDs(chatID string) []string {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return nil
	}
	if strings.HasPrefix(chatID, "@") || (strings.HasPrefix(chatID, "-") && !strings.HasPrefix(chatID, "-100")) {
		return []string{chatID}
	}
	short := canonicalCollectSourceChatID(chatID)
	if short == "" {
		return nil
	}
	return []string{short, "-100" + short}
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
	if err := cache.Instance().Set(ctx, collectSourceCacheVersionKey, strconv.FormatInt(time.Now().UnixNano(), 10), 24*time.Hour); err != nil {
		g.Log().Warning(ctx, "采集配置缓存失效失败，审批拒绝缓存最多保留一分钟", g.Map{"event": "collect_approval_cache_invalidation", "outcome": "failed", "error": err.Error()})
		return
	}
	g.Log().Info(ctx, "采集配置变更已使审批拒绝缓存失效", g.Map{"event": "collect_approval_cache_invalidation", "outcome": "invalidated"})
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
