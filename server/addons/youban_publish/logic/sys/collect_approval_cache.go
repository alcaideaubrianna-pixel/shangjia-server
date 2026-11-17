package sys

import (
	"context"
	"fmt"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	publishconsts "hotgo/addons/youban_publish/consts"
	"hotgo/internal/library/cache"
)

const collectApprovalRejectedTTL = time.Minute

type collectApprovalCacheOutcome string

const (
	collectApprovalCacheHit         collectApprovalCacheOutcome = "hit_rejected"
	collectApprovalCacheMiss        collectApprovalCacheOutcome = "miss"
	collectApprovalCacheReadFailed  collectApprovalCacheOutcome = "read_failed_db_fallback"
	collectApprovalCacheWriteFailed collectApprovalCacheOutcome = "write_failed"
	collectApprovalCacheStored      collectApprovalCacheOutcome = "stored_rejected"
)

// Capture the generation before the database read. A concurrent configuration
// change therefore cannot repopulate a rejection in the new generation.
func (s *sSysPublish) collectApprovalRejectedKey(ctx context.Context, tenantID, accountID, sourceID, botID int64, chatID string) string {
	version, err := cache.Instance().Get(ctx, collectSourceCacheVersionKey)
	if err != nil {
		g.Log().Warning(ctx, "读取采集审批缓存版本失败，回查数据库", g.Map{"tenantId": tenantID, "sourceId": sourceID, "error": err.Error()})
		return ""
	}
	generation := ""
	if version == nil || version.IsNil() {
		// Do not overwrite a generation published concurrently by configuration edits.
		candidate := fmt.Sprint(time.Now().UnixNano())
		if _, err := cache.Instance().SetIfNotExist(ctx, collectSourceCacheVersionKey, candidate, 24*time.Hour); err != nil {
			return ""
		}
		current, err := cache.Instance().Get(ctx, collectSourceCacheVersionKey)
		if err != nil || current == nil || current.IsNil() {
			return ""
		}
		generation = current.String()
	} else {
		generation = version.String()
	}
	return fmt.Sprintf("%s%d:%d:%d:%d:%s:%s", publishconsts.CollectApprovalRejectedKeyPrefix, tenantID, accountID, sourceID, botID, canonicalCollectSourceChatID(chatID), generation)
}

func logCollectApprovalCache(ctx context.Context, delivery *collectorin.CollectorDelivery, accountID int64, outcome collectApprovalCacheOutcome) {
	g.Log().Info(ctx, "采集审批拒绝缓存", g.Map{
		"log_type": "telegram_collect_pipeline", "event": "collect_approval_cache",
		"stage": "approval.cache", "cacheOutcome": outcome,
		"tenantId": delivery.TenantID, "accountId": accountID, "sourceId": delivery.SourceID,
		"sourceChatId": delivery.SourceChatID, "sourceMessageId": delivery.SourceMessageID,
		"eventId": delivery.EventID, "deliveryId": delivery.ID,
	})
}
