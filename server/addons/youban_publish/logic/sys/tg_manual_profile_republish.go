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
	previousJobIds := make([]int64, 0, len(previous))
	for _, job := range previous {
		if err = s.deleteTelegramMessageSetLockedByChannel(ctx, job, "资料重新上架"); err != nil {
			return false, gerror.Wrap(err, "删除频道历史上架消息失败")
		}
		previousJobIds = append(previousJobIds, job.Id)
	}
	undeletableJobs, err := s.telegramJobsWithUndeletableMessages(ctx, previousJobIds)
	if err != nil {
		return false, err
	}
	for _, job := range previous {
		if undeletableJobs[job.Id] {
			message := "频道中存在已超过Telegram删除时限的同资料旧消息，无法删除，将继续推送最新资料，频道内可能暂时保留历史消息"
			s.appendTelegramJobLog(ctx, current, "republish", "warning", message)
			g.Log().Warningf(ctx, "资料上架保留Telegram不可删除旧消息，继续推送新消息 profileId:%d channelId:%d oldJobId:%d currentJobId:%d", current.ProfileId, current.ChannelId, job.Id, current.Id)
		}
		if err = s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return false, err
		}
		s.appendTelegramJobLog(ctx, job, "republish", "superseded", "资料重新上架，已由该频道的新上架任务替换")
	}
	return true, nil
}
