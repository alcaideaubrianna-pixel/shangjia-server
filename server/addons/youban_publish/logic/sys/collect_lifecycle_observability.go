package sys

import (
	"context"
	"time"

	"hotgo/internal/library/collectobserve"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/gtrace"
)

// One owned, successful creation commit is one lifecycle observation. Reuse,
// rollback and an unconfirmed outer transaction must not become slow profiles.
// Business identifiers stay in logs rather than creating metric series.
func collectProfileLifecycleFields(ctx context.Context, profileID int64, created, topLevel bool, commitErr error, finishedAt time.Time) g.Map {
	if commitErr != nil || profileID <= 0 || !created || !topLevel {
		return nil
	}
	meta := collectobserve.From(ctx)
	fields := g.Map{
		"log_type": "telegram_collect_pipeline", "event": "collect_profile_committed",
		"result": "persisted", "action": "created",
		"profileId": profileID, "tenantId": meta.TenantID, "accountId": meta.AccountID,
		"sourceId": meta.SourceID, "sourceChatId": meta.ChatID,
		"canonicalChatId": canonicalCollectSourceChatID(meta.ChatID),
		"botId":           meta.BotID, "tgAccountId": meta.TgAccountID,
		"collectEventId": meta.EventID, "upstreamEventId": meta.UpstreamEventID,
		"deliveryId": meta.DeliveryID, "ruleId": meta.RuleID,
		"sourceType":     collectobserve.SourceType(meta.SourceType),
		"collectionMode": collectobserve.Mode(meta.Mode), "traceId": gtrace.GetTraceID(ctx),
		"committedAtUnixMs": finishedAt.UnixMilli(), "lifecycleSample": "valid",
		"timeBasis": collectobserve.TimeBasis(meta.TimeBasis), "slowThresholdSeconds": 600,
	}
	switch {
	case meta.ReceivedAt.IsZero():
		fields["lifecycleSample"] = "missing_received_at"
	case meta.ReceivedAt.After(finishedAt):
		fields["lifecycleSample"] = "future_received_at"
	default:
		elapsed := finishedAt.Sub(meta.ReceivedAt)
		fields["receivedAtUnixMs"] = meta.ReceivedAt.UnixMilli()
		fields["lifecycleSeconds"] = elapsed.Seconds()
		fields["lifecycleMs"] = elapsed.Milliseconds()
		fields["slow"] = elapsed >= 10*time.Minute
	}
	return fields
}

func logCollectProfileLifecycle(ctx context.Context, profileID int64, created, topLevel bool, commitErr error, finishedAt time.Time) {
	fields := collectProfileLifecycleFields(ctx, profileID, created, topLevel, commitErr, finishedAt)
	if fields == nil {
		return
	}
	g.Log().Info(ctx, "采集资料事务提交完成", fields)
}
