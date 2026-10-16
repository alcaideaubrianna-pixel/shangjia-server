package sys

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_publish/model/input/sysin"
)

const (
	telegramBotMediaSendTimeout = 10 * time.Minute
	telegramDisplaySendTimeout  = telegramBotMediaSendTimeout
	telegramVerifySendTimeout   = telegramBotMediaSendTimeout
)

var (
	errTelegramJobSuperseded              = errors.New("TG推送任务已废弃")
	errTelegramDeliveryUncertain          = errors.New("Telegram发送结果待对账")
	errTelegramMediaFallbackQueued        = errors.New("Telegram媒体降级任务已提交")
	errTelegramMediaFallbackSubmitDelayed = errors.New("Telegram媒体降级任务等待重新提交")
	errTelegramMediaFallbackUnavailable   = errors.New("Telegram媒体降级不可用")
	errTelegramRemoteConfirmed            = errors.New("Telegram已发送成功，等待修复本地投影")
)

func (s *sSysPublish) SendTelegramJob(ctx context.Context, jobId int64) (resultErr error) {
	ctx = telegramPublishTraceContext(ctx, jobId)
	started := time.Now()
	defer func() {
		if recovered := recover(); recovered != nil {
			logTelegramPublishSummary(ctx, started, fmt.Errorf("panic: %v", recovered))
			panic(recovered)
		}
		logTelegramPublishSummary(ctx, started, resultErr)
	}()
	stage := beginTelegramPublishStage(ctx, "job.execute")
	defer stage.Finish(&resultErr)
	step := beginTelegramPublishStage(ctx, "job.load")
	targetJob, err := s.telegramJobById(ctx, jobId)
	step.End(err)
	if err != nil {
		return err
	}
	ctx = telegramPublishJobTraceContext(ctx, targetJob)
	if isMessagePushOperationNo(targetJob.OperationNo) {
		return s.SendMessagePushJob(ctx, jobId)
	}
	step = beginTelegramPublishStage(ctx, "job.media_ready")
	ready, readyErr := s.profileMediaReady(ctx, targetJob.ProfileId)
	step.End(readyErr, g.Map{"ready": ready})
	if readyErr != nil {
		return readyErr
	} else if !ready {
		beginTelegramPublishStage(ctx, "job.deferred").End(nil, g.Map{"reason": "media_not_ready"})
		return s.postponeTelegramJobUntilMediaReady(ctx, targetJob)
	}
	if targetJob.CollectSourceId > 0 && !s.collectPushEnabled(ctx) {
		beginTelegramPublishStage(ctx, "job.deferred").End(nil, g.Map{"reason": "collect_push_paused"})
		return s.postponeTelegramJobForCollectPushPause(ctx, targetJob)
	}
	// An uncertain Telegram result belongs to the reconciliation lane. It must
	// not acquire the normal channel lease or delay the next publish operation;
	// the attempt timeout/reconcile worker is the only owner allowed to settle
	// this job.
	if targetJob.Status == telegramJobStatusConfirming {
		beginTelegramPublishStage(ctx, "job.reconciliation_lane").End(nil)
		if waiting, waitErr := s.telegramJobHasWaitingAttempt(ctx, targetJob.Id); waitErr != nil {
			return waitErr
		} else if waiting {
			return nil
		}
		return s.reconcileUnknownTelegramJob(ctx, targetJob)
	}
	step = beginTelegramPublishStage(ctx, "channel.send_lease.acquire")
	lease, ok, err := s.tryTelegramPublishChannelLease(ctx, targetJob)
	step.End(err, g.Map{"acquired": ok, "leaseTtlMs": telegramChannelLeaseTTL.Milliseconds()})
	if err != nil {
		return err
	}
	if !ok {
		beginTelegramPublishStage(ctx, "job.deferred").End(nil, g.Map{"reason": "channel_lease_busy", "alreadySending": targetJob.Status == "sending"})
		if targetJob.Status == "sending" {
			return nil
		}
		delay := s.telegramChannelBusyDelay(ctx, jobId, targetJob.DispatchCount)
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", jobId).
			WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
			Data(g.Map{
				"dispatch_status":     tgDispatchStatusIdle,
				"next_retry_at":       gtime.Now().Add(delay),
				"last_dispatch_error": "频道正在发送其他任务，已等待重新调度",
				"updated_at":          gtime.Now(),
			}).Update()
		// The lease holder wakes the highest-priority job when it finishes.
		// Re-enqueuing here creates a hot dispatch loop while the same channel is
		// still busy and can starve foreground publishes behind bulk work. The
		// periodic recovery loop remains the fallback if the lease holder exits.
		return nil
	}
	defer func() {
		releaseStage := beginTelegramPublishStage(ctx, "channel.send_lease.release")
		s.releaseTelegramChannelLease(ctx, lease)
		releaseStage.End(nil)
		wakeStage := beginTelegramPublishStage(ctx, "channel.wake_next")
		wakeErr := s.wakeNextTelegramChannelJob(ctx, targetJob)
		wakeStage.End(wakeErr)
		if wakeErr != nil {
			g.Log().Warningf(ctx, "释放频道发送槽位后唤醒下一条TG任务失败 jobId:%d channelId:%d err:%+v", targetJob.Id, targetJob.ChannelId, wakeErr)
		}
	}()
	// Recheck under the send lease: another worker may have opened the circuit
	// between enqueueing this job and acquiring the channel.
	if paused, pauseErr := s.postponePermissionPausedJob(ctx, targetJob); pauseErr != nil || paused {
		return pauseErr
	}
	step = beginTelegramPublishStage(ctx, "channel.publish_interval")
	delay, delayErr := s.telegramPublishIntervalDelay(ctx, targetJob)
	step.End(delayErr, g.Map{"delayMs": delay.Milliseconds()})
	if delayErr != nil {
		g.Log().Warningf(ctx, "检查频道发送间隔失败，本次降级继续发送 jobId:%d channelId:%d err:%+v", targetJob.Id, targetJob.ChannelId, delayErr)
	} else if delay > 0 {
		return s.postponeTelegramJobForPublishInterval(ctx, targetJob, delay)
	}
	step = beginTelegramPublishStage(ctx, "job.reload_before_send")
	targetJob, err = s.telegramJobById(ctx, jobId)
	step.End(err)
	if err != nil {
		return err
	}
	if isDownTelegramOperationNo(targetJob.OperationNo) {
		return s.sendQueuedDownChannelTelegramJob(ctx, jobId)
	}
	step = beginTelegramPublishStage(ctx, "channel.order_check")
	waitingOrder, err := s.telegramChannelHasEarlierActiveJob(ctx, targetJob)
	step.End(err, g.Map{"hasEarlier": waitingOrder})
	if err != nil {
		return err
	}
	if waitingOrder {
		beginTelegramPublishStage(ctx, "job.deferred").End(nil, g.Map{"reason": "earlier_channel_job"})
		return s.postponeTelegramJobForChannelOrder(ctx, targetJob)
	}
	return s.sendTelegramJobLockedByChannel(ctx, jobId)
}

