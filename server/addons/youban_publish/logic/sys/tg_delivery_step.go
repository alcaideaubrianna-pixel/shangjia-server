package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const publishTgDeliveryStepTable = "hg_youban_publish_tg_delivery_step"

const (
	telegramDeliveryStepCleanup  = "cleanup"
	telegramDeliveryStepDisplay  = "display"
	telegramDeliveryStepVerify   = "verify"
	telegramDeliveryStepFinalize = "finalize"

	telegramDeliveryStepPending    = "pending"
	telegramDeliveryStepRunning    = "running"
	telegramDeliveryStepConfirming = "confirming"
	telegramDeliveryStepSucceeded  = "succeeded"
	telegramDeliveryStepRetryWait  = "retry_wait"
	telegramDeliveryStepCanceled   = "canceled"
	telegramDeliveryStepSkipped    = "skipped"
)

type telegramDeliveryStepProjection struct {
	StepKey string
	Status  string
}

func telegramDeliveryStepsForPhase(phase string) []telegramDeliveryStepProjection {
	statuses := map[string]string{
		telegramDeliveryStepCleanup:  telegramDeliveryStepPending,
		telegramDeliveryStepDisplay:  telegramDeliveryStepPending,
		telegramDeliveryStepVerify:   telegramDeliveryStepPending,
		telegramDeliveryStepFinalize: telegramDeliveryStepPending,
	}
	switch strings.TrimSpace(phase) {
	case telegramSendPhaseCleanupProcessing:
		statuses[telegramDeliveryStepCleanup] = telegramDeliveryStepRunning
	case telegramSendPhaseCleanupConfirmed:
		statuses[telegramDeliveryStepCleanup] = telegramDeliveryStepSucceeded
	case telegramSendPhaseDisplaySending:
		statuses[telegramDeliveryStepCleanup] = telegramDeliveryStepSucceeded
	case telegramSendPhaseDisplayConfirmed:
		statuses[telegramDeliveryStepCleanup] = telegramDeliveryStepSucceeded
	case telegramSendPhaseVerifySending:
		statuses[telegramDeliveryStepCleanup] = telegramDeliveryStepSucceeded
	case telegramSendPhaseVerifyConfirmed:
		statuses[telegramDeliveryStepCleanup] = telegramDeliveryStepSucceeded
	case telegramSendPhaseCompletedNoVerify:
		statuses[telegramDeliveryStepCleanup] = telegramDeliveryStepSucceeded
		statuses[telegramDeliveryStepVerify] = telegramDeliveryStepSkipped
	}
	order := []string{telegramDeliveryStepCleanup, telegramDeliveryStepDisplay, telegramDeliveryStepVerify, telegramDeliveryStepFinalize}
	result := make([]telegramDeliveryStepProjection, 0, len(order))
	for _, stepKey := range order {
		result = append(result, telegramDeliveryStepProjection{StepKey: stepKey, Status: statuses[stepKey]})
	}
	return result
}

// projectTelegramDeliverySteps is an observational dual-write. Until reads are
// migrated, a projection failure must not turn a confirmed Telegram send into
// a retry and cause duplicate delivery.
func (s *sSysPublish) projectTelegramDeliverySteps(ctx context.Context, jobId int64, phase string) error {
	if jobId <= 0 {
		return gerror.New("TG任务ID不能为空")
	}
	now := gtime.Now()
	for _, step := range telegramDeliveryStepsForPhase(phase) {
		if err := s.projectTelegramDeliveryStep(ctx, jobId, step.StepKey, step.Status, 0, "", now); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) projectTelegramDeliveryStep(ctx context.Context, jobId int64, stepKey, status string, attemptId int64, message string, now *gtime.Time) error {
	if jobId <= 0 || strings.TrimSpace(stepKey) == "" || strings.TrimSpace(status) == "" {
		return gerror.New("TG发送步骤投影参数不完整")
	}
	if now == nil {
		now = gtime.Now()
	}
	data := g.Map{
		"job_id": jobId, "step_key": stepKey, "status": status,
		"error_message": strings.TrimSpace(message), "created_at": now, "updated_at": now,
	}
	duplicateFields := "status,error_message,updated_at"
	if status == telegramDeliveryStepPending && attemptId == 0 {
		// Phase projection initializes missing steps but never regresses an
		// Attempt-owned display/verify step back to pending.
		duplicateFields = "step_key"
	}
	if attemptId > 0 {
		data["attempt_id"] = attemptId
		duplicateFields += ",attempt_id"
	}
	if status == telegramDeliveryStepRunning {
		data["started_at"] = now
		duplicateFields += ",started_at"
	}
	if status == telegramDeliveryStepSucceeded || status == telegramDeliveryStepSkipped {
		data["confirmed_at"] = now
		duplicateFields += ",confirmed_at"
	}
	if _, err := g.DB().Model(publishTgDeliveryStepTable).Safe().Ctx(ctx).
		Data(data).OnConflict("job_id,step_key").OnDuplicate(duplicateFields).Save(); err != nil {
		return gerror.Wrapf(err, "投影TG发送步骤失败 jobId:%d step:%s", jobId, stepKey)
	}
	return nil
}

func telegramDeliveryStepForAttempt(attempt telegramDeliveryAttempt) (string, string) {
	stepKey := strings.TrimSpace(attempt.Phase)
	if stepKey != telegramDeliveryStepDisplay && stepKey != telegramDeliveryStepVerify {
		return "", ""
	}
	status := map[string]string{
		telegramAttemptStatusSending:    telegramDeliveryStepRunning,
		telegramAttemptStatusWaiting:    telegramDeliveryStepConfirming,
		telegramAttemptStatusConfirmed:  telegramDeliveryStepSucceeded,
		telegramAttemptStatusRetryWait:  telegramDeliveryStepRetryWait,
		telegramAttemptStatusSuperseded: telegramDeliveryStepCanceled,
	}[strings.TrimSpace(attempt.Status)]
	return stepKey, status
}

func (s *sSysPublish) projectTelegramDeliveryAttempt(ctx context.Context, attempt telegramDeliveryAttempt, message string) {
	stepKey, status := telegramDeliveryStepForAttempt(attempt)
	if attempt.Id <= 0 || attempt.JobId <= 0 || stepKey == "" || status == "" {
		return
	}
	if err := s.projectTelegramDeliveryStep(ctx, attempt.JobId, stepKey, status, attempt.Id, message, nil); err != nil {
		g.Log().Warningf(ctx, "投影TG Attempt步骤失败 jobId:%d attemptId:%d step:%s status:%s err:%+v", attempt.JobId, attempt.Id, stepKey, status, err)
	}
}
