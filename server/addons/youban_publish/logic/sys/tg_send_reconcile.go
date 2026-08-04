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
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

const (
	telegramSendPhaseDisplaySending   = "display_sending"
	telegramSendPhaseDisplayConfirmed = "display_confirmed"
	telegramSendPhaseVerifySending    = "verify_sending"
	telegramSendPhaseVerifyConfirmed  = "verify_confirmed"
	telegramUnknownReconcileDelay     = 20 * time.Second
	telegramUnknownReconcileMaxCount  = 2
)

type telegramReconcileChannel struct {
	Id           int64  `json:"id"`
	TgAccountId  int64  `json:"tgAccountId"`
	TargetChatId string `json:"targetChatId"`
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
	message := "Telegram 返回结果不确定，已暂停重发并等待频道消息对账：" + cause.Error()
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
	purpose := "display"
	if job.SendPhase == telegramSendPhaseVerifySending {
		purpose = "verify"
	}
	expectedCount, err := s.telegramJobPhaseExpectedMessageCount(ctx, job, purpose)
	if err != nil {
		return s.postponeUnknownTelegramJob(ctx, job, err)
	}
	messages, err := s.findTelegramJobPhaseMessages(ctx, job, purpose, expectedCount)
	if err != nil {
		return s.postponeUnknownTelegramJob(ctx, job, err)
	}
	if len(messages) == 0 {
		return s.postponeUnknownTelegramJob(ctx, job, nil)
	}
	if err = s.saveTelegramSentMessages(ctx, job, messages); err != nil {
		return s.postponeUnknownTelegramJob(ctx, job, err)
	}
	if len(messages) < expectedCount {
		return s.recoverIncompleteTelegramJobPhase(ctx, job, purpose, len(messages), expectedCount)
	}
	if purpose == "verify" {
		result, updateErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("status", "unknown").Data(g.Map{
			"status": "sending", "send_phase": telegramSendPhaseVerifyConfirmed, "dispatch_status": tgDispatchStatusProcessing, "error_message": "", "updated_at": gtime.Now(),
		}).Update()
		if updateErr != nil {
			return s.postponeUnknownTelegramJob(ctx, job, updateErr)
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return nil
		}
		if err = s.completeTelegramJob(ctx, job); err != nil {
			return s.handleTelegramJobError(ctx, job, gerror.Wrap(err, "完成TG对账任务失败"))
		}
		return nil
	}
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("status", "unknown").Data(g.Map{
		"status": "pending", "send_phase": telegramSendPhaseDisplayConfirmed, "dispatch_status": tgDispatchStatusIdle,
		"next_retry_at": nil, "error_message": "", "reconcile_count": 0, "updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return s.postponeUnknownTelegramJob(ctx, job, err)
	}
	return nil
}

func (s *sSysPublish) telegramJobPhaseExpectedMessageCount(ctx context.Context, job telegramJobRecord, purpose string) (int, error) {
	media, err := s.telegramJobMedia(ctx, job, purpose)
	if err != nil {
		return 0, err
	}
	if purpose == "display" {
		media, err = s.selectTelegramDisplayMediaForTenant(ctx, job, media)
		if err != nil {
			return 0, err
		}
		if len(media) == 0 {
			return 1, nil
		}
	}
	return len(media), nil
}

func (s *sSysPublish) recoverIncompleteTelegramJobPhase(ctx context.Context, job telegramJobRecord, purpose string, foundCount, expectedCount int) error {
	message := fmt.Sprintf("TG%s资料仅对账到%d/%d条消息，清理不完整消息后重新发送", purpose, foundCount, expectedCount)
	if err := s.deleteTelegramMessagePurposeSetLockedByChannel(ctx, job, purpose, message); err != nil {
		return s.postponeUnknownTelegramJob(ctx, job, err)
	}
	phase := ""
	if purpose == "verify" {
		phase = telegramSendPhaseDisplayConfirmed
	}
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("status", "unknown").Data(g.Map{
		"status": "failed_retry", "send_phase": phase, "dispatch_status": tgDispatchStatusIdle,
		"next_retry_at": gtime.Now(), "error_message": message, "reconcile_count": 0, "updated_at": gtime.Now(),
	}).Update()
	if err == nil {
		s.appendTelegramJobLog(ctx, job, "reconcile", "partial", message)
	}
	return err
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

func (s *sSysPublish) postponeUnknownTelegramJob(ctx context.Context, job telegramJobRecord, cause error) error {
	if cause != nil {
		message := "TG频道消息对账暂时不可用，任务继续保持UNKNOWN：" + cause.Error()
		_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("status", "unknown").Data(g.Map{
			"dispatch_status": tgDispatchStatusIdle, "next_retry_at": gtime.Now().Add(time.Minute),
			"error_message": message, "updated_at": gtime.Now(),
		}).Update()
		if err == nil {
			s.appendTelegramJobLog(ctx, job, "reconcile", "waiting", message)
		}
		return err
	}
	count := job.ReconcileCount + 1
	if count < telegramUnknownReconcileMaxCount {
		_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("status", "unknown").Data(g.Map{
			"reconcile_count": count, "dispatch_status": tgDispatchStatusIdle, "next_retry_at": gtime.Now().Add(telegramUnknownReconcileDelay), "updated_at": gtime.Now(),
		}).Update()
		return err
	}
	message := "频道历史连续两次未发现对应消息，恢复正常重试"
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("status", "unknown").Data(g.Map{
		"status": "failed_retry", "dispatch_status": tgDispatchStatusIdle, "retry_count": job.RetryCount + 1,
		"next_retry_at": gtime.Now().Add(telegramRecoverableRetryDelay(cause, job.RetryCount+1)), "error_message": message,
		"reconcile_count": count, "updated_at": gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) findTelegramJobPhaseMessages(ctx context.Context, job telegramJobRecord, purpose string, expectedCount int) ([]*telegramSentMessage, error) {
	var channel telegramReconcileChannel
	if err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).Fields("id,tg_account_id,target_chat_id").Where("id", job.ChannelId).Where("tenant_id", job.TenantId).Scan(&channel); err != nil {
		return nil, gerror.Wrap(err, "读取TG对账频道失败")
	}
	if channel.TgAccountId <= 0 {
		return nil, gerror.New("频道未绑定协议号，无法自动对账")
	}
	cache, err := s.tgChannelCacheByChannelId(ctx, job.TenantId, channel.TgAccountId, channel.TargetChatId)
	if err != nil {
		return nil, err
	}
	channelId, err := strconv.ParseInt(cache.ChannelId, 10, 64)
	if err != nil {
		return nil, gerror.New("频道ID无效，无法自动对账")
	}
	accessHash, err := strconv.ParseInt(cache.AccessHash, 10, 64)
	if err != nil {
		return nil, gerror.New("频道AccessHash无效，无法自动对账")
	}
	marker := telegramJobPhaseMarker(job.Id, purpose)
	var found []*telegramSentMessage
	err = s.executeTelegramAccountOperation(ctx, channel.TgAccountId, 45*time.Second, func(runCtx context.Context, client *telegram.Client) error {
		cutoff := time.Now().Add(-15 * time.Minute).Unix()
		if job.CreatedAt != nil {
			cutoff = job.CreatedAt.Add(-2 * time.Minute).Unix()
		}
		offsetId := 0
		for page := 0; page < 10; page++ {
			res, requestErr := client.API().MessagesGetHistory(runCtx, &tg.MessagesGetHistoryRequest{Peer: &tg.InputPeerChannel{ChannelID: channelId, AccessHash: accessHash}, OffsetID: offsetId, Limit: 100})
			if requestErr != nil {
				return requestErr
			}
			items := tgHistoryMessages(res)
			if len(items) == 0 {
				break
			}
			groupIds := make(map[int64]struct{})
			stop := false
			for _, item := range items {
				if item == nil || item.ID <= 0 {
					continue
				}
				if int64(item.Date) < cutoff {
					stop = true
					continue
				}
				if offsetId == 0 || item.ID < offsetId {
					offsetId = item.ID
				}
				if strings.Contains(item.Message, marker) {
					groupIds[item.GroupedID] = struct{}{}
				}
			}
			for _, item := range items {
				if item == nil {
					continue
				}
				_, sameGroup := groupIds[item.GroupedID]
				if strings.Contains(item.Message, marker) || item.GroupedID > 0 && sameGroup {
					found = append(found, telegramReconciledMessage(item, purpose))
				}
			}
			if expectedCount > 0 && len(found) >= expectedCount || stop || offsetId <= 0 {
				break
			}
		}
		return nil
	})
	return found, err
}

func telegramReconciledMessage(item *tg.Message, purpose string) *telegramSentMessage {
	groupId := ""
	if item.GroupedID > 0 {
		groupId = strconv.FormatInt(item.GroupedID, 10)
	}
	return &telegramSentMessage{MessageId: int64(item.ID), MediaGroupId: groupId, Purpose: purpose}
}