func (s *sSysPublish) telegramChannelBusyDelay(ctx context.Context, jobId int64, dispatchCounts ...int) time.Duration {
	base := g.Cfg().MustGet(ctx, "youbanPublish.queue.channelBusyDelaySeconds", 3).Int()
	if base <= 0 {
		base = 3
	}
	dispatchCount := 0
	if len(dispatchCounts) > 0 && dispatchCounts[0] > 0 {
		dispatchCount = dispatchCounts[0]
	}
	return telegramChannelBusyDelayDuration(base, jobId, dispatchCount)
}

func telegramChannelBusyDelayDuration(base int, jobId int64, dispatchCount int) time.Duration {
	if base <= 0 {
		base = 3
	}
	if dispatchCount < 0 {
		dispatchCount = 0
	}
	level := dispatchCount / 5
	if level > 4 {
		level = 4
	}
	backoff := 0
	if level > 0 {
		backoff = 1 << level
	}
	jitter := int(jobId % 3)
	delay := base + backoff + jitter
	if delay > 30 {
		delay = 30
	}
	return time.Duration(delay) * time.Second
}

func (s *sSysPublish) sendTelegramJobLockedByChannel(ctx context.Context, jobId int64) (resultErr error) {
	stage := beginTelegramPublishStage(ctx, "job.locked_send")
	defer stage.Finish(&resultErr)
	step := beginTelegramPublishStage(ctx, "job.claim")
	job, locked, err := s.lockTelegramJob(ctx, jobId)
	step.End(err, g.Map{"claimed": locked})
	if err != nil || !locked {
		return err
	}
	ctx = telegramPublishJobTraceContext(ctx, job)
	bounded, cancelBudget, budgetErr := s.telegramSendBudgetContext(ctx, job)
	if budgetErr != nil {
		return budgetErr
	}
	defer cancelBudget()
	ctx = bounded
	step = beginTelegramPublishStage(ctx, "job.can_send")
	allowed, err := s.canSendTelegramJob(ctx, job)
	step.End(err, g.Map{"allowed": allowed})
	if err != nil {
		return err
	}
	if !allowed {
		s.appendTelegramJobLog(ctx, job, "publish", "skipped", "上架任务已下架或不可发布，跳过TG推送")
		return s.supersedeTelegramJobAndCompleteOperation(ctx, job)
	}
	if !telegramSendPhaseHasCleanup(job.SendPhase) {
		if err = s.updateTelegramJobSendPhase(ctx, job.Id, telegramSendPhaseCleanupProcessing); err != nil {
			return s.handleTelegramJobError(ctx, job, err)
		}
		job.SendPhase = telegramSendPhaseCleanupProcessing
		step = beginTelegramPublishStage(ctx, "job.cleanup_previous_publish")
		allowed, err = s.prepareProfileChannelPublish(ctx, job)
		step.End(err, g.Map{"allowed": allowed})
		if err != nil {
			return s.handleTelegramJobError(ctx, job, err)
		}
		if !allowed {
			s.appendTelegramJobLog(ctx, job, "publish", "skipped", "频道已有同资料有效消息，跳过重复TG推送")
			return s.supersedeTelegramJobAndCompleteOperation(ctx, job)
		}
		if err = s.updateTelegramJobSendPhase(ctx, job.Id, telegramSendPhaseCleanupConfirmed); err != nil {
			return s.handleTelegramJobError(ctx, job, err)
		}
		job.SendPhase = telegramSendPhaseCleanupConfirmed
	}
	step = beginTelegramPublishStage(ctx, "job.project_publishing_state")
	err = s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusPublishing)
	step.End(err)
	if err != nil {
		return err
	}
	step = beginTelegramPublishStage(ctx, "job.upsert_sending_record")
	recordErr := s.upsertPublishJobRecord(ctx, job, "sending", "")
	step.End(recordErr)
	if recordErr != nil {
		g.Log().Warningf(ctx, "更新发布发送中记录失败 jobId:%d err:%+v", job.Id, recordErr)
	}
	step = beginTelegramPublishStage(ctx, "job.append_started_log")
	s.appendTelegramJobLog(ctx, job, "publish", "started", s.telegramJobPublishMessage(ctx, job, "开始推送TG资料"))
	step.End(nil)
	if err = s.sendLockedTelegramJob(ctx, job); err != nil {
		if errors.Is(err, errTelegramMediaFallbackQueued) {
			s.appendTelegramJobLog(ctx, job, "account_fallback", "waiting", "协议号媒体降级任务已排队，TG任务保持发送中并等待账号服务完成")
			return nil
		}
		return s.handleTelegramJobError(ctx, job, err)
	}
	step = beginTelegramPublishStage(ctx, "job.complete")
	err = s.completeTelegramJob(ctx, job)
	step.End(err)
	if err != nil {
		return s.handleTelegramJobError(ctx, job, gerror.Wrap(err, "完成TG推送任务失败"))
	}
	return nil
}

