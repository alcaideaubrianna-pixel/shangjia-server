package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

func (s *sSysPublish) lockTelegramJob(ctx context.Context, jobId int64) (telegramJobRecord, bool, error) {
	var job telegramJobRecord
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).
		WhereIn("status", []string{"pending", "failed_retry"}).
		Data(g.Map{"status": "sending", "updated_at": gtime.Now()}).
		Update()
	if err != nil {
		return job, false, gerror.Wrap(err, "锁定TG任务失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return job, false, nil
	}
	if err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).
		Scan(&job); err != nil {
		return job, false, gerror.Wrap(err, "读取TG任务失败")
	}
	if job.Id <= 0 {
		return job, false, gerror.New("TG任务不存在")
	}
	return job, true, nil
}

func (s *sSysPublish) telegramJobMedia(ctx context.Context, job telegramJobRecord, purpose string) ([]*telegramMediaItem, error) {
	var rows []*telegramMediaItem
	mod := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("task_id", job.TaskId).
		Where("purpose", purpose).
		WhereNull("deleted_at").
		OrderAsc("sort_index").OrderAsc("id")
	if err := mod.Fields("id,media_type,purpose,file_url,poster_url,tg_file_id,tg_thumb_file_id,sort_index").Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取TG媒体失败")
	}
	return rows, nil
}

func (s *sSysPublish) telegramJobTask(ctx context.Context, taskId int64) (gdb.Record, error) {
	row, err := g.DB().Model(publishTaskTable).Safe().Ctx(ctx).
		Where("id", taskId).
		WhereNull("deleted_at").
		One()
	if err != nil {
		return nil, gerror.Wrap(err, "读取上架任务失败")
	}
	if row.IsEmpty() {
		return nil, gerror.New("上架任务不存在")
	}
	return row, nil
}

func (s *sSysPublish) saveTelegramSentMessages(ctx context.Context, job telegramJobRecord, messages []*telegramSentMessage) error {
	now := gtime.Now()
	for _, item := range messages {
		if item == nil || item.MessageId <= 0 {
			continue
		}
		_, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).Data(g.Map{
			"job_id":         job.Id,
			"task_id":        job.TaskId,
			"tenant_id":      job.TenantId,
			"account_id":     job.AccountId,
			"profile_id":     job.ProfileId,
			"bot_id":         job.BotId,
			"target_chat_id": job.TargetChatId,
			"tg_message_id":  item.MessageId,
			"media_group_id": item.MediaGroupId,
			"media_id":       item.MediaId,
			"purpose":        item.Purpose,
			"tg_file_id":     item.TgFileId,
			"status":         "sent",
			"sent_at":        now,
			"created_at":     now,
			"updated_at":     now,
		}).Insert()
		if err != nil {
			return gerror.Wrap(err, "保存TG消息记录失败")
		}
	}
	return nil
}

func (s *sSysPublish) appendTelegramJobLog(ctx context.Context, job telegramJobRecord, action string, status string, message string) {
	_, _ = g.DB().Model(publishTgJobLogTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id":     job.Id,
		"task_id":    job.TaskId,
		"tenant_id":  job.TenantId,
		"account_id": job.AccountId,
		"profile_id": job.ProfileId,
		"bot_id":     job.BotId,
		"action":     action,
		"status":     status,
		"message":    message,
		"created_at": gtime.Now(),
	}).Insert()
}
