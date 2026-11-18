package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

type collectPublishTargetCheckKey struct{}

// A receipt is local to this processing call, never a cross-request allow cache.
// Rule branches inherit it so fanout does not repeat the same SQL per rule.
type collectPublishTargetCheck struct {
	tenantID int64
	chatID   string
}

func (s *sSysPublish) checkCollectEventPublishTarget(ctx context.Context, event gdb.Record) (context.Context, bool, error) {
	tenantID := event["tenant_id"].Int64()
	chatID := canonicalCollectSourceChatID(event["source_chat_id"].String())
	if tenantID <= 0 || chatID == "" {
		return ctx, false, gerror.New("采集事件缺少实际来源频道，不能检查发布频道回流")
	}
	if checked, ok := ctx.Value(collectPublishTargetCheckKey{}).(collectPublishTargetCheck); ok &&
		checked.tenantID == tenantID && checked.chatID == chatID {
		return ctx, false, nil
	}
	blocked, err := s.collectMessageFromAccountPublishChannel(ctx, tenantID, 0, event["source_chat_id"].String())
	if err != nil {
		return ctx, false, gerror.Wrap(err, "检查采集事件发布频道回流失败")
	}
	if blocked {
		g.Log().Info(ctx, "采集事件来自发布频道，禁止生成资料和分发", g.Map{
			"event": "collect_self_loop_blocked", "reason": "self_loop_publish_target",
			"tenantId": tenantID, "accountId": event["account_id"].Int64(),
			"sourceId": event["source_id"].Int64(), "collectEventId": event["id"].Int64(),
			"sourceChatId": event["source_chat_id"].String(), "sourceMessageId": event["source_message_id"].Int64(),
		})
		return ctx, true, nil
	}
	return context.WithValue(ctx, collectPublishTargetCheckKey{}, collectPublishTargetCheck{tenantID, chatID}), false, nil
}
