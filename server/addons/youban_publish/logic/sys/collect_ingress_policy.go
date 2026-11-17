package sys

import (
	"context"
	"strconv"

	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/telegram_collector/domain"
	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	"hotgo/addons/youban_publish/library/ingresspolicy"
	"hotgo/internal/library/cache"
)

func (h *publishCollectorDeliveryHandler) ClassifyCollectorBotIngress(ctx context.Context, scope collectorin.BotIngressScope) collectorin.BotIngressDecision {
	return classifyCollectorBotIngress(ctx, scope)
}

func classifyCollectorBotIngress(ctx context.Context, scope collectorin.BotIngressScope) collectorin.BotIngressDecision {
	reason, err := ingresspolicy.Lookup(ctx, cache.Instance(), collectSourceCacheVersionKey, scope)
	if err != nil {
		g.Log().Warning(ctx, "采集入口策略缓存异常，保持实时处理", g.Map{"event": "collector.ingress.policy_fallback", "tenantId": scope.TenantID, "sourceId": scope.SourceID, "error": err.Error()})
		return collectorin.BotIngressDecision{Lane: collectorin.IngressLaneRealtime}
	}
	if reason.Valid() {
		return collectorin.BotIngressDecision{Lane: collectorin.IngressLaneBackground, Reason: reason}
	}
	return collectorin.BotIngressDecision{Lane: collectorin.IngressLaneRealtime}
}

func cachedCollectorDeliveryDecision(ctx context.Context, delivery *collectorin.CollectorDelivery) collectorin.BotIngressDecision {
	chat, err := strconv.ParseInt(delivery.SourceChatID, 10, 64)
	if err != nil {
		return collectorin.BotIngressDecision{Lane: collectorin.IngressLaneRealtime}
	}
	return classifyCollectorBotIngress(ctx, collectorin.BotIngressScope{TenantID: domain.TenantID(delivery.TenantID), SourceID: domain.SourceID(delivery.SourceID), ChatID: domain.TelegramChatID(chat)})
}

func rememberCollectorIngressDeferral(ctx context.Context, delivery *collectorin.CollectorDelivery, version string, reason collectorin.IngressDeferralReason) {
	chat, err := strconv.ParseInt(delivery.SourceChatID, 10, 64)
	if err != nil {
		return
	}
	scope := collectorin.BotIngressScope{TenantID: domain.TenantID(delivery.TenantID), SourceID: domain.SourceID(delivery.SourceID), ChatID: domain.TelegramChatID(chat)}
	if err := ingresspolicy.Remember(ctx, cache.Instance(), scope, version, reason); err != nil {
		g.Log().Warning(ctx, "记录采集入口后台路由失败", g.Map{"event": "collector.ingress.policy_cache_failed", "sourceId": delivery.SourceID, "error": err.Error()})
	}
}

func collectIngressPolicyVersion(ctx context.Context) string {
	v, err := ingresspolicy.EnsureVersion(ctx, cache.Instance(), collectSourceCacheVersionKey)
	if err != nil {
		return ""
	}
	return v
}
