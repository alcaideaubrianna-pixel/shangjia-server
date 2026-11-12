package sys

import (
	"context"
	"database/sql"
	"errors"
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

// collectMessageFromAccountPublishChannel 拦截所有已配置为上架目标的频道消息，
// 防止上架频道的消息再次进入采集链路形成死循环。
func (s *sSysPublish) collectMessageFromAccountPublishChannel(ctx context.Context, tenantId, tgAccountId int64, chatId string, _ ...int64) (bool, error) {
	chatId = normalizeTelegramChannelChatID(chatId)
	if tenantId <= 0 || chatId == "" {
		return false, nil
	}
	// Do this lookup synchronously at the ingestion boundary. A long-lived
	// cached target list can outlive a channel reconfiguration and let messages
	// from a publish target re-enter collection, creating an A -> A loop.
	// Telegram represents channel IDs both as N and -100N, so compare both
	// canonical forms without stripping arbitrary "100" substrings.
	var target struct {
		Id int64 `json:"id"`
	}
	model := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id").
		Where("tenant_id", tenantId).
		Where("publish_direction", "up").
		Where("status", 1).
		// Keep soft-deleted publish targets in the loop guard.  Telegram can
		// still deliver messages from a channel after its web configuration is
		// removed, and allowing that historical target back into collection
		// creates the same A -> A loop we are trying to prevent.
		Where("(target_chat_id=? OR target_chat_id=? OR REPLACE(target_chat_id,'-100','')=?)", chatId, "-100"+chatId, chatId)
	// 上架目标是租户级频道边界，不能按采集协议账号过滤；同一目标可能由
	// Bot、多个协议账号或多个账户共用，任何入口都必须拦截，避免自采集循环。
	err := model.Scan(&target)
	// 没有匹配的上架目标是正常路径，表示该来源频道可以继续采集。
	// GoFrame 在 Scan 到空结果时可能返回 sql.ErrNoRows，不能把它当成采集失败。
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return target.Id > 0, nil
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
