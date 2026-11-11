package sys

import (
	"context"
	"encoding/json"
	"errors"
	"hotgo/addons/youban_publish/model/input/sysin"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

const tgTaskTypeProfileProjection = "youban_publish:profile:operation_projection"

type streamProfileProjectionPayload struct {
	TenantId    int64  `json:"tenantId"`
	AccountId   int64  `json:"accountId"`
	ProfileId   int64  `json:"profileId"`
	OperationNo string `json:"operationNo"`
}

// enqueueStreamProfileProjection 仅在整份资料的频道任务均已终态时提交合并投影，不让每个频道争抢资料锁。
func (s *sSysPublish) enqueueStreamProfileProjection(ctx context.Context, job telegramJobRecord) error {
	pending, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).Where("account_id", job.AccountId).
		Where("profile_id", job.ProfileId).Where("operation_no", job.OperationNo).
		WhereNotIn("status", []string{telegramJobStatusSent, telegramJobStatusSuperseded, telegramJobStatusFailed}).
		Fields("id").Limit(1).Value()
	if err != nil {
		return gerror.Wrap(err, "检查资料投影完成条件失败")
	}
	if !pending.IsNil() && pending.Int64() > 0 {
		return nil
	}
	body, err := json.Marshal(streamProfileProjectionPayload{TenantId: job.TenantId, AccountId: job.AccountId, ProfileId: job.ProfileId, OperationNo: job.OperationNo})
	if err != nil {
		return err
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypeProfileProjection, body),
		asynq.Queue(tgQueueNameBackground), asynq.Unique(time.Minute), asynq.Timeout(time.Minute), asynq.MaxRetry(10))
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

// handleStreamProfileProjection 使用已有后台执行角色处理合并投影，失败由Asynq重试，不重发TG消息。
func (s *sSysPublish) handleStreamProfileProjection(ctx context.Context, task *asynq.Task) error {
	var payload streamProfileProjectionPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	if payload.TenantId <= 0 || payload.AccountId <= 0 || payload.ProfileId <= 0 || strings.TrimSpace(payload.OperationNo) == "" {
		return gerror.New("资料操作投影参数不完整")
	}
	var job telegramJobRecord
	if err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", payload.TenantId).Where("account_id", payload.AccountId).
		Where("profile_id", payload.ProfileId).Where("operation_no", payload.OperationNo).
		WhereIn("status", []string{telegramJobStatusSent, telegramJobStatusFailed}).Order("CASE WHEN status='sent' THEN 0 ELSE 1 END,id DESC").Limit(1).Scan(&job); err != nil {
		return err
	}
	if job.Id <= 0 {
		return nil
	}
	if job.Status == telegramJobStatusFailed {
		return s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed)
	}
	return s.projectCompletedTelegramOperation(ctx, job)
}

// projectCompletedTelegramOperation 复用既有状态、统计、循环清理和通知能力；新引擎仅在后台合并执行。
func (s *sSysPublish) projectCompletedTelegramOperation(ctx context.Context, job telegramJobRecord) error {
	operationCompleted := false
	if err := s.withProfileLifecycleLock(ctx, job.TenantId, job.ProfileId, func() error {
		return s.completeTelegramJobLockedByProfile(ctx, job, &operationCompleted)
	}); err != nil {
		return err
	}
	if !operationCompleted {
		return nil
	}
	reserved, err := reserveBotProfilePublishNotice(ctx, job.ProfileId, job.OperationNo)
	if err != nil {
		return gerror.Wrap(err, "占用Bot资料上架通知幂等锁失败")
	}
	if !reserved {
		return nil
	}
	if err := s.notifyBotProfilePublishCompleted(ctx, job); err != nil {
		g.Log().Warningf(ctx, "发送Bot资料上架完成通知失败 profileId:%d operationNo:%s err:%+v", job.ProfileId, job.OperationNo, err)
	}
	return nil
}
