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

	telegramDeliveryStepPending   = "pending"
	telegramDeliveryStepRunning   = "running"
	telegramDeliveryStepSucceeded = "succeeded"
	telegramDeliveryStepSkipped   = "skipped"
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
		statuses[telegramDeliveryStepDisplay] = telegramDeliveryStepRunning
	case telegramSendPhaseDisplayConfirmed:
		statuses[telegramDeliveryStepCleanup] = telegramDeliveryStepSucceeded
		statuses[telegramDeliveryStepDisplay] = telegramDeliveryStepSucceeded
	case telegramSendPhaseVerifySending:
		statuses[telegramDeliveryStepCleanup] = telegramDeliveryStepSucceeded
		statuses[telegramDeliveryStepDisplay] = telegramDeliveryStepSucceeded
		statuses[telegramDeliveryStepVerify] = telegramDeliveryStepRunning
	case telegramSendPhaseVerifyConfirmed:
		statuses[telegramDeliveryStepCleanup] = telegramDeliveryStepSucceeded
		statuses[telegramDeliveryStepDisplay] = telegramDeliveryStepSucceeded
		statuses[telegramDeliveryStepVerify] = telegramDeliveryStepSucceeded
		statuses[telegramDeliveryStepFinalize] = telegramDeliveryStepSucceeded
	case telegramSendPhaseCompletedNoVerify:
		statuses[telegramDeliveryStepCleanup] = telegramDeliveryStepSucceeded
		statuses[telegramDeliveryStepDisplay] = telegramDeliveryStepSucceeded
		statuses[telegramDeliveryStepVerify] = telegramDeliveryStepSkipped
		statuses[telegramDeliveryStepFinalize] = telegramDeliveryStepSucceeded
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
		data := g.Map{
			"job_id": jobId, "step_key": step.StepKey, "status": step.Status,
			"created_at": now, "updated_at": now,
		}
		if step.Status == telegramDeliveryStepRunning {
			data["started_at"] = now
		}
		if step.Status == telegramDeliveryStepSucceeded || step.Status == telegramDeliveryStepSkipped {
			data["confirmed_at"] = now
		}
		if _, err := g.DB().Model(publishTgDeliveryStepTable).Safe().Ctx(ctx).
			Data(data).
			OnConflict("job_id,step_key").
			OnDuplicate("status,started_at,confirmed_at,updated_at").
			Save(); err != nil {
			return gerror.Wrapf(err, "投影TG发送步骤失败 jobId:%d step:%s", jobId, step.StepKey)
		}
	}
	return nil
}
