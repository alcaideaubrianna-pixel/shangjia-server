package sys

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

const terminalPublishRecoveryDelay = 10 * time.Minute

type terminalPublishRecoveryPayload struct {
	JobId int64 `json:"jobId"`
}

func terminalPublishRecoveryOperationNo(job telegramJobRecord) string {
	return fmt.Sprintf("auto-recover:%d:%d", job.Id, job.ProfileId)
}

func terminalPublishRecoveryEligible(job telegramJobRecord, cause error) bool {
	if job.Id <= 0 || job.ProfileId <= 0 || job.ChannelId <= 0 || isMessagePushOperationNo(job.OperationNo) {
		return false
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(job.OperationNo)), "auto-recover:") {
		return false
	}
	return !isTelegramPermanentAccountAuthError(cause) && !isTelegramPermanentSendError(cause)
}

func (s *sSysPublish) enqueueTerminalPublishRecoveryBestEffort(ctx context.Context, job telegramJobRecord, cause error) {
	if !terminalPublishRecoveryEligible(job, cause) {
		return
	}
	if err := s.enqueueTerminalPublishRecovery(ctx, job.Id); err != nil {
		g.Log().Warningf(ctx, "提交终态失败补偿任务失败 jobId:%d profileId:%d channelId:%d err:%+v", job.Id, job.ProfileId, job.ChannelId, err)
	}
}

func (s *sSysPublish) enqueueTerminalPublishRecovery(ctx context.Context, jobId int64) error {
	body, err := json.Marshal(terminalPublishRecoveryPayload{JobId: jobId})
	if err != nil {
		return gerror.Wrap(err, "编码终态失败补偿任务失败")
	}
	client, err := s.telegramQueueClient(ctx)
	if err != nil {
		return err
	}
	_, err = client.EnqueueContext(ctx, asynq.NewTask(tgTaskTypePublishRecovery, body),
		asynq.Queue(tgQueueNameProfileMaintenance),
		asynq.TaskID(fmt.Sprintf("publish-recovery:%d", jobId)),
		asynq.ProcessIn(terminalPublishRecoveryDelay),
		asynq.MaxRetry(5),
		asynq.Timeout(5*time.Minute),
	)
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return gerror.Wrap(err, "提交终态失败补偿任务失败")
}

func (s *sSysPublish) handleTerminalPublishRecoveryTask(ctx context.Context, task *asynq.Task) error {
	var payload terminalPublishRecoveryPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return gerror.Wrap(err, "解析终态失败补偿任务失败")
	}
	if payload.JobId <= 0 {
		return gerror.New("终态失败补偿任务参数不完整")
	}
	job, err := s.telegramJobById(ctx, payload.JobId)
	if err != nil {
		observeTelegramPublishRecovery(ctx, "failed")
		return err
	}
	if job.Id <= 0 || job.Status != "failed" || !terminalPublishRecoveryEligible(job, errors.New(jobErrorMessage(job))) {
		observeTelegramPublishRecovery(ctx, "skipped")
		return nil
	}
	current, err := s.profilePublishOperationIsCurrent(ctx, job)
	if err != nil {
		return err
	}
	if !current {
		observeTelegramPublishRecovery(ctx, "superseded")
		return nil
	}
	complete, err := collectProfileMediaComplete(ctx, job.ProfileId)
	if err != nil {
		return err
	}
	if !complete {
		_, err = s.AIOpsQueueBotMediaRepair(ctx, []int64{job.ProfileId}, 1)
		if err == nil {
			observeTelegramPublishRecovery(ctx, "media_repair")
		}
		return err
	}
	operationNo := terminalPublishRecoveryOperationNo(job)
	count, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("operation_no", operationNo).Where("profile_id", job.ProfileId).Where("channel_id", job.ChannelId).Count()
	if err != nil {
		return gerror.Wrap(err, "检查终态失败补偿任务幂等状态失败")
	}
	if count > 0 {
		observeTelegramPublishRecovery(ctx, "idempotent")
		return nil
	}
	if err = s.submitProfilePublish(ctx, job.ProfileId, job.TenantId, job.AccountId, 0, operationNo, []int64{job.ChannelId}, false); err != nil {
		if errors.Is(err, errPublishProfileUnavailable) {
			observeTelegramPublishRecovery(ctx, "unavailable")
			return nil
		}
		observeTelegramPublishRecovery(ctx, "failed")
		return gerror.Wrap(err, "重新提交终态失败资料失败")
	}
	observeTelegramPublishRecovery(ctx, "requeued")
	g.Log().Info(ctx, "终态失败资料已自动重新上架", g.Map{"sourceJobId": job.Id, "profileId": job.ProfileId, "channelId": job.ChannelId, "operationNo": operationNo})
	return nil
}

func jobErrorMessage(job telegramJobRecord) string {
	return job.ErrorMessage
}
