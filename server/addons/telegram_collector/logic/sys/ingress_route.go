package sys

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/telegram_collector/domain"
	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

func classifyBotIngress(ctx context.Context, tenantID, sourceID, chatID int64) sysin.BotIngressDecision {
	router, ok := collectorservice.CollectorDeliveryHandler().(collectorservice.BotIngressRouter)
	if !ok {
		return sysin.BotIngressDecision{Lane: sysin.IngressLaneRealtime}
	}
	return router.ClassifyCollectorBotIngress(ctx, sysin.BotIngressScope{
		TenantID: domain.TenantID(tenantID), SourceID: domain.SourceID(sourceID), ChatID: domain.TelegramChatID(chatID),
	})
}

func logBotIngressDeferral(ctx context.Context, tenantID, sourceID, eventID, messageID int64, decision sysin.BotIngressDecision) {
	g.Log().Info(ctx, "已知拒绝采集任务转入后台预算", g.Map{"event": "collector.ingress.deferred", "tenantId": tenantID, "sourceId": sourceID, "eventId": eventID, "sourceMessageId": messageID, "reason": decision.Reason})
}
