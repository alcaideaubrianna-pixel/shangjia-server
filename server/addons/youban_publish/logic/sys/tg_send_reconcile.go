package sys

import (
	"context"
	"crypto/sha256"
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
	message := strings.ToLower(err.Error())
	for _, part := range []string{"context deadline exceeded", "client.timeout exceeded", "closed pipe", "broken pipe", "goaway", "cannot rewind body", "connection reset", "unexpected eof"} {
		if strings.Contains(message, part) {
			return true
		}
	}
	return false
}

func (s *sSysPublish) updateTelegramJobSendPhase(ctx context.Context, jobId int64, phase string) error {
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).Data(g.Map{"send_phase": phase, "updated_at": gtime.Now()}).Update()
	return err
}

func (s *sSysPublish) markTelegramJobUnknown(ctx context.Context, job telegramJobRecord, cause error) error {
	message := "Telegram 返回结果不确定，已暂停重发并等待频道消息对账：" + cause.Error()
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
		"status": "unknown", "dispatch_status": tgDispatchStatusIdle, "next_retry_at": gtime.Now().Add(telegramUnknownReconcileDelay),
		"error_message": message, "updated_at": gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "标记TG任务待对账失败")
	}
	s.appendTelegramJobLog(ctx, job, "publish", "unknown", message)
	return nil
}

func (s *sSysPublish) reconcileUnknownTelegramJob(ctx context.Context, job telegramJobRecord) error {
	purpose := "display"
	if job.SendPhase == telegramSendPhaseVerifySending {
		purpose = "verify"
	}
	messages, err := s.findTelegramJobPhaseMessages(ctx, job, purpose)
	if err != nil {
		return s.postponeUnknownTelegramJob(ctx, job, err)
	}
	if len(messages) == 0 {
		return s.postponeUnknownTelegramJob(ctx, job, nil)
	}
	if err = s.saveTelegramSentMessages(ctx, job, messages); err != nil {
		return err
	}
	if purpose == "verify" {
		_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("status", "unknown").Data(g.Map{
			"status": "sending", "send_phase": telegramSendPhaseVerifyConfirmed, "dispatch_status": tgDispatchStatusProcessing, "error_message": "", "updated_at": gtime.Now(),
		}).Update()
		if err != nil {
			return err
		}
		return s.completeTelegramJob(ctx, job)
	}
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("status", "unknown").Data(g.Map{
		"status": "pending", "send_phase": telegramSendPhaseDisplayConfirmed, "dispatch_status": tgDispatchStatusIdle,
		"next_retry_at": nil, "error_message": "", "reconcile_count": 0, "updated_at": gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) postponeUnknownTelegramJob(ctx context.Context, job telegramJobRecord, cause error) error {
	count := job.ReconcileCount + 1
	if count < telegramUnknownReconcileMaxCount {
		_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("status", "unknown").Data(g.Map{
			"reconcile_count": count, "dispatch_status": tgDispatchStatusIdle, "next_retry_at": gtime.Now().Add(telegramUnknownReconcileDelay), "updated_at": gtime.Now(),
		}).Update()
		return err
	}
	message := "频道历史连续两次未发现对应消息，恢复正常重试"
	if cause != nil {
		message = "频道消息对账失败，恢复正常重试：" + cause.Error()
	}
	_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Where("status", "unknown").Data(g.Map{
		"status": "failed_retry", "dispatch_status": tgDispatchStatusIdle, "retry_count": job.RetryCount + 1,
		"next_retry_at": gtime.Now().Add(telegramRecoverableRetryDelay(cause, job.RetryCount+1)), "error_message": message,
		"reconcile_count": count, "updated_at": gtime.Now(),
	}).Update()
	return err
}

func (s *sSysPublish) findTelegramJobPhaseMessages(ctx context.Context, job telegramJobRecord, purpose string) ([]*telegramSentMessage, error) {
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
		res, requestErr := client.API().MessagesGetHistory(runCtx, &tg.MessagesGetHistoryRequest{Peer: &tg.InputPeerChannel{ChannelID: channelId, AccessHash: accessHash}, Limit: 60})
		if requestErr != nil {
			return requestErr
		}
		items := tgHistoryMessages(res)
		var groupedId int64
		for _, item := range items {
			if item != nil && strings.Contains(item.Message, marker) {
				groupedId = item.GroupedID
				found = append(found, &telegramSentMessage{MessageId: int64(item.ID), MediaGroupId: strconv.FormatInt(item.GroupedID, 10), Purpose: purpose})
				break
			}
		}
		if groupedId > 0 {
			for _, item := range items {
				if item != nil && item.GroupedID == groupedId && !strings.Contains(item.Message, marker) {
					found = append(found, &telegramSentMessage{MessageId: int64(item.ID), MediaGroupId: strconv.FormatInt(groupedId, 10), Purpose: purpose})
				}
			}
		}
		return nil
	})
	return found, err
}
