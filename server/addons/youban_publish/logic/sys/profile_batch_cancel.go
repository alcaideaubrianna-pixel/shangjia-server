package sys

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/hibiken/asynq"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const adminBatchTextCancelMessage = "批量文案操作已取消"

var adminBatchTextIdPattern = regexp.MustCompile(`^[A-Za-z0-9-]{8,48}$`)

func normalizeAdminBatchTextId(batchId string) (string, error) {
	batchId = strings.TrimSpace(batchId)
	if !adminBatchTextIdPattern.MatchString(batchId) {
		return "", gerror.New("批量操作ID不合法")
	}
	return batchId, nil
}

func adminBatchTextOperationPrefix(operatorId int64, batchId string) (string, error) {
	batchId, err := normalizeAdminBatchTextId(batchId)
	if err != nil {
		return "", err
	}
	if operatorId <= 0 {
		return "", gerror.New("操作账号不能为空")
	}
	return fmt.Sprintf("batchtext:%d:%s:profile:", operatorId, batchId), nil
}

func adminBatchTextOperationNo(operatorId int64, batchId string, profileId int64) (string, error) {
	if strings.TrimSpace(batchId) == "" {
		return "", nil
	}
	prefix, err := adminBatchTextOperationPrefix(operatorId, batchId)
	if err != nil {
		return "", err
	}
	if profileId <= 0 {
		return "", gerror.New("资料ID不能为空")
	}
	return fmt.Sprintf("%s%d", prefix, profileId), nil
}

func (s *sSysPublish) AdminProfileBatchCancel(ctx context.Context, in *sysin.AdminProfileBatchCancelInp) (*sysin.AdminProfileBatchCancelModel, error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("批量操作ID不能为空")
	}
	prefix, err := adminBatchTextOperationPrefix(account.Id, in.BatchId)
	if err != nil {
		return nil, err
	}

	var jobs []telegramJobRecord
	err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("id,operation_no,tenant_id,account_id,profile_id,channel_id,bot_id,target_chat_id,status,asynq_task_id,queue_name").
		Where("tenant_id", account.TenantId).
		WhereLike("operation_no", prefix+"%").
		WhereIn("status", []string{"pending", "failed_retry", "unknown", "sending", "superseded"}).
		OrderAsc("id").
		Scan(&jobs)
	if err != nil {
		return nil, gerror.Wrap(err, "读取批量发布任务失败")
	}

	res := &sysin.AdminProfileBatchCancelModel{}
	cancelable := make([]telegramJobRecord, 0, len(jobs))
	sendingJobIds := make(map[int64]struct{})
	for _, job := range jobs {
		if job.Status == "sending" {
			res.Sending++
			sendingJobIds[job.Id] = struct{}{}
		}
		if job.Status != "superseded" {
			cancelable = append(cancelable, job)
		}
	}
	ids := make([]int64, 0, len(cancelable))
	for _, job := range cancelable {
		ids = append(ids, job.Id)
	}
	if len(ids) > 0 {
		result, updateErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			WhereIn("id", ids).
			WhereIn("status", []string{"pending", "failed_retry", "unknown", "sending"}).
			Data(g.Map{
				"status":              "superseded",
				"dispatch_status":     tgDispatchStatusDone,
				"next_retry_at":       nil,
				"error_message":       adminBatchTextCancelMessage,
				"last_dispatch_error": adminBatchTextCancelMessage,
				"updated_at":          gtime.Now(),
			}).
			Update()
		if updateErr != nil {
			return nil, gerror.Wrap(updateErr, "取消批量发布任务失败")
		}
		affected, _ := result.RowsAffected()
		res.Canceled = int(affected)
	}
	var canceledJobs []telegramJobRecord
	if res.Canceled > 0 {
		err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Fields("id,operation_no,tenant_id,account_id,profile_id,channel_id,bot_id,target_chat_id,status,asynq_task_id,queue_name").
			WhereIn("id", ids).
			Where("status", "superseded").
			Where("error_message", adminBatchTextCancelMessage).
			OrderAsc("id").
			Scan(&canceledJobs)
		if err != nil {
			return nil, gerror.Wrap(err, "读取已取消批量发布任务失败")
		}
	}

	inspector := asynq.NewInspector(telegramQueueRedisOpt(ctx))
	defer inspector.Close()
	for _, job := range canceledJobs {
		if _, sending := sendingJobIds[job.Id]; sending {
			if cleanupErr := s.enqueueTelegramCleanupJob(ctx, job.Id, 0); cleanupErr != nil {
				g.Log().Warningf(ctx, "加入批量发布消息清理队列失败 jobId:%d err:%+v", job.Id, cleanupErr)
			}
		} else if job.AsynqTaskId != "" && job.QueueName != "" {
			if deleteErr := inspector.DeleteTask(job.QueueName, job.AsynqTaskId); deleteErr != nil && !errors.Is(deleteErr, asynq.ErrTaskNotFound) {
				g.Log().Warningf(ctx, "删除批量发布排队任务失败 jobId:%d taskId:%s queue:%s err:%+v", job.Id, job.AsynqTaskId, job.QueueName, deleteErr)
			}
		}
		s.appendTelegramJobLog(ctx, job, "publish", "superseded", adminBatchTextCancelMessage)
		if recordErr := s.upsertPublishJobRecord(ctx, job, "superseded", adminBatchTextCancelMessage); recordErr != nil {
			g.Log().Warningf(ctx, "更新批量发布取消记录失败 jobId:%d err:%+v", job.Id, recordErr)
		}
	}

	operations := make(map[string]telegramJobRecord)
	for _, job := range jobs {
		operations[job.OperationNo] = job
	}
	for operationNo, job := range operations {
		active, countErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("tenant_id", account.TenantId).
			Where("operation_no", operationNo).
			WhereIn("status", []string{"pending", "sending", "failed_retry", "unknown"}).
			Count()
		if countErr != nil {
			g.Log().Warningf(ctx, "检查批量发布剩余任务失败 operationNo:%s err:%+v", operationNo, countErr)
			continue
		}
		if active == 0 {
			if clearErr := s.clearProfilePublishOperationState(ctx, job); clearErr != nil {
				g.Log().Warningf(ctx, "清理批量发布状态失败 profileId:%d operationNo:%s err:%+v", job.ProfileId, operationNo, clearErr)
			}
		}
	}
	return res, nil
}