func (s *sSysPublish) sendLockedTelegramJob(ctx context.Context, job telegramJobRecord) (resultErr error) {
	ctx = telegramPublishJobTraceContext(ctx, job)
	stage := beginTelegramPublishStage(ctx, "send.execute")
	defer stage.Finish(&resultErr)
	job.TargetChatId = normalizeTelegramChannelChatID(job.TargetChatId)
	if strings.TrimSpace(job.TargetChatId) == "" {
		return gerror.New("TG目标频道未配置")
	}
	step := beginTelegramPublishStage(ctx, "prepare.bot_token")
	botToken, err := s.telegramJobBotToken(ctx, job.BotId, job.TenantId)
	step.End(err)
	if err != nil {
		return err
	}
	step = beginTelegramPublishStage(ctx, "prepare.caption")
	caption, err := s.telegramJobCaption(ctx, job)
	step.End(err, g.Map{"captionBytes": len(caption)})
	if err != nil {
		return err
	}
	step = beginTelegramPublishStage(ctx, "prepare.display_media")
	displayMedia, err := s.telegramJobMedia(ctx, job, "display")
	step.End(err, g.Map{"mediaCount": len(displayMedia)})
	if err != nil {
		return err
	}
	step = beginTelegramPublishStage(ctx, "prepare.media_selection")
	displayMedia, err = s.selectTelegramDisplayMediaForTenant(ctx, job, displayMedia)
	step.End(err, g.Map{"selectedCount": len(displayMedia)})
	if err != nil {
		return err
	}
	step = beginTelegramPublishStage(ctx, "prepare.verify_media")
	verifyMedia, err := s.telegramJobMedia(ctx, job, "verify")
	step.End(err, g.Map{"mediaCount": len(verifyMedia)})
	if err != nil {
		return err
	}
	// Legacy jobs used verify_confirmed for both verified and display-only
	// publishes. Keep verified jobs idempotent, while allowing display-only
	// jobs to migrate to the truthful completed_without_verify phase below.
	if job.SendPhase == telegramSendPhaseVerifyConfirmed && len(verifyMedia) > 0 {
		return nil
	}
	step = beginTelegramPublishStage(ctx, "prepare.bot_client")
	bot, err := s.telegramMediaBot(ctx, botToken)
	step.End(err)
	if err != nil {
		return err
	}
	step = beginTelegramPublishStage(ctx, "prepare.content_protection")
	caption, err = s.applyTelegramJobContentProtection(ctx, job, caption, displayMedia, verifyMedia)
	step.End(err)
	if err != nil {
		return err
	}
	step = beginTelegramPublishStage(ctx, "display.check_job_state")
	stillSending, err := s.telegramJobStillSending(ctx, job.Id)
	step.End(err, g.Map{"stillSending": stillSending})
	if err != nil {
		return err
	} else if !stillSending {
		s.appendTelegramJobLog(ctx, job, "publish", "superseded", "TG展示资料发送前任务已废弃，停止推送")
		return errTelegramJobSuperseded
	}
	if telegramSendPhaseHasDisplay(job.SendPhase) && len(verifyMedia) > 0 {
		displayConfirmed, evidenceErr := s.telegramDisplayDeliveryConfirmed(ctx, job.Id, telegramDisplayExpectedCount(len(displayMedia)))
		if evidenceErr != nil {
			return evidenceErr
		}
		if !displayConfirmed {
			return gerror.New("TG任务缺少完整展示发送证据，禁止孤立发送验证资料")
		}
	}
	messages := make([]*telegramSentMessage, 0)
	if !telegramSendPhaseHasDisplay(job.SendPhase) {
		restored, restoreErr := s.restoreConfirmedTelegramResponse(ctx, job, "display", displayMedia)
		if restoreErr != nil {
			return gerror.Wrap(errTelegramRemoteConfirmed, restoreErr.Error())
		}
		if restored {
			job.SendPhase = telegramSendPhaseDisplayConfirmed
		} else {
			expectedCount := telegramDisplayExpectedCount(len(displayMedia))
			step = beginTelegramPublishStage(ctx, "display.attempt_create", g.Map{"expectedCount": expectedCount})
			attempt, attemptErr := s.prepareTelegramDeliveryAttempt(ctx, job, "display", expectedCount, true)
			step.End(attemptErr, g.Map{"attemptId": attempt.Id})
			if attemptErr != nil {
				return attemptErr
			}
			displayCaption := telegramCaptionWithJobMarker(caption, job.Id, "display") + telegramAttemptMarker(attempt.AttemptToken)
			g.Log().Infof(ctx, "TG展示资料开始Bot发送 jobId:%d botId:%d chat:%s media:%s", job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(displayMedia))
			stopAttemptHeartbeat := s.startTelegramAttemptHeartbeat(attempt.Id)
			displaySendCtx, cancelDisplaySend := context.WithTimeout(ctx, telegramDisplaySendTimeout)
			displaySendStartedAt := time.Now()
			step = beginTelegramPublishStage(ctx, "display.api_send", g.Map{"attemptId": attempt.Id, "mediaCount": len(displayMedia)})
			messages, err = s.sendTelegramDisplayPart(displaySendCtx, bot, job.TargetChatId, displayCaption, displayMedia)
			step.End(err, g.Map{"messageIds": telegramSentMessageIDs(messages), "messageCount": len(messages)})
			cancelDisplaySend()
			displaySendDuration := time.Since(displaySendStartedAt)
			observeTelegramMediaSend(ctx, "display", err, displaySendDuration)
			g.Log().Infof(ctx, "TG展示资料Bot发送结束 jobId:%d profileId:%d channelId:%d botId:%d chat:%s duration:%s messages:%d messageIds:%v err:%v", job.Id, job.ProfileId, job.ChannelId, job.BotId, job.TargetChatId, displaySendDuration.Round(time.Millisecond), len(messages), telegramSentMessageIDs(messages), err)
			if err != nil && len(messages) == 0 && shouldFallbackTelegramMediaToAccount(err) {
				g.Log().Warningf(ctx, "Bot展示媒体发送失败，命中媒体降级条件 jobId:%d botId:%d chat:%s media:%s err:%+v", job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(displayMedia), err)
				messages, err = s.sendTelegramJobMediaByAccount(ctx, job, "display", displayCaption, displayMedia, err)
			}
			stopAttemptHeartbeat()
			if err != nil {
				if errors.Is(err, errTelegramMediaFallbackQueued) {
					return err
				}
				if !isTelegramMediaSizeLimitError(err) {
					logTelegramBotMediaSendFailure(ctx, job, "展示", displayMedia, err)
				}
				if isTelegramAmbiguousDeliveryError(err) {
					if len(messages) > 0 {
						_ = s.saveTelegramSentMessages(ctx, job, messages)
					}
					_ = s.recordTelegramAttemptMessages(ctx, attempt, messages, "response")
					if waitErr := s.waitTelegramDeliveryWebhook(ctx, attempt, err); waitErr != nil {
						return waitErr
					}
					return errTelegramAwaitingWebhook
				}
				s.abandonTelegramDeliveryAttempt(ctx, attempt, messages, err)
				_ = s.cleanupTelegramSentMessages(ctx, bot, job.TargetChatId, messages, "展示资料分片推送失败")
				return gerror.Wrapf(err, "TG展示资料推送失败，job:%d，channel:%d，chat:%s", job.Id, job.ChannelId, job.TargetChatId)
			}
			if err = validateTelegramMediaSendResult("display", displayMedia, messages); err != nil {
				_ = s.cleanupTelegramSentMessages(ctx, bot, job.TargetChatId, messages, "展示媒体发送结果不完整")
				return err
			}
			// A successful Bot API response with message IDs is authoritative. Commit
			// its evidence and projections together; only a failed local commit enters
			// reconciliation, and the external send is never retried directly.
			step = beginTelegramPublishStage(ctx, "display.confirm_api_success", g.Map{"attemptId": attempt.Id, "messageIds": telegramSentMessageIDs(messages)})
			err = s.confirmTelegramAPISuccess(ctx, job, attempt, messages)
			step.End(err, g.Map{"apiConfirmed": true, "localCommitted": err == nil, "waitForWebhook": false})
			if err != nil {
				return gerror.Wrap(errTelegramRemoteConfirmed, err.Error())
			}
			step = beginTelegramPublishStage(ctx, "display.cache_file_ids")
			err = s.updateTelegramMediaFileIds(ctx, messages)
			step.End(err)
			if err != nil {
				g.Log().Warningf(ctx, "更新TG展示媒体file_id失败 job:%d err:%+v", job.Id, err)
			}
			job.SendPhase = telegramSendPhaseDisplayConfirmed
		}
	}
	if stillSending, err := s.telegramJobStillSending(ctx, job.Id); err != nil {
		return err
	} else if !stillSending {
		s.appendTelegramJobLog(ctx, job, "publish", "superseded", "TG展示资料已发送但任务已废弃，停止推送验证资料")
		return errTelegramJobSuperseded
	}
	if len(verifyMedia) == 0 {
		g.Log().Infof(ctx, "TG任务无验证资料，跳过验证发送 jobId:%d channelId:%d", job.Id, job.ChannelId)
		return s.finalizeTelegramMediaDelivery(ctx, job, false, false)
	}
	step = beginTelegramPublishStage(ctx, "verify.restore_response")
	restored, restoreErr := s.restoreConfirmedTelegramResponse(ctx, job, "verify", verifyMedia)
	step.End(restoreErr, g.Map{"restored": restored})
	if restoreErr != nil {
		return gerror.Wrap(errTelegramRemoteConfirmed, restoreErr.Error())
	} else if restored {
		return nil
	}
	// Always give the Bot API the first chance to upload videos without a
	// reusable file_id. The account fallback requires channel-admin access and
	// is only safe after the Bot send returns a concrete failure.
	if telegramVerifyMediaNeedsAccountUpload(verifyMedia) {
		g.Log().Infof(ctx, "TG验证资料无可复用file_id，先尝试Bot本地上传 jobId:%d channelId:%d media:%s", job.Id, job.ChannelId, telegramMediaDebugSummary(verifyMedia))
	}
	step = beginTelegramPublishStage(ctx, "verify.attempt_create", g.Map{"expectedCount": len(verifyMedia)})
	verifyAttempt, err := s.prepareTelegramDeliveryAttempt(ctx, job, "verify", len(verifyMedia), true)
	step.End(err, g.Map{"attemptId": verifyAttempt.Id})
	if err != nil {
		return err
	}
	// 标记必须附着于验证媒体；采集分类前移除内部标记。
	verifyCaption := telegramAttemptMarker(verifyAttempt.AttemptToken)
	g.Log().Infof(ctx, "TG验证资料开始Bot发送 jobId:%d botId:%d chat:%s media:%s", job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(verifyMedia))
	stopAttemptHeartbeat := s.startTelegramAttemptHeartbeat(verifyAttempt.Id)
	verifySendCtx, cancelVerifySend := context.WithTimeout(ctx, telegramVerifySendTimeout)
	verifySendStartedAt := time.Now()
	step = beginTelegramPublishStage(ctx, "verify.api_send", g.Map{"attemptId": verifyAttempt.Id, "mediaCount": len(verifyMedia)})
	verifyMessages, err := s.sendTelegramVerifyPart(verifySendCtx, bot, job.TargetChatId, verifyCaption, verifyMedia)
	step.End(err, g.Map{"messageIds": telegramSentMessageIDs(verifyMessages), "messageCount": len(verifyMessages)})
	cancelVerifySend()
	verifySendDuration := time.Since(verifySendStartedAt)
	observeTelegramMediaSend(ctx, "verify", err, verifySendDuration)
	g.Log().Infof(ctx, "TG验证资料Bot发送结束 jobId:%d profileId:%d channelId:%d botId:%d chat:%s duration:%s messages:%d messageIds:%v err:%v", job.Id, job.ProfileId, job.ChannelId, job.BotId, job.TargetChatId, verifySendDuration.Round(time.Millisecond), len(verifyMessages), telegramSentMessageIDs(verifyMessages), err)
	if err != nil && len(verifyMessages) == 0 && shouldFallbackTelegramMediaToAccount(err) {
		g.Log().Warningf(ctx, "Bot验证媒体发送失败，命中媒体降级条件 jobId:%d botId:%d chat:%s media:%s err:%+v", job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(verifyMedia), err)
		verifyMessages, err = s.sendTelegramJobMediaByAccount(ctx, job, "verify", verifyCaption, verifyMedia, err)
	}
	stopAttemptHeartbeat()
	if err != nil {
		if errors.Is(err, errTelegramMediaFallbackQueued) {
			return err
		}
		if !isTelegramMediaSizeLimitError(err) {
			logTelegramBotMediaSendFailure(ctx, job, "验证", verifyMedia, err)
		}
		if isTelegramAmbiguousDeliveryError(err) {
			if len(verifyMessages) > 0 {
				_ = s.saveTelegramSentMessages(ctx, job, verifyMessages)
			}
			_ = s.recordTelegramAttemptMessages(ctx, verifyAttempt, verifyMessages, "response")
			if waitErr := s.waitTelegramDeliveryWebhook(ctx, verifyAttempt, err); waitErr != nil {
				return waitErr
			}
			return errTelegramAwaitingWebhook
		}
		s.abandonTelegramDeliveryAttempt(ctx, verifyAttempt, verifyMessages, err)
		if !isTelegramAmbiguousDeliveryError(err) {
			if len(verifyMessages) > 0 {
				if saveErr := s.saveTelegramSentMessages(ctx, job, verifyMessages); saveErr != nil {
					return telegramDeliveryUncertainError(saveErr)
				}
			}
			if cleanupErr := s.deleteTelegramMessagePurposeSetLockedByChannel(ctx, job, "verify", "验证资料推送失败，仅清理本次验证消息"); cleanupErr != nil {
				return cleanupErr
			}
			if phaseErr := s.updateTelegramJobSendPhase(ctx, job.Id, telegramSendPhaseDisplayConfirmed); phaseErr != nil {
				return phaseErr
			}
		}
		return gerror.Wrapf(err, "TG验证资料推送失败，job:%d，channel:%d，chat:%s", job.Id, job.ChannelId, job.TargetChatId)
	}
	if stillSending, stateErr := s.telegramJobStillSending(ctx, job.Id); stateErr != nil {
		return stateErr
	} else if !stillSending {
		deletedIds := s.cleanupTelegramSentMessages(ctx, bot, job.TargetChatId, verifyMessages, "验证资料Bot响应迟到，任务已由其他发送链路完成")
		observeTelegramLateDelivery(ctx, "verify", "cleanup", len(deletedIds))
		g.Log().Warningf(ctx, "TG验证资料Bot响应迟到，已自动清理 jobId:%d profileId:%d channelId:%d botId:%d chat:%s returnedMessageIds:%v deletedMessageIds:%v", job.Id, job.ProfileId, job.ChannelId, job.BotId, job.TargetChatId, telegramSentMessageIDs(verifyMessages), deletedIds)
		return errTelegramJobSuperseded
	}
	if err = validateTelegramMediaSendResult("verify", verifyMedia, verifyMessages); err != nil {
		_ = s.cleanupTelegramSentMessages(ctx, bot, job.TargetChatId, verifyMessages, "验证媒体发送结果不完整")
		return err
	}
	step = beginTelegramPublishStage(ctx, "verify.confirm_api_success", g.Map{"attemptId": verifyAttempt.Id, "messageIds": telegramSentMessageIDs(verifyMessages)})
	err = s.confirmTelegramAPISuccess(ctx, job, verifyAttempt, verifyMessages)
	step.End(err, g.Map{"apiConfirmed": true, "localCommitted": err == nil, "waitForWebhook": false})
	if err != nil {
		return gerror.Wrap(errTelegramRemoteConfirmed, err.Error())
	}
	step = beginTelegramPublishStage(ctx, "verify.cache_file_ids")
	err = s.updateTelegramMediaFileIds(ctx, verifyMessages)
	step.End(err)
	return err
}

