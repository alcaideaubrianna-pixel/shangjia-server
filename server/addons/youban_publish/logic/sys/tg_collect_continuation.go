package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

func (s *sSysPublish) telegramChannelHasPendingCollectContinuation(ctx context.Context, current telegramJobRecord) (bool, error) {
	if current.Id <= 0 {
		return false, nil
	}
	if current.CollectSourceId <= 0 || current.CollectSourceMessageId <= 0 || current.CollectSourceChatId == "" {
		return false, nil
	}
	candidate, err := s.telegramChannelPendingCollectContinuation(ctx, current)
	if err != nil || candidate.IsEmpty() {
		return false, err
	}
	if candidate["id"].Int64() == current.Id {
		return false, nil
	}
	return true, nil
}

func (s *sSysPublish) telegramChannelPendingCollectContinuation(ctx context.Context, current telegramJobRecord) (gdb.Record, error) {
	now := gtime.Now()
	mod := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		LeftJoin(publishTaskTable+" t", "t.id=j.task_id").
		LeftJoin(pdaoCollectEventTable()+" e", "e.id=t.collect_event_id").
		Fields("j.id,j.task_id,j.collect_source_id,j.collect_source_chat_id,j.collect_source_message_id").
		Where("j.id <> 0").
		WhereIn("j.status", []string{"pending", "failed_retry"}).
		Where("(j.dispatch_status = ? OR j.dispatch_status = '')", tgDispatchStatusIdle).
		Where("(j.next_retry_at IS NULL OR j.next_retry_at <= ?)", now).
		Where("j.collect_source_id > 0").
		Where("j.collect_source_message_id > 0").
		Where("j.collect_source_id", current.CollectSourceId).
		Where("j.collect_source_chat_id", current.CollectSourceChatId).
		Where("e.id > 0").
		Where("e.media_count = 1").
		Where("e.source_grouped_id = ''").
		Where("EXISTS (" + collectContinuationPreviousGroupSQL() + ")").
		Where("EXISTS (" + collectContinuationVideoMediaSQL() + ")")
	if current.ChannelId > 0 {
		mod = mod.Where("j.channel_id", current.ChannelId)
	} else {
		mod = mod.Where("j.target_chat_id", normalizeTelegramChannelChatID(current.TargetChatId))
	}
	row, err := mod.OrderAsc("j.created_at").OrderAsc("j.id").Limit(1).One()
	if err != nil {
		return nil, gerror.Wrap(err, "检查采集绑定后续消息失败")
	}
	return row, nil
}

func collectContinuationPreviousGroupSQL() string {
	return `
SELECT 1
FROM ` + pdaoCollectEventTable() + ` pe
JOIN ` + publishTgJobTable + ` pj ON pj.collect_event_id = pe.id
WHERE pe.tenant_id = e.tenant_id
  AND pe.account_id = e.account_id
  AND pe.source_id = e.source_id
  AND pe.source_chat_id = e.source_chat_id
  AND pe.source_message_id < e.source_message_id
  AND pe.media_count > 1
  AND pe.source_grouped_id <> ''
  AND pj.status = 'sent'
  AND pj.channel_id = j.channel_id
  AND pj.target_chat_id = j.target_chat_id
  AND NOT EXISTS (
    SELECT 1
    FROM ` + pdaoCollectEventTable() + ` mid
    WHERE mid.tenant_id = e.tenant_id
      AND mid.account_id = e.account_id
      AND mid.source_id = e.source_id
      AND mid.source_chat_id = e.source_chat_id
      AND mid.source_message_id > pe.source_message_id
      AND mid.source_message_id < e.source_message_id
      AND mid.media_count > 0
  )
`
}

func collectContinuationVideoMediaSQL() string {
	return `
SELECT 1
FROM ` + publishMediaTable + ` m
WHERE m.profile_id = t.profile_id
  AND m.purpose = 'display'
  AND m.media_type = 'video'
  AND m.deleted_at IS NULL
`
}

func pdaoCollectEventTable() string {
	return pdao.YoubanPublishCollectEvent.Table()
}
