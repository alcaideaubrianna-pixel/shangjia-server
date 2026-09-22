package sys

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const (
	telegramSendPhaseCleanupProcessing = "cleanup_processing"
	telegramSendPhaseCleanupConfirmed  = "cleanup_confirmed"
	telegramSendPhaseDisplaySending    = "display_sending"
	telegramSendPhaseDisplayConfirmed  = "display_confirmed"
	telegramSendPhaseVerifySending     = "verify_sending"
	telegramSendPhaseVerifyConfirmed   = "verify_confirmed"
	// Telegram does not provide an idempotency key for send requests. An
	// ambiguous response therefore needs an asynchronous reconciliation window
	// before the job is allowed to retry.
	telegramUnknownReconcileDelay = 10 * time.Second
)

func telegramSendPhaseHasCleanup(phase string) bool {
	switch strings.TrimSpace(phase) {
	case telegramSendPhaseCleanupConfirmed,
		telegramSendPhaseDisplaySending, telegramSendPhaseDisplayConfirmed,
		telegramSendPhaseVerifySending, telegramSendPhaseVerifyConfirmed:
		return true
	default:
		return false
	}
}

func telegramSendPhaseIsCleanup(phase string) bool {
	return strings.TrimSpace(phase) == telegramSendPhaseCleanupProcessing
}

func telegramSendPhaseHasDisplay(phase string) bool {
	switch strings.TrimSpace(phase) {
	case telegramSendPhaseDisplayConfirmed, telegramSendPhaseVerifySending, telegramSendPhaseVerifyConfirmed:
		return true
	default:
		return false
	}
}

func telegramCaptionWithJobMarker(caption string, jobId int64, purpose string) string {
	return caption + telegramJobPhaseMarker(jobId, purpose)
}

func telegramJobPhaseMarker(jobId int64, purpose string) string {
	sum := sha256.Sum256([]byte(strconv.FormatInt(jobId, 10) + ":" + strings.TrimSpace(purpose)))
	var marker strings.Builder
	marker.WriteRune('\u2063')
	for _, value := range sum[:8] {
		for bit := 7; bit >= 0; bit-- {
			if value&(1<<bit) == 0 {
				marker.WriteRune('\u200b')
			} else {
				marker.WriteRune('\u200c')
			}
		}
	}
	marker.WriteRune('\u2064')
	return marker.String()
}

func isTelegramAmbiguousDeliveryError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errTelegramDeliveryUncertain) {
		return true
	}
	message := strings.ToLower(err.Error())
	// Media preparation/download failures happen before Telegram accepts a
	// request, so delivery is known to have failed and must be retried normally.
	for _, part := range []string{"下载远程媒体失败", "保存媒体缓存临时文件失败", "创建媒体缓存临时文件失败", "写入媒体缓存文件失败"} {
		if strings.Contains(message, strings.ToLower(part)) {
			return false
		}
	}
	for _, part := range []string{"context deadline exceeded", "client.timeout exceeded", "closed pipe", "broken pipe", "goaway", "cannot rewind body", "connection reset", "unexpected eof"} {
		if strings.Contains(message, part) {
			return true
		}
	}
	return false
}

func telegramDeliveryUncertainError(err error) error {
	if err == nil {
		return errTelegramDeliveryUncertain
	}
	return fmt.Errorf("%w：%v", errTelegramDeliveryUncertain, err)
}

func (s *sSysPublish) updateTelegramJobSendPhase(ctx context.Context, jobId int64, phase string) error {
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).Where("status", "sending").
		Data(g.Map{"send_phase": phase, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新TG发送阶段失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return gerror.New("TG任务已不在发送状态，停止更新发送阶段")
	}
	return nil
}

func (s *sSysPublish) markTelegramJobUnknown(ctx context.Context, job telegramJobRecord, cause error) error {
	causeMessage := "未知原因"
	if cause != nil {
		causeMessage = cause.Error()
	}
	message := "Telegram 返回结果不确定，已暂停重发并等待频道消息对账：" + causeMessage
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("status", "sending").Data(g.Map{
		"status": "unknown", "dispatch_status": tgDispatchStatusIdle, "next_retry_at": gtime.Now().Add(telegramUnknownReconcileDelay),
		"error_message": message, "reconcile_count": 0, "updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "标记TG任务待对账失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	s.appendTelegramJobLog(ctx, job, "publish", "unknown", message)
	return nil
}

func (s *sSysPublish) reconcileUnknownTelegramJob(ctx context.Context, job telegramJobRecord) error {
	claimed, err := s.claimUnknownTelegramJob(ctx, job.Id)
	if err != nil || !claimed {
		return err
	}
	job, err = s.telegramJobById(ctx, job.Id)
	if err != nil {
		return s.releaseUnknownTelegramJobClaim(ctx, job.Id, err)
	}
	phase := telegramSendPhaseCleanupConfirmed
	if job.SendPhase == telegramSendPhaseVerifySending {
		phase = telegramSendPhaseDisplayConfirmed
	}
	message := "历史待对账任务已停止主动扫描频道，移到队列尾部重新发送"
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("status", "unknown").Data(g.Map{
		"status": "failed_retry", "send_phase": phase, "dispatch_status": tgDispatchStatusIdle,
		"retry_count": job.RetryCount + 1, "reconcile_count": 0, "next_retry_at": nil,
		"error_message": message, "updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "迁移历史TG待对账任务失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	observeTelegramReconcile(ctx, "legacy_requeued")
	s.appendTelegramJobLog(ctx, job, "reconcile", "retired", message)
	return s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false)
}

func (s *sSysPublish) claimUnknownTelegramJob(ctx context.Context, jobId int64) (bool, error) {
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).Where("status", "unknown").
		WhereIn("dispatch_status", []string{tgDispatchStatusQueued, tgDispatchStatusProcessing}).
		Data(g.Map{"dispatch_status": tgDispatchStatusProcessing, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return false, gerror.Wrap(err, "领取TG对账任务失败")
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (s *sSysPublish) releaseUnknownTelegramJobClaim(ctx context.Context, jobId int64, cause error) error {
	message := "释放TG对账任务失败"
	if cause != nil {
		message = "TG对账任务读取失败，等待重试：" + cause.Error()
	}
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", jobId).Where("status", "unknown").
		Data(g.Map{"dispatch_status": tgDispatchStatusIdle, "next_retry_at": gtime.Now().Add(time.Minute), "error_message": message, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "释放TG对账任务领取失败")
	}
	return nil
}