func telegramVerifyMediaNeedsAccountUpload(media []*telegramMediaItem) bool {
	for _, item := range media {
		if item == nil || item.MediaType != "video" {
			continue
		}
		if !telegramMediaUsesReusableFileId(item) {
			return true
		}
	}
	return false
}

func telegramSentMessageIDs(messages []*telegramSentMessage) []int64 {
	ids := make([]int64, 0, len(messages))
	for _, message := range messages {
		if message != nil && message.MessageId > 0 {
			ids = append(ids, message.MessageId)
		}
	}
	return ids
}

func logTelegramBotMediaSendFailure(ctx context.Context, job telegramJobRecord, purpose string, media []*telegramMediaItem, err error) {
	if isTelegramAccountBusyError(err) || isTelegramNetworkRetryError(err) {
		g.Log().Warningf(ctx, "Bot%s媒体发送遇到可恢复错误，等待队列重试 jobId:%d botId:%d chat:%s media:%s err:%v", purpose, job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(media), err)
		return
	}
	g.Log().Errorf(ctx, "Bot%s媒体发送失败，未进入协议号降级 jobId:%d botId:%d chat:%s media:%s err:%+v", purpose, job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(media), err)
}

func isTelegramMediaSourceUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "下载远程媒体失败：http 404") ||
		strings.Contains(message, "下载远程媒体失败: http 404") ||
		strings.Contains(message, "下载远程媒体失败：http 403") ||
		strings.Contains(message, "下载远程媒体失败: http 403") ||
		strings.Contains(message, "下载远程媒体失败：http 451") ||
		strings.Contains(message, "下载远程媒体失败: http 451")
}

