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

func (s *sSysPublish) prepareProfileChannelPublish(ctx context.Context, current telegramJobRecord) (bool, error) {
	if current.Id <= 0 || current.TenantId <= 0 || current.ProfileId <= 0 || current.ChannelId <= 0 {
		return true, nil
	}
	var previous []telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("id,task_id,operation_no,tenant_id,account_id,profile_id,channel_id,bot_id,target_chat_id,status").
		Where("tenant_id", current.TenantId).
		Where("profile_id", current.ProfileId).
		Where("target_chat_id", normalizeTelegramChannelChatID(current.TargetChatId)).
		WhereLT("id", current.Id).
		WhereIn("status", []string{"pending", "sending", "failed_retry", "sent", "superseded"}).
		OrderAsc("id").
		Scan(&previous)
	if err != nil {
		return false, gerror.Wrap(err, "读取频道历史上架任务失败")
	}
	for _, job := range previous {
		if err = s.deleteTelegramMessageSetLockedByChannel(ctx, job, "资料重新上架"); err != nil {
			return false, gerror.Wrap(err, "删除频道历史上架消息失败")
		}
		if hasUndeletable, checkErr := s.telegramJobHasUndeletableMessages(ctx, job.Id); checkErr != nil {
			return false, checkErr
		} else if hasUndeletable {
			s.appendTelegramJobLog(ctx, current, "republish", "skipped", "频道中已有同资料消息且已超过Telegram可删除时限，跳过重复推送")
			return false, nil
		}
		if err = s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return false, err
		}
		s.appendTelegramJobLog(ctx, job, "republish", "superseded", "资料重新上架，已由该频道的新上架任务替换")
	}
	return true, nil
}
