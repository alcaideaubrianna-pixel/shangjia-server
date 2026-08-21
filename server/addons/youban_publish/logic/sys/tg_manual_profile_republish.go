package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func isManualProfilePublishOperation(operationNo string) bool {
	operationNo = strings.ToLower(strings.TrimSpace(operationNo))
	return strings.HasPrefix(operationNo, "profile:") || strings.HasPrefix(operationNo, "batchtext:")
}

func (s *sSysPublish) prepareProfileChannelPublish(ctx context.Context, current telegramJobRecord) (bool, error) {
	if current.Id <= 0 || current.TenantId <= 0 || current.ProfileId <= 0 || current.ChannelId <= 0 {
		return true, nil
	}
	preserveHistory, err := s.channelPreservesHistoryMessages(ctx, current.TenantId, current.ChannelId)
	if err != nil {
		return false, err
	}
	var previous []telegramJobRecord
	err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("id,task_id,operation_no,tenant_id,account_id,profile_id,channel_id,bot_id,target_chat_id,status").
		Where("tenant_id", current.TenantId).
		Where("profile_id", current.ProfileId).
		Where("target_chat_id", normalizeTelegramChannelChatID(current.TargetChatId)).
		WhereLT("id", current.Id).
		WhereIn("status", []string{"pending", "sending", "failed_retry", "unknown", "sent"}).
		OrderAsc("id").
		Scan(&previous)
	if err != nil {
		return false, gerror.Wrap(err, "读取频道历史上架任务失败")
	}
	if !preserveHistory {
		legacy, legacyErr := s.telegramSupersededJobsWithActiveMessages(ctx, current)
		if legacyErr != nil {
			return false, legacyErr
		}
		for _, job := range legacy {
			s.enqueueTelegramMessageDeleteFallback(ctx, job, "资料重新上架清理历史遗留消息", nil)
		}
	}
	previousJobIds := make([]int64, 0, len(previous))
	activeMessages := make(map[int64][]telegramDeleteMessage, len(previous))
	for _, job := range previous {
		messages, messageErr := s.telegramJobActiveMessages(ctx, job)
		if messageErr != nil {
			return false, messageErr
		}
		if len(messages) > 0 {
			activeMessages[job.Id] = messages
		}
		if shouldDeleteChannelHistory(preserveHistory, len(messages)) {
			if err = s.deleteTelegramMessagesLockedByChannel(ctx, job, messages, "资料重新上架"); err != nil {
				return false, gerror.Wrap(err, "删除频道历史上架消息失败")
			}
		}
		previousJobIds = append(previousJobIds, job.Id)
	}
	undeletableJobs := make(map[int64]bool)
	if !preserveHistory {
		undeletableJobs, err = s.telegramJobsWithUndeletableMessages(ctx, previousJobIds)
		if err != nil {
			return false, err
		}
	}
	for _, job := range previous {
		if len(activeMessages[job.Id]) > 0 && undeletableJobs[job.Id] {
			message := "频道中存在已超过Telegram删除时限的同资料旧消息，无法删除，将继续推送最新资料，频道内可能暂时保留历史消息"
			s.appendTelegramJobLog(ctx, current, "republish", "warning", message)
			g.Log().Warningf(ctx, "资料上架保留Telegram不可删除旧消息，继续推送新消息 profileId:%d channelId:%d oldJobId:%d currentJobId:%d", current.ProfileId, current.ChannelId, job.Id, current.Id)
		}
		if err = s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return false, err
		}
		if preserveHistory && len(activeMessages[job.Id]) > 0 {
			s.appendTelegramJobLog(ctx, job, "republish", "superseded", "资料重新上架，频道已配置保留旧消息")
		} else if len(activeMessages[job.Id]) > 0 {
			s.appendTelegramJobLog(ctx, job, "republish", "superseded", "资料重新上架，已由该频道的新上架任务替换")
		} else {
			s.appendTelegramJobLog(ctx, job, "publish", "superseded", "资料已有新的上架任务，旧待发送任务已废弃")
		}
	}
	return true, nil
}

func shouldDeleteChannelHistory(preserveHistory bool, messageCount int) bool {
	return !preserveHistory && messageCount > 0
}

func (s *sSysPublish) telegramSupersededJobsWithActiveMessages(ctx context.Context, current telegramJobRecord) ([]telegramJobRecord, error) {
	var jobs []telegramJobRecord
	err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		Fields("j.id,j.task_id,j.operation_no,j.tenant_id,j.account_id,j.profile_id,j.channel_id,j.bot_id,j.target_chat_id,j.status").
		Where("j.tenant_id", current.TenantId).
		Where("j.profile_id", current.ProfileId).
		Where("j.target_chat_id", normalizeTelegramChannelChatID(current.TargetChatId)).
		WhereLT("j.id", current.Id).
		Where("j.status", "superseded").
		Where("EXISTS (SELECT 1 FROM " + publishTgMessageTable + " m WHERE m.job_id=j.id AND m.status IN ('sent','undeletable'))").
		OrderAsc("j.id").Scan(&jobs)
	if err != nil {
		return nil, gerror.Wrap(err, "读取频道历史遗留消息失败")
	}
	return jobs, nil
}