func isTelegramBotMediaProcessingError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToUpper(err.Error()), "IMAGE_PROCESS_FAILED")
}

func shouldFallbackTelegramMediaToAccount(err error) bool {
	// Timeout hands ownership to the account queue. Its existing Attempt must
	// be reconciled before the account sends; never start a parallel sender.
	if isTelegramAPITimeout(err) {
		return true
	}
	if isTelegramAmbiguousDeliveryError(err) {
		return false
	}
	return isTelegramMediaSizeLimitError(err) ||
		isTelegramMediaSourceUnavailableError(err) ||
		isTelegramBotMediaProcessingError(err)
}

func isTelegramAPITimeout(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var timeout net.Error
	return errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout())
}

func telegramMediaDebugSummary(media []*telegramMediaItem) string {
	parts := make([]string, 0, len(media))
	for _, item := range media {
		if item == nil {
			continue
		}
		sizeBytes := int64(0)
		if path := strings.TrimSpace(item.StoragePath); path != "" {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				sizeBytes = info.Size()
			}
		}
		fileMode := "upload"
		if strings.TrimSpace(item.TgFileId) != "" && !item.ForceUpload && !item.AntiScanEnabled {
			fileMode = "reuse"
		}
		parts = append(parts, fmt.Sprintf("id=%d,type=%s,purpose=%s,sizeBytes=%d,mode=%s,hasFileId=%t,antiScan=%t", item.Id, item.MediaType, item.Purpose, sizeBytes, fileMode, strings.TrimSpace(item.TgFileId) != "", item.AntiScanEnabled))
	}
	return strings.Join(parts, ";")
}

func (s *sSysPublish) cleanupTelegramSentMessages(ctx context.Context, bot *tgbot.Bot, chatId string, messages []*telegramSentMessage, reason string) []int64 {
	deletedIds := make([]int64, 0, len(messages))
	if bot == nil || len(messages) == 0 {
		return deletedIds
	}
	chatId = normalizeTelegramChannelChatID(chatId)
	for _, item := range messages {
		if item == nil || item.MessageId <= 0 {
			continue
		}
		if _, err := bot.DeleteMessage(ctx, &tgbot.DeleteMessageParams{ChatID: chatId, MessageID: int(item.MessageId)}); err != nil {
			if isTelegramMessageAlreadyDeletedError(err) {
				deletedIds = append(deletedIds, item.MessageId)
				g.Log().Infof(ctx, "清理TG半组消息跳过，消息已不存在 chat:%s message:%d reason:%s", chatId, item.MessageId, reason)
				continue
			}
			g.Log().Warningf(ctx, "清理TG半组消息失败 chat:%s message:%d reason:%s err:%+v", chatId, item.MessageId, reason, err)
			continue
		}
		deletedIds = append(deletedIds, item.MessageId)
	}
	return deletedIds
}

func (s *sSysPublish) sendTelegramDisplayPart(ctx context.Context, bot *tgbot.Bot, chatId string, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	if len(media) > 0 {
		return s.sendTelegramMediaSet(ctx, bot, chatId, "display", caption, media)
	}
	if strings.TrimSpace(caption) == "" {
		return nil, gerror.New("展示资料和推送文案不能同时为空")
	}
	msg, err := bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatId, Text: caption, ParseMode: models.ParseModeHTML})
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, nil
	}
	return []*telegramSentMessage{{MessageId: int64(msg.ID), Purpose: "display"}}, nil
}

func (s *sSysPublish) sendTelegramVerifyPart(ctx context.Context, bot *tgbot.Bot, chatId string, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	if len(media) == 0 {
		return nil, nil
	}
	return s.sendTelegramMediaSet(ctx, bot, chatId, "verify", caption, telegramMediaSetWithoutCopyRefs(media))
}

