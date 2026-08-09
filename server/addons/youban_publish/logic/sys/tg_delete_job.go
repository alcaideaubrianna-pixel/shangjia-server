package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (s *sSysPublish) CleanupTelegramJobMessages(ctx context.Context, jobId int64) error {
	job, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	lease, ok, err := s.tryTelegramChannelLease(ctx, job.TargetChatId)
	if err != nil {
		return err
	}
	if !ok {
		delay := s.telegramChannelBusyDelay(ctx, jobId, job.DispatchCount)
		return s.requeueTelegramJob(ctx, tgTaskTypeCleanup, jobId, delay)
	}
	defer s.releaseTelegramChannelLease(ctx, lease)
	return s.deleteTelegramMessageSetLockedByChannel(ctx, job, "资料清理")
}

func (s *sSysPublish) telegramJobById(ctx context.Context, jobId int64) (telegramJobRecord, error) {
	var job telegramJobRecord
	if jobId <= 0 {
		return job, gerror.New("TG任务ID不能为空")
	}
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).Scan(&job); err != nil {
		return job, gerror.Wrap(err, "读取TG任务失败")
	}
	if job.Id <= 0 {
		return job, gerror.New("TG任务不存在")
	}
	return job, nil
}

func (s *sSysPublish) telegramJobActiveMessages(ctx context.Context, job telegramJobRecord, purpose ...string) ([]telegramDeleteMessage, error) {
	var rows []telegramDeleteMessage
	mod := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Fields("id,target_chat_id,tg_message_id AS message_id").
		Where("job_id", job.Id).
		Where("status", "sent")
	if len(purpose) > 0 && strings.TrimSpace(purpose[0]) != "" {
		mod = mod.Where("purpose", strings.TrimSpace(purpose[0]))
	}
	err := mod.
		OrderAsc("id").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取TG消息记录失败")
	}
	return rows, nil
}

func (s *sSysPublish) telegramJobsWithUndeletableMessages(ctx context.Context, jobIds []int64) (map[int64]bool, error) {
	jobIds = uniqueIds(jobIds)
	result := make(map[int64]bool, len(jobIds))
	if len(jobIds) == 0 {
		return result, nil
	}
	var rows []struct {
		JobId int64 `orm:"job_id"`
	}
	err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Fields("job_id").
		WhereIn("job_id", jobIds).
		Where("status", "undeletable").
		Group("job_id").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "批量检查TG历史消息删除状态失败")
	}
	for _, row := range rows {
		result[row.JobId] = true
	}
	return result, nil
}

func (s *sSysPublish) telegramJobsWithActiveMessages(ctx context.Context, tenantId int64, profileIds []int64, cutoffAt string) ([]telegramResubmitJob, error) {
	profileIds = uniqueIds(profileIds)
	if tenantId <= 0 || len(profileIds) == 0 {
		return []telegramResubmitJob{}, nil
	}
	var jobs []telegramResubmitJob
	mod := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		Fields("j.*").
		Where("j.tenant_id", tenantId).
		WhereIn("j.profile_id", profileIds).
		Where("EXISTS (SELECT 1 FROM " + publishTgMessageTable + " m WHERE m.job_id=j.id AND m.status='sent')")
	if strings.TrimSpace(cutoffAt) != "" {
		mod = mod.WhereLTE("j.created_at", cutoffAt)
	}
	if err := mod.OrderAsc("j.id").Scan(&jobs); err != nil {
		return nil, gerror.Wrap(err, "读取待清理TG历史消息失败")
	}
	return jobs, nil
}

type telegramDeleteMessage struct {
	Id           int64  `json:"id"`
	TargetChatId string `json:"targetChatId"`
	MessageId    int64  `json:"messageId"`
}
