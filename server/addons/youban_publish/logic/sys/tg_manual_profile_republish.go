package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func isManualProfilePublishOperation(operationNo string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(operationNo)), "profile:")
}

func (s *sSysPublish) prepareProfileChannelPublish(ctx context.Context, current telegramJobRecord) error {
	if !isManualProfilePublishOperation(current.OperationNo) || current.Id <= 0 || current.TenantId <= 0 || current.ProfileId <= 0 || current.ChannelId <= 0 {
		return nil
	}
	var previous []telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("id,task_id,operation_no,tenant_id,account_id,profile_id,channel_id,bot_id,target_chat_id,status").
		Where("tenant_id", current.TenantId).
		Where("profile_id", current.ProfileId).
		Where("target_chat_id", normalizeTelegramChannelChatID(current.TargetChatId)).
		WhereLT("id", current.Id).
		Where("operation_no NOT LIKE ?", "down:%").
		Where("(status IN('pending','failed_retry') OR (status IN('sent','superseded') AND EXISTS (SELECT 1 FROM " + publishTgMessageTable + " m WHERE m.job_id=" + publishTgJobTable + ".id AND m.status='sent')))").
		OrderAsc("id").
		Scan(&previous)
	if err != nil {
		return gerror.Wrap(err, "读取频道历史上架任务失败")
	}
	for _, job := range previous {
		if job.Status == "sent" || job.Status == "superseded" {
			if err = s.deleteTelegramMessageSetLockedByChannel(ctx, job, "资料重新上架"); err != nil {
				return gerror.Wrap(err, "删除频道历史上架消息失败")
			}
		}
		if err = s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return err
		}
		s.appendTelegramJobLog(ctx, job, "republish", "superseded", "资料重新上架，已由该频道的新上架任务替换")
	}
	return nil
}