func (s *sSysPublish) sendTelegramJobMediaByAccount(ctx context.Context, job telegramJobRecord, purpose string, caption string, media []*telegramMediaItem, botErr error) (resultMessages []*telegramSentMessage, resultErr error) {
	if isTelegramAPITimeout(botErr) {
		// Queue the handoff even when the HTTP request consumed its context.
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
	}
	ctx = telegramPublishJobTraceContext(ctx, job)
	stage := beginTelegramPublishStage(ctx, "account_fallback.submit", g.Map{"purpose": purpose, "mediaCount": len(media), "botError": telegramPublishLogError(botErr)})
	defer stage.Finish(&resultErr)
	channel, err := s.messagePushChannelFromJob(ctx, job)
	if err != nil {
		return nil, gerror.Wrap(err, "读取协议号降级发送频道失败")
	}
	if channel.TgAccountId <= 0 {
		return nil, gerror.Newf("Bot发送媒体超过限制，频道未绑定协议号，无法整组降级发送：%v", botErr)
	}
	g.Log().Warning(ctx, "Bot媒体发送触发协议号整组降级", g.Map{
		"jobId": job.Id, "channelId": job.ChannelId, "tgAccountId": channel.TgAccountId,
		"purpose": purpose, "mediaCount": len(media), "media": telegramMediaDebugSummary(media), "reason": botErr.Error(),
	})
	err = s.submitTelegramMediaFallback(ctx, job, channel.TgAccountId, purpose)
	if err != nil {
		if errors.Is(err, errTelegramMediaFallbackSubmitDelayed) {
			g.Log().Warningf(ctx, "协议号媒体降级任务暂未提交，等待自动恢复 jobId:%d purpose:%s err:%+v", job.Id, purpose, err)
			return nil, errTelegramMediaFallbackQueued
		}
		return nil, gerror.Wrap(err, "提交协议号媒体降级任务失败")
	}
	s.appendTelegramJobLog(ctx, job, "account_fallback", "queued", fmt.Sprintf("Bot媒体发送失败，已提交账号服务常驻客户端发送 purpose:%s reason:%v", purpose, botErr))
	return nil, errTelegramMediaFallbackQueued
}

const telegramMediaFallbackMarker = "awaiting_account_fallback:"

func telegramMediaFallbackReason(purpose string) string {
	return telegramMediaFallbackMarker + strings.TrimSpace(purpose)
}

func telegramMediaFallbackPurpose(reason string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(reason), telegramMediaFallbackMarker))
}

func (s *sSysPublish) submitTelegramMediaFallback(ctx context.Context, job telegramJobRecord, tgAccountID int64, purpose string) error {
	purpose = strings.TrimSpace(purpose)
	if job.Id <= 0 || tgAccountID <= 0 || (purpose != "display" && purpose != "verify") {
		return gerror.New("协议号媒体降级任务参数不完整")
	}
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).Where("status", telegramJobStatusSending).
		Data(g.Map{
			"last_dispatch_error": telegramMediaFallbackReason(purpose),
			"next_retry_at":       gtime.Now().Add(telegramMediaFallbackRecoverAfter),
			"reconcile_count":     0,
			"updated_at":          gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, "标记协议号媒体降级等待状态失败")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "确认协议号媒体降级等待状态失败")
	}
	if affected == 0 {
		return gerror.New("TG任务已不在发送状态，取消协议号媒体降级提交")
	}
	if _, err = collectorservice.AccountTasks().Submit(ctx, &collectorin.AccountTaskSubmit{
		TenantID: job.TenantId, AccountID: tgAccountID,
		TaskType: collectorin.AccountTaskTypeMessageMediaFallback,
		TaskKey:  fmt.Sprintf("message-media-fallback:%d:%s", job.Id, purpose),
		Priority: collectorin.EventPriorityUrgent, MaxAttempts: 5,
	}); err != nil {
		// Ownership already belongs to the account fallback. Keep the marker so
		// the recovery loop retries task creation without reopening Bot delivery.
		g.Log().Warningf(ctx, "提交协议号媒体降级任务失败，等待自动恢复 jobId:%d purpose:%s err:%+v", job.Id, purpose, err)
		return gerror.Wrap(errTelegramMediaFallbackSubmitDelayed, err.Error())
	}
	return nil
}

