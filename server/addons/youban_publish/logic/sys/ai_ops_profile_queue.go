package sys

import (
	"context"
	"errors"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) AIOpsProfileClearQueue(ctx context.Context, tenantId, profileId int64) (*sysin.AIOpsProfileQueueClearModel, error) {
	if tenantId <= 0 || profileId <= 0 {
		return nil, gerror.New("租户或资料不能为空")
	}
	var jobs []telegramJobRecord
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Fields("id,operation_no,tenant_id,account_id,profile_id,channel_id,bot_id,target_chat_id,status,asynq_task_id,queue_name").Where("tenant_id", tenantId).Where("profile_id", profileId).WhereIn("status", []string{"pending", "sending", "confirming", "retry_wait", "failed_retry", "unknown"}).OrderDesc("id").Scan(&jobs); err != nil {
		return nil, gerror.Wrap(err, "读取资料活动发布任务失败")
	}
	res := &sysin.AIOpsProfileQueueClearModel{ProfileId: profileId}
	inspector := asynq.NewInspector(telegramQueueRedisOpt(ctx))
	defer inspector.Close()
	for i, job := range jobs {
		if i == 0 {
			res.KeptJobId = job.Id
			continue
		}
		if _, err := s.transitionTelegramJobs(ctx, []int64{job.Id}, telegramJobTransition{From: []string{"pending", "sending", "confirming", "retry_wait", "failed_retry", "unknown"}, To: telegramJobStatusSuperseded, ErrorMessage: "AI运维清理资料重复发布任务", Additional: g.Map{"last_dispatch_error": "AI运维清理资料重复发布任务"}}); err != nil {
			return nil, gerror.Wrap(err, "取消资料重复发布任务失败")
		}
		res.Cleared++
		if job.Status == "sending" || job.Status == "confirming" {
			_ = s.enqueueTelegramCleanupJob(ctx, job.Id, 0)
		} else if job.AsynqTaskId != "" && job.QueueName != "" {
			if err := inspector.DeleteTask(job.QueueName, job.AsynqTaskId); err != nil && !errors.Is(err, asynq.ErrTaskNotFound) {
				g.Log().Warningf(ctx, "删除资料重复排队任务失败 jobId:%d err:%+v", job.Id, err)
			}
		}
	}
	return res, nil
}
