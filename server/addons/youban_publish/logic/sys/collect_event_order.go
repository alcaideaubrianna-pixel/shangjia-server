package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const collectOrderRetryDelay = 3 * time.Second

func (s *sSysPublish) waitCollectEventSourceOrder(ctx context.Context, event gdb.Record) (bool, error) {
	if event.IsEmpty() || event["media_count"].Int() <= 0 || event["source_message_id"].Int64() <= 0 || strings.TrimSpace(event["source_chat_id"].String()) == "" {
		return false, nil
	}
	previous, err := s.previousActiveCollectEvent(ctx, event)
	if err != nil {
		return false, err
	}
	if previous.IsEmpty() {
		return false, nil
	}
	message := fmt.Sprintf("等待前序消息处理完成 sourceMessageId=%d event=%d", previous["source_message_id"].Int64(), previous["id"].Int64())
	_, err = pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", event["id"].Int64()).Data(g.Map{
		"status":        sysin.CollectEventStatusWaitingOrder,
		"error_message": message,
		"updated_at":    gtime.Now(),
	}).Update()
	if err != nil {
		return false, gerror.Wrap(err, "标记采集事件等待顺序失败")
	}
	s.appendCollectEventLogForRecord(ctx, event, "order", "waiting", message, fmt.Sprintf("previous=%d", previous["id"].Int64()))
	if err = s.enqueueCollectProcess(ctx, collectProcessQueuePayload{
		EventId:   event["id"].Int64(),
		TenantId:  event["tenant_id"].Int64(),
		AccountId: event["account_id"].Int64(),
	}, collectOrderRetryDelay); err != nil {
		return false, gerror.Wrap(err, "投递采集顺序等待重试失败")
	}
	return true, nil
}

func (s *sSysPublish) previousActiveCollectEvent(ctx context.Context, event gdb.Record) (gdb.Record, error) {
	row, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Fields("id,source_message_id,status,media_count").
		Where("tenant_id", event["tenant_id"].Int64()).
		Where("account_id", event["account_id"].Int64()).
		Where("source_id", event["source_id"].Int64()).
		Where("source_chat_id", event["source_chat_id"].String()).
		Where("source_message_id <", event["source_message_id"].Int64()).
		WhereGT("media_count", 0).
		WhereIn("status", collectEventOrderBlockingStatuses()).
		OrderDesc("source_message_id").
		OrderDesc("id").
		Limit(1).
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取前序采集事件失败")
	}
	return row, nil
}

func collectEventOrderBlockingStatuses() []string {
	return []string{
		sysin.CollectEventStatusPending,
		sysin.CollectEventStatusWaitingOrder,
		sysin.CollectEventStatusPrechecked,
		sysin.CollectEventStatusMediaPending,
		sysin.CollectEventStatusMediaReady,
	}
}