func (s *sSysPublish) handleTelegramJobError(ctx context.Context, job telegramJobRecord, err error) (resultErr error) {
	ctx = telegramPublishJobTraceContext(ctx, job)
	stage := beginTelegramPublishStage(ctx, "job.handle_error", g.Map{"cause": telegramPublishLogError(err)})
	defer stage.Finish(&resultErr)
	if errors.Is(err, errTelegramJobSuperseded) {
		beginTelegramPublishStage(ctx, "job.error_decision").End(nil, g.Map{"decision": "superseded"})
		return nil
	}
	if errors.Is(err, errTelegramAwaitingWebhook) {
		beginTelegramPublishStage(ctx, "job.error_decision").End(nil, g.Map{"decision": "await_ambiguous_delivery"})
		return nil
	}
	if errors.Is(err, errTelegramAttemptNeedsReconcile) {
		return s.markTelegramJobUnknown(ctx, job, err)
	}
	if errors.Is(err, errTelegramRemoteConfirmed) {
		beginTelegramPublishStage(ctx, "job.error_decision").End(nil, g.Map{"decision": "repair_local_projection", "apiConfirmed": true, "resend": false})
		// Telegram returned message IDs, but the atomic local confirmation did
		// not commit. Never retry the external call from this state: reconcile
		// the marker from channel history and only repair local projections.
		return s.markTelegramJobUnknown(ctx, job, err)
	}
	if !telegramSendPhaseIsCleanup(job.SendPhase) && !isTelegramAccountBusyError(err) && isTelegramAmbiguousDeliveryError(err) {
		return s.markTelegramJobUnknown(ctx, job, err)
	}
	allowed, allowedErr := s.canSendTelegramJob(ctx, job)
	if allowedErr == nil && !allowed {
		s.appendTelegramJobLog(ctx, job, "publish", "superseded", "TG推送失败时任务已被新操作覆盖，旧任务已废弃")
		return s.supersedeTelegramJobAndCompleteOperation(ctx, job)
	}
	var tooMany *tgbot.TooManyRequestsError
	if errors.As(err, &tooMany) {
		if switched, switchErr := s.switchTelegramJobToNextBot(ctx, job, err); switchErr != nil {
			return switchErr
		} else if switched {
			return nil
		}
	}
	if isTelegramNetworkRetryError(err) {
		s.clearTelegramBotCache()
	}
	conf, confErr := s.telegramPublishConfig(ctx, job.TenantId, job.AccountId)
	if confErr != nil {
		g.Log().Warningf(ctx, "读取TG重试策略失败，使用默认策略 jobId:%d err:%+v", job.Id, confErr)
		conf = defaultPublishConfig()
	}
	decision := telegramJobFailureNextStateWithConfig(err, job.RetryCount, conf)
	step := beginTelegramPublishStage(ctx, "job.failure_transition", g.Map{"nextStatus": decision.Status, "retryCount": decision.RetryCount, "retryDelayMs": decision.RetryDelay.Milliseconds()})
	transitioned, updateErr := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId:        job.Id,
		From:         []string{telegramJobStatusSending},
		To:           decision.Status,
		RetryDelay:   decision.RetryDelay,
		ErrorMessage: decision.Message,
		Additional: g.Map{
			"retry_count":         decision.RetryCount,
			"last_dispatch_error": decision.Message,
		},
	})
	step.End(updateErr, g.Map{"transitioned": transitioned})
	if updateErr != nil {
		return gerror.Wrap(updateErr, "更新TG失败任务状态失败")
	}
	if !transitioned {
		return nil
	}
	if decision.Status == "failed" {
		if circuitErr := s.recordChannelPermissionFailure(ctx, job, err); circuitErr != nil {
			g.Log().Warningf(ctx, "记录频道权限熔断失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, circuitErr)
		}
	}
	if recordErr := s.upsertPublishJobRecord(ctx, job, decision.Status, decision.Message); recordErr != nil {
		g.Log().Warningf(ctx, "更新发布失败记录失败 jobId:%d status:%s err:%+v", job.Id, decision.Status, recordErr)
	}
	s.appendTelegramJobLog(ctx, job, "publish", decision.Status, decision.Message)
	if decision.Status == "failed" && job.CollectEventId > 0 {
		_ = s.markCollectDispatchFailedByProfile(ctx, job.ProfileId, job.CollectEventId, decision.Message)
	}
	if decision.Status == "failed" {
		observeTelegramPublishFailure(ctx, "send")
		s.enqueueTerminalPublishRecoveryBestEffort(ctx, job, err)
	}
	projectedStatus := sysin.PublishTaskStatusPending
	if decision.Status == "failed" {
		projectedStatus = sysin.PublishTaskStatusFailed
	}
	if stateErr := s.updateProfilePublishOperationState(ctx, job, projectedStatus); stateErr != nil {
		return stateErr
	}
	return nil
}

func (s *sSysPublish) failTelegramMediaFallbackJob(ctx context.Context, job telegramJobRecord, cause error) error {
	message := "协议号媒体发送已达最大重试次数"
	if errors.Is(cause, errTelegramMediaFallbackUnavailable) {
		message = "协议号媒体降级不可用"
	}
	if cause != nil {
		message += "：" + cause.Error()
	}
	transitioned, err := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId:        job.Id,
		From:         []string{telegramJobStatusSending, telegramJobStatusConfirming},
		To:           telegramJobStatusFailed,
		ErrorMessage: message,
		Additional:   g.Map{"last_dispatch_error": message},
	})
	if err != nil {
		return gerror.Wrap(err, "终止协议号媒体失败任务失败")
	}
	if !transitioned {
		status, statusErr := s.telegramJobCurrentStatus(ctx, job.Id)
		if statusErr != nil {
			return statusErr
		}
		if status != telegramJobStatusFailed {
			return nil
		}
	}
	if transitioned {
		observeTelegramPublishFailure(ctx, "media_fallback")
		if !isTelegramMediaSourceUnavailableError(cause) && !errors.Is(cause, errTelegramMediaFallbackUnavailable) {
			s.enqueueTerminalPublishRecoveryBestEffort(ctx, job, cause)
		}
		if recordErr := s.upsertPublishJobRecord(ctx, job, "failed", message); recordErr != nil {
			g.Log().Warningf(ctx, "更新协议号媒体失败记录失败 jobId:%d err:%+v", job.Id, recordErr)
		}
		s.appendTelegramJobLog(ctx, job, "account_fallback", "failed", message)
	}
	if job.CollectEventId > 0 {
		_ = s.markCollectDispatchFailedByProfile(ctx, job.ProfileId, job.CollectEventId, message)
	}
	if err = s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed); err != nil {
		return err
	}
	if transitioned && isTelegramMediaSourceUnavailableError(cause) {
		queued, repairErr := s.enqueueProfileMediaRepairTask(ctx, job.ProfileId, job.Id)
		if repairErr != nil {
			g.Log().Warningf(ctx, "TG媒体源失效后提交Bot媒体恢复失败 jobId:%d profileId:%d err:%+v", job.Id, job.ProfileId, repairErr)
		} else if queued {
			s.appendTelegramJobLog(ctx, job, "media_repair", "queued", "Telegram临时媒体地址失效，已提交媒体持久化恢复并重新上架")
			g.Log().Infof(ctx, "TG媒体源失效后已提交Bot媒体恢复 jobId:%d profileId:%d", job.Id, job.ProfileId)
		}
	}
	return s.wakeNextTelegramChannelJob(ctx, job)
}

func (s *sSysPublish) switchTelegramJobToNextBot(ctx context.Context, job telegramJobRecord, cause error) (bool, error) {
	nextBotId, err := s.nextTelegramChannelBotId(ctx, job)
	if err != nil {
		return false, err
	}
	if nextBotId <= 0 || nextBotId == job.BotId {
		return false, nil
	}
	transitioned, err := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId: job.Id,
		From:  []string{telegramJobStatusSending},
		To:    telegramJobStatusPending,
		Additional: g.Map{
			"bot_id":        nextBotId,
			"retry_count":   job.RetryCount + 1,
			"error_message": "",
		},
	})
	if err != nil {
		return false, gerror.Wrap(err, "切换备用BOT失败")
	}
	if !transitioned {
		return false, nil
	}
	job.BotId = nextBotId
	if err = s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusPending); err != nil {
		return false, err
	}
	if recordErr := s.upsertPublishJobRecord(ctx, job, "pending", "已切换备用BOT，等待重新发送"); recordErr != nil {
		g.Log().Warningf(ctx, "更新发布待发送记录失败 jobId:%d err:%+v", job.Id, recordErr)
	}
	s.appendTelegramJobLog(ctx, job, "publish", "bot_switched", fmt.Sprintf("当前BOT限流，已切换备用BOT：%d，原因：%s", nextBotId, cause.Error()))
	return true, nil
}

