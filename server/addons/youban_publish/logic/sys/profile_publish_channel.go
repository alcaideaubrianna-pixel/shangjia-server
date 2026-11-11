package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

// prepareProfileChannelJobTx 复用统一任务工厂，将频道任务及其Outbox放在同一事务。
func (s *sSysPublish) prepareProfileChannelJobTx(ctx context.Context, tx gdb.TX, source gdb.Record, channel telegramJobChannel, operationNo string, meta telegramProfilePublishMeta) (int64, error) {
	allowed, err := cycleChannelAllowed(ctx, source["tenant_id"].Int64(), channel.Id, operationNo)
	if err != nil || !allowed {
		return 0, err
	}
	jobID, err := s.ensureTelegramProfileJobWithMetaTx(ctx, tx, source, channel, operationNo, meta)
	if err != nil || jobID == 0 {
		return jobID, err
	}
	if strings.TrimSpace(meta.InitialSendPhase) != "" {
		if _, err := tx.Model(publishTgJobTable).Ctx(ctx).Where("id", jobID).Where("status", telegramJobStatusPending).Data(g.Map{"send_phase": strings.TrimSpace(meta.InitialSendPhase)}).Update(); err != nil {
			return 0, err
		}
	}
	if err := s.ensureTelegramJobOutboxTx(ctx, tx, jobID, 0); err != nil {
		return 0, err
	}
	return jobID, nil
}

// recordProfileChannelPreparationFailureTx 保留失败频道的独立任务供用户查看，失败不得回滚其他频道或伪造成功。
func (s *sSysPublish) recordProfileChannelPreparationFailureTx(ctx context.Context, tx gdb.TX, source gdb.Record, channel telegramJobChannel, operationNo string, meta telegramProfilePublishMeta, cause error) (int64, error) {
	job := telegramJobRecord{TenantId: source["tenant_id"].Int64(), AccountId: source["account_id"].Int64(), ProfileId: source["profile_id"].Int64(), ChannelId: channel.Id, TargetChatId: normalizeTelegramChannelChatID(channel.TargetChatId), OperationNo: operationNo, Priority: meta.InitialPriority}
	return tx.Model(publishTgJobTable).Data(g.Map{
		"tenant_id": job.TenantId, "merchant_id": job.TenantId, "account_id": job.AccountId, "notify_account_id": job.AccountId,
		"profile_id": job.ProfileId, "channel_id": job.ChannelId, "target_chat_id": job.TargetChatId, "operation_no": operationNo, "operation_type": string(profileOperationTypeFromNo(operationNo)),
		"status": telegramJobStatusFailed, "dispatch_status": tgDispatchStatusDone, "priority": s.telegramJobPriority(job),
		"collect_event_id": meta.CollectEventId, "collect_source_id": meta.CollectSourceId,
		"error_message": "准备频道推送失败：" + cause.Error(), "profile_no_snapshot": source["profile_no"].String(), "title_snapshot": source["title"].String(),
	}).InsertAndGetId()
}