func (s *sSysPublish) completeTelegramJob(ctx context.Context, job telegramJobRecord) error {
	return s.withProfileLifecycleLock(ctx, job.TenantId, job.ProfileId, func() error {
		return s.completeTelegramJobLockedByProfile(ctx, job)
	})
}

func (s *sSysPublish) completeTelegramJobLockedByProfile(ctx context.Context, job telegramJobRecord) error {
	if status, err := s.telegramJobCurrentStatus(ctx, job.Id); err != nil {
		return err
	} else if status != "sending" {
		s.appendTelegramJobLog(ctx, job, "publish", "skipped", "TG发送完成前任务状态已变更，跳过成功标记："+status)
		return nil
	}
	allowed, err := s.canSendTelegramJob(ctx, job)
	if err != nil {
		return err
	}
	if !allowed {
		s.appendTelegramJobLog(ctx, job, "publish", "skipped", "TG发送完成前任务已下架，开始清理已发送消息")
		if err = s.deleteTelegramMessageSetLockedByChannel(ctx, job, "任务已下架"); err != nil {
			return err
		}
		return s.supersedeTelegramJobAndCompleteOperation(ctx, job)
	}
	if err = s.validateTelegramJobCompletion(ctx, job.Id, job.ProfileId); err != nil {
		return err
	}
	sentAt := gtime.Now()
	transitioned, err := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId:      job.Id,
		From:       []string{telegramJobStatusSending},
		To:         telegramJobStatusSent,
		Additional: g.Map{"sent_at": sentAt},
	})
	if err != nil {
		return gerror.Wrap(err, "更新TG任务状态失败")
	}
	if !transitioned {
		s.appendTelegramJobLog(ctx, job, "publish", "skipped", "TG发送完成时任务已不再发送中，跳过成功标记")
		return nil
	}
	if stepErr := s.projectTelegramDeliveryStep(ctx, job.Id, telegramDeliveryStepFinalize, telegramDeliveryStepSucceeded, 0, "", sentAt); stepErr != nil {
		g.Log().Warningf(ctx, "投影TG完成步骤失败 jobId:%d err:%+v", job.Id, stepErr)
	}
	job.SentAt = sentAt
	if attemptErr := s.supersedeOpenTelegramAttempts(ctx, job.Id); attemptErr != nil {
		g.Log().Warningf(ctx, "关闭TG任务遗留Attempt失败 jobId:%d err:%+v", job.Id, attemptErr)
	}
	s.markTelegramLastSuccess(ctx, job, sentAt)
	s.appendTelegramJobLog(ctx, job, "publish", "sent", s.telegramJobPublishMessage(ctx, job, "TG资料推送成功"))
	if recordErr := s.appendPublishSuccessRecord(ctx, job); recordErr != nil {
		g.Log().Warningf(ctx, "保存成功发布记录失败 jobId:%d err:%+v", job.Id, recordErr)
	}
	if recoveryErr := s.reconcileRecoveredCycleJob(ctx, job); recoveryErr != nil {
		g.Log().Warningf(ctx, "回写循环批次自动补偿状态失败 jobId:%d err:%+v", job.Id, recoveryErr)
	}
	if indexErr := s.upsertChannelProfileFromJob(ctx, job); indexErr != nil {
		g.Log().Warningf(ctx, "更新频道资料索引失败 jobId:%d err:%+v", job.Id, indexErr)
	} else if cycleErr := s.syncTelegramJobCycleSchedule(ctx, job); cycleErr != nil {
		g.Log().Warningf(ctx, "更新资料循环计划失败 jobId:%d err:%+v", job.Id, cycleErr)
	}
	isCycle := isCycleBatchOperation(job.OperationNo)
	operationCompleted, completeErr := s.completeProfileTelegramOperation(ctx, job, isCycle)
	if completeErr != nil {
		return completeErr
	}
	if !operationCompleted {
		return nil
	}
	if job.CollectEventId > 0 {
		return s.markCollectDispatchSentByProfile(ctx, job.ProfileId, job.CollectEventId)
	}
	return nil
}

func (s *sSysPublish) validateTelegramJobCompletion(ctx context.Context, jobID, profileID int64) error {
	job, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("send_phase").Where("id", jobID).One()
	if err != nil {
		return gerror.Wrap(err, "读取TG任务完成阶段失败")
	}
	if job.IsEmpty() {
		return gerror.Newf("TG任务不存在 jobId:%d", jobID)
	}
	phase := strings.TrimSpace(job["send_phase"].String())
	verifyMediaCount, err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Where("profile_id", profileID).Where("purpose", "verify").WhereNull("deleted_at").Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG任务验证媒体失败")
	}
	verifyMessageCount, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Where("job_id", jobID).Where("purpose", "verify").Where("status", "sent").WhereNull("deleted_at").Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG任务验证消息失败")
	}
	if err = telegramJobCompletionError(phase, verifyMediaCount, verifyMessageCount); err != nil {
		return gerror.Wrapf(err, "TG任务完成条件不满足 jobId:%d profileId:%d", jobID, profileID)
	}
	return nil
}

func telegramJobCompletionError(phase string, verifyMediaCount, verifyMessageCount int) error {
	phase = strings.TrimSpace(phase)
	if verifyMediaCount > 0 {
		if phase != telegramSendPhaseVerifyConfirmed || verifyMessageCount <= 0 {
			return gerror.Newf("资料已有验证媒体但验证消息未确认 phase:%s verifyMessages:%d", phase, verifyMessageCount)
		}
		return nil
	}
	if phase != telegramSendPhaseCompletedNoVerify {
		return gerror.Newf("无验证资料任务完成阶段不合法 phase:%s", phase)
	}
	return nil
}

func (s *sSysPublish) telegramJobPublishMessage(ctx context.Context, job telegramJobRecord, message string) string {
	if isCycleBatchOperation(job.OperationNo) {
		message = "循环上架发布：" + message
	}
	policy, err := s.telegramChannelSendPolicy(ctx, job)
	if err != nil {
		return message
	}
	if policy.AntiScanEnabled {
		message += " [防扫图: 开启]"
	}
	if policy.TextObfuscationEnabled {
		message += " [图片混淆: 开启]"
	}
	return message
}

func (s *sSysPublish) canSendTelegramJob(ctx context.Context, job telegramJobRecord) (bool, error) {
	requireOnline := strings.HasPrefix(job.OperationNo, "full_push:") || isCycleBatchOperation(job.OperationNo)
	_, err := s.profilePublishSource(ctx, job.ProfileId, job.TenantId, job.AccountId, requireOnline)
	if err != nil {
		if errors.Is(err, errPublishProfileUnavailable) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func isCycleBatchOperation(operationNo string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(operationNo)), "cycle_batch:")
}
