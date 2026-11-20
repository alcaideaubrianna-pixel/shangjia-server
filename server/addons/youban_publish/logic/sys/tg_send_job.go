package sys

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hotgo/addons/youban_publish/model"
	"net"
	"os"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/utility/telegrambudget"
)

const (
	telegramBotMediaSendTimeout = telegrambudget.DefaultBotRequest
	telegramDisplaySendTimeout  = telegrambudget.DefaultDisplay
	telegramVerifySendTimeout   = telegrambudget.DefaultVerify
)

var (
	errTelegramJobSuperseded              = errors.New("TG推送任务已废弃")
	errTelegramDeliveryUncertain          = errors.New("Telegram发送结果待对账")
	errTelegramMediaFallbackQueued        = errors.New("Telegram媒体降级任务已提交")
	errTelegramMediaFallbackSubmitDelayed = errors.New("Telegram媒体降级任务等待重新提交")
	errTelegramMediaFallbackUnavailable   = errors.New("Telegram媒体降级不可用")
	errTelegramRemoteConfirmed            = errors.New("Telegram已发送成功，等待修复本地投影")
)

// telegramProfilePublishable 读取资料主表的发布可见性。资料 status 是
// 后台审核/上下架投影字段，采集自动发布可能先完成 visibility 写入而
// status 仍处于历史值；发送前不能再用 status==1 阻断已经公开的资料。
func telegramProfilePublishable(ctx context.Context, profileID int64) (bool, error) {
	if profileID <= 0 {
		return false, nil
	}
	var row struct {
		Visibility string `orm:"visibility"`
		Status     int    `orm:"status"`
	}
	if err := g.DB().Model("hg_content_profile").Safe().Ctx(ctx).
		Fields("visibility", "status").Where("id", profileID).WhereNull("deleted_at").Scan(&row); err != nil {
		// A missing or deleted profile is a stale queued job, not a transient
		// database failure. Let the caller supersede and settle it so the
		// channel queue can continue.
		if errors.Is(err, sql.ErrNoRows) || errors.Is(gerror.Cause(err), sql.ErrNoRows) {
			return false, nil
		}
		return false, gerror.Wrap(err, "读取资料发布状态失败")
	}
	return row.Status != sysin.ProfileStatusReviewPending && strings.EqualFold(strings.TrimSpace(row.Visibility), "public"), nil
}

// SendTelegramJob 串行执行目标频道任务，统一账户间隔、发送确认和完成后唤醒。
func (s *sSysPublish) SendTelegramJob(ctx context.Context, jobId int64) (resultErr error) {
	ctx, executionSpan := beginTelegramPublishExecution(ctx, tgQueuePayload{JobId: jobId})
	ctx, openStages := withPublishStageRegistry(ctx)
	ctx = telegramPublishTraceContext(ctx, jobId)
	started := time.Now()
	defer func() {
		if recovered := recover(); recovered != nil {
			openStages.close(errPublishStageAbandoned)
			finishTelegramPublishSpan(executionSpan, errors.New("publishing execution panicked"))
			logTelegramPublishSummary(ctx, started, fmt.Errorf("panic: %v", recovered))
			panic(recovered)
		}
		openStages.close(resultErr)
		logTelegramPublishSummary(ctx, started, resultErr)
		finishTelegramPublishSpan(executionSpan, resultErr)
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
	// 资料生命周期以后一次操作为准。下架可能与已入队的上架任务并发，
	// 发送前必须重新读取主资料状态，避免下架完成后仍把旧任务发到 Telegram。
	if targetJob.ProfileId > 0 && !isMessagePushOperationNo(targetJob.OperationNo) {
		if active, stateErr := telegramProfilePublishable(ctx, targetJob.ProfileId); stateErr != nil {
			return stateErr
		} else if !active {
			newer, checkErr := s.hasNewerSameChannelOperation(ctx, targetJob)
			if checkErr != nil {
				return checkErr
			}
			if newer {
				if supersedeErr := s.markTelegramJobSuperseded(ctx, targetJob.Id); supersedeErr != nil {
					return supersedeErr
				}
				return nil
			}
			// A non-public profile is a stale job. Leaving it pending blocks
			// the channel head forever and keeps the Bot operation processing.
			return s.supersedeTelegramJobAndCompleteOperation(ctx, targetJob)
		}
	}
	// A queued Bot notification may outlive the Account handoff. Do not let
	// ordinary scheduling overwrite its durable ownership marker.
	if telegramMediaFallbackJob(targetJob.LastDispatchError) {
		logTelegramFallbackHandoff(ctx, "waiting", targetJob, telegramMediaFallbackPurpose(targetJob.LastDispatchError), 0, "owned_by_account", targetJob.Status, targetJob.Status)
		return nil
	}
	if isMessagePushOperationNo(targetJob.OperationNo) {
		return s.SendMessagePushJob(ctx, jobId)
	}
	if telegramJobLifecycleNeedsRetry(targetJob.Status.String()) && strings.TrimSpace(targetJob.SendPhase.String()) == "" {
		step = beginTelegramPublishStage(ctx, "job.media_ready")
		ready, readyErr := s.profileMediaReady(ctx, targetJob.ProfileId)
		step.End(readyErr, g.Map{"ready": ready})
		if readyErr != nil {
			if isProfileMediaValidationError(readyErr) {
				_, settleErr := s.settleStreamFailedMediaJob(ctx, targetJob, readyErr)
				return settleErr
			}
			if errors.Is(readyErr, errTelegramProfileMissing) {
				return s.settleMissingTelegramProfileJob(ctx, targetJob)
			}
			return readyErr
		} else if !ready {
			beginTelegramPublishStage(ctx, "job.deferred").End(nil, g.Map{"reason": "media_not_ready"})
			return s.postponeTelegramJobUntilMediaReady(ctx, targetJob)
		}
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
	step.End(err, g.Map{"acquired": ok, "leaseTtlMs": telegramChannelLeaseTTL(ctx).Milliseconds()})
	if err != nil {
		return err
	}
	if !ok {
		beginTelegramPublishStage(ctx, "job.deferred").End(nil, g.Map{"reason": "channel_lease_busy", "alreadySending": targetJob.Status == "sending"})
		if targetJob.Status == "sending" {
			return nil
		}
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", jobId).
			WhereIn("status", []string{"pending", "failed_retry", "unknown"}).
			Where(telegramFallbackMarkerSQL(), telegramMediaFallbackMarker+"%").
			Where("(dispatch_status <> ? OR dispatch_status IS NULL OR next_retry_at IS NOT NULL OR COALESCE(last_dispatch_error,'') <> ?)",
				tgDispatchStatusIdle, "频道正在发送其他任务，已等待重新调度").
			Data(g.Map{
				"dispatch_status":     tgDispatchStatusIdle,
				"next_retry_at":       nil,
				"last_dispatch_error": "频道正在发送其他任务，已等待重新调度",
				"updated_at":          gtime.Now(),
			}).Update()
		// The lease holder wakes the highest-priority job when it finishes.
		// Re-enqueuing here creates a hot dispatch loop while the same channel is
		// still busy and can starve foreground publishes behind bulk work. The
		// periodic recovery loop remains the fallback if the lease holder exits.
		return nil
	}
	stopExecution := func() {}
	streamEngine := s.accountPushEngine(ctx, targetJob.TenantId, targetJob.AccountId) == pushEngineUnifiedStream
	if streamEngine {
		ctx, stopExecution = telegramStreamExecutionContext(ctx, lease)
	}
	// 所有持租约的退出路径都必须释放槽位；通过最终闸门后才唤醒后续任务。
	wakeNextAllowed := false
	defer func() {
		stopExecution()
		finalizeCtx, cancelFinalize := telegramFinalizeContext(ctx, "channel.send_lease.finalize")
		defer cancelFinalize()
		releaseStage := beginTelegramPublishStage(finalizeCtx, "channel.send_lease.release")
		s.releaseTelegramChannelLease(finalizeCtx, lease)
		releaseStage.End(nil)
		if !wakeNextAllowed {
			return
		}
		wakeStage := beginTelegramPublishStage(finalizeCtx, "channel.wake_next")
		wakeErr := s.wakeNextTelegramChannelJob(finalizeCtx, targetJob)
		wakeStage.End(wakeErr)
		if wakeErr != nil {
			g.Log().Warningf(finalizeCtx, "释放频道发送槽位后唤醒下一条TG任务失败 jobId:%d channelId:%d err:%+v", targetJob.Id, targetJob.ChannelId, wakeErr)
		}
	}()
	// Recheck under the send lease before the order gate: a paused channel must
	// settle its queued jobs even when an earlier job still blocks the head.
	if paused, pauseErr := s.postponePermissionPausedJob(ctx, targetJob); pauseErr != nil || paused {
		return pauseErr
	}
	// 获取频道执行租约后必须再次确认队首。Outbox/Stream 入队检查与
	// Worker 开始执行之间存在时间窗口，单靠前一次检查会让后创建的任务
	// 在旧任务尚未发送时提前进入 Telegram。此处在频道租约保护下重新读取
	// 主库，非队首任务立即退避并释放租约，禁止进入 sendMediaGroup。
	step = beginTelegramPublishStage(ctx, "channel.send_gate")
	gateWaitingOrder, gateErr := s.telegramChannelHasEarlierActiveJob(ctx, targetJob)
	step.End(gateErr, g.Map{"hasEarlier": gateWaitingOrder, "gate": "execution"})
	if gateErr != nil {
		return gerror.Wrap(gateErr, "执行阶段频道队首闸门检查失败")
	}
	if gateWaitingOrder {
		stopExecution()
		finalizeCtx, cancelFinalize := telegramFinalizeContext(ctx, "job.postpone_channel_order")
		defer cancelFinalize()
		if postponeErr := s.postponeTelegramJobForChannelOrder(finalizeCtx, targetJob); postponeErr != nil {
			return gerror.Wrap(postponeErr, "执行阶段非队首任务退避失败")
		}
		return nil
	}
	// 下架可能刚好发生在取队首/获取 lease 期间，再做一次最终闸门检查。
	if targetJob.ProfileId > 0 && !isMessagePushOperationNo(targetJob.OperationNo) {
		if active, stateErr := telegramProfilePublishable(ctx, targetJob.ProfileId); stateErr != nil {
			return stateErr
		} else if !active {
			stopExecution()
			finalizeCtx, cancelFinalize := telegramFinalizeContext(ctx, "job.supersede_before_send")
			defer cancelFinalize()
			if supersedeErr := s.markTelegramJobSuperseded(finalizeCtx, targetJob.Id); supersedeErr != nil {
				return supersedeErr
			}
			return nil
		}
	}
	wakeNextAllowed = true
	// 只拦截尚未开始发送的循环；在途、未知结果和已确认检查点继续对账/续跑。
	if targetJob.Status == telegramJobStatusPending || targetJob.Status == telegramJobStatusRetryWait {
		allowed, guardErr := cycleChannelAllowed(ctx, targetJob.TenantId, targetJob.ChannelId, targetJob.OperationNo, cycleProfileScope{ProfileID: targetJob.ProfileId, AccountID: targetJob.AccountId})
		if guardErr != nil {
			return guardErr
		}
		if !allowed {
			attempts, err := g.DB().Model(publishTgAttemptTable).Ctx(ctx).Where("job_id", targetJob.Id).Count()
			if err != nil {
				return err
			}
			if attempts == 0 {
				return s.supersedeTelegramJobAndCompleteOperation(ctx, targetJob)
			}
		}
	}
	step = beginTelegramPublishStage(ctx, "channel.publish_interval")
	delay, delayErr := time.Duration(0), error(nil)
	checkInterval := !telegramJobBypassesAutomaticSchedule(targetJob)
	// 手工优先只绕过自动发布时间窗口，不能绕过账户的频道发送间隔；删除仍使用独立通道。
	if streamEngine {
		checkInterval = !isDownTelegramOperationNo(targetJob.OperationNo)
	}
	if checkInterval {
		delay, delayErr = s.telegramPublishIntervalDelay(ctx, targetJob)
	}
	step.End(delayErr, g.Map{"delayMs": delay.Milliseconds()})
	if delayErr != nil {
		if streamEngine {
			return gerror.Wrap(delayErr, "读取频道推送间隔失败，等待重试")
		}
		g.Log().Warningf(ctx, "检查频道发送间隔失败，本次降级继续发送 jobId:%d channelId:%d err:%+v", targetJob.Id, targetJob.ChannelId, delayErr)
	} else if delay > 0 {
		return s.postponeTelegramJobForPublishInterval(ctx, targetJob, delay)
	}
	step = beginTelegramPublishStage(ctx, "job.reload_before_send")
	targetJob, err = s.telegramJobById(ctx, jobId)
	step.End(err)
	if err != nil {
		// 任务首次加载成功后，二次刷新偶发读空不能让频道队列永久卡死。
		// 保留首次读取的快照继续执行；首次加载仍严格返回错误，避免执行未知任务。
		if !errors.Is(err, sql.ErrNoRows) && !errors.Is(gerror.Cause(err), sql.ErrNoRows) {
			return err
		}
		g.Log().Warning(ctx, "TG任务二次刷新读空，继续使用已加载快照", g.Map{
			"jobId": jobId, "error": err.Error(), "event": "telegram_job_reload_fallback",
		})
	}
	if isDownTelegramOperationNo(targetJob.OperationNo) {
		return s.sendQueuedDownChannelTelegramJob(ctx, jobId)
	}
	step = beginTelegramPublishStage(ctx, "channel.order_check")
	waitingOrder, err := s.telegramChannelHasEarlierActiveJob(ctx, targetJob)
	step.End(err, g.Map{"hasEarlier": waitingOrder})
	if err != nil {
		// 前序查询失败时只退避当前频道任务，不能让 Asynq 高频重试占满
		// 全局资源；任务保持可恢复状态，其他频道继续发送。
		if rearmErr := s.postponeTelegramChannelJob(ctx, targetJob, 5*time.Second, "频道前序检查失败，等待重试"); rearmErr != nil {
			return gerror.Wrap(rearmErr, "频道前序检查失败且任务退避失败")
		}
		return gerror.Wrap(err, "频道前序检查失败，已退避重试")
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
		// 发送预算已到期时，不能只把错误交给队列重试；先隔离旧任务、释放
		// 频道队首并唤醒下一条，迟到的 Telegram 回执由 watchdog 对账处理。
		if strings.Contains(budgetErr.Error(), telegramSendTimeoutText(ctx)) {
			var expired telegramJobRecord
			var replacement int64
			finalizeCtx, cancelFinalize := telegramFinalizeContext(ctx, "job.expire")
			defer cancelFinalize()
			if fenceErr := g.DB().Transaction(finalizeCtx, func(txCtx context.Context, tx gdb.TX) error {
				var err error
				expired, replacement, err = s.expireTelegramSendTx(txCtx, tx, job.Id)
				return err
			}); fenceErr == nil && expired.Id > 0 {
				_ = s.wakeNextTelegramChannelJob(finalizeCtx, expired)
				if replacement > 0 {
					g.Log().Warningf(ctx, "TG发送预算到期已隔离并重入队 jobId:%d replacementJobId:%d", expired.Id, replacement)
				}
			}
		}
		return budgetErr
	}
	defer cancelBudget()
	ctx = bounded
	step = beginTelegramPublishStage(ctx, "job.check_operation_before_prepare")
	checkCtx, cancelCheck := context.WithTimeout(ctx, 2*time.Second)
	fenceErr := checkProfilePublishOperationBeforePrepare(checkCtx, job)
	cancelCheck()
	step.End(fenceErr)
	if fenceErr != nil {
		return s.handleTelegramJobError(ctx, job, fenceErr)
	}
	step = beginTelegramPublishStage(ctx, "job.can_send")
	allowed, err := s.canSendTelegramJob(ctx, job)
	step.End(err, g.Map{"allowed": allowed})
	if err != nil {
		return err
	}
	if !allowed {
		s.appendTelegramJobLog(ctx, job, "publish", "skipped", "上架任务已下架或不可发布，跳过TG推送")
		newer, checkErr := s.hasNewerSameChannelOperation(ctx, job)
		if checkErr != nil {
			return checkErr
		}
		if newer {
			return s.supersedeTelegramJobAndCompleteOperation(ctx, job)
		}
		return gerror.New("资料当前不可发布，等待资料操作状态收敛")
	}
	if relayPackageEnabled(ctx, job) {
		relayErr := s.sendRelayPackageJob(ctx, job)
		if errors.Is(relayErr, errTelegramMediaFallbackQueued) {
			return nil
		}
		if relayErr != nil {
			var current telegramJobRecord
			if loadErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Master().Where("id", job.Id).Scan(&current); loadErr == nil && current.Id > 0 {
				job = current
			}
			return s.handleTelegramJobError(ctx, job, relayErr)
		}
		return s.completeTelegramJob(ctx, job)
	}
	if !telegramSendPhaseHasCleanup(job.SendPhase.String()) {
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
	recordErr := s.upsertPublishJobRecord(ctx, job, model.PublishRecordStatusSending, "")
	step.End(recordErr)
	if recordErr != nil {
		g.Log().Warningf(ctx, "更新发布发送中记录失败 jobId:%d err:%+v", job.Id, recordErr)
	}
	step = beginTelegramPublishStage(ctx, "job.append_started_log")
	s.appendTelegramJobLog(ctx, job, "publish", "started", s.telegramJobPublishMessage(ctx, job, "开始推送TG资料"))
	step.End(nil)
	if err = s.sendLockedTelegramJob(ctx, job); err != nil {
		if errors.Is(err, errTelegramMediaFallbackQueued) {
			s.appendTelegramJobLog(ctx, job, "account_fallback", "waiting", "协议号媒体降级交接状态已持久化，等待账号服务接管并完成当前阶段")
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
	if job.CreatedAt != nil {
		observeTelegramProfilePublishQueueWait(ctx, job, time.Since(job.CreatedAt.Time))
	}
	ctx = telegramPublishJobTraceContext(ctx, job)
	stage := beginTelegramPublishStage(ctx, "send.execute")
	startedAt := time.Now()
	defer stage.Finish(&resultErr)
	defer func() {
		result := "success"
		if resultErr != nil {
			result = "error"
		}
		observeTelegramProfilePublish(ctx, job, result, time.Since(startedAt), resultErr)
	}()
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
	if len(displayMedia) == 0 {
		ready, checkErr := s.profileDisplayMediaReady(ctx, job.ProfileId)
		if checkErr != nil {
			return checkErr
		}
		if !ready {
			return gerror.New("资料媒体还在处理中，请稍后再上架")
		}
	}
	step = beginTelegramPublishStage(ctx, "prepare.verify_media")
	verifyMedia, err := s.telegramJobMedia(ctx, job, "verify")
	step.End(err, g.Map{"mediaCount": len(verifyMedia)})
	if err != nil {
		return err
	}
	// Collected profiles must arrive with their verification media already
	// persisted. Reject an incomplete historical job before sending its display;
	// media matching belongs to collection, never to the publish worker.
	if !telegramCollectedVerificationReady(job, verifyMedia) {
		g.Log().Warning(ctx, "采集资料缺少验证媒体，拒绝发送展示", g.Map{
			"event": "telegram_collect_media_incomplete", "jobId": job.Id,
			"profileId": job.ProfileId, "channelId": job.ChannelId,
			"collectEventId": job.CollectEventId, "collectSourceId": job.CollectSourceId,
			"reason": "verification_media_missing", "displaySent": telegramSendPhaseHasDisplay(job.SendPhase.String()),
		})
		return gerror.New("采集资料验证媒体未完整写入，禁止发送展示，请完成采集配对后重试")
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
	if telegramAntiScanMediaCount(displayMedia)+telegramAntiScanMediaCount(verifyMedia) > 0 {
		step = beginTelegramPublishStage(ctx, "prepare.content_protection_all")
		err = s.prepareTelegramAntiScanMediaSet(ctx, displayMedia, verifyMedia)
		step.End(err, g.Map{"displayMediaCount": len(displayMedia), "verifyMediaCount": len(verifyMedia)})
		if err != nil {
			// Anti-scan preparation is a best-effort pre-send transform. Never
			// hold the channel lease until the global publish deadline when a
			// provider/cache is unavailable; release prepared files and fall back
			// to the original media so the queue can make progress.
			g.Log().Warningf(ctx, "防扫图准备失败，降级发送原始媒体 jobId:%d err:%+v", job.Id, err)
			fallbackTelegramAntiScanToOriginal(displayMedia, verifyMedia)
		}
		if telegramAntiScanMediaCount(displayMedia)+telegramAntiScanMediaCount(verifyMedia) > 0 {
			defer cleanupPreparedTelegramMedia(displayMedia, verifyMedia)
		}
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
	if telegramSendPhaseHasDisplay(job.SendPhase.String()) && len(verifyMedia) > 0 {
		displayConfirmed, evidenceErr := s.telegramDisplayDeliveryConfirmed(ctx, job.Id, telegramDisplayExpectedCount(len(displayMedia)))
		if evidenceErr != nil {
			return evidenceErr
		}
		if !displayConfirmed {
			return gerror.New("TG任务缺少完整展示发送证据，禁止孤立发送验证资料")
		}
	}
	messages := make([]*telegramSentMessage, 0)
	if !telegramSendPhaseHasDisplay(job.SendPhase.String()) {
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
			displaySendMedia := telegramMediaSetForGatewayRetry(job, displayMedia)
			g.Log().Infof(ctx, "TG展示资料开始Bot发送 jobId:%d botId:%d chat:%s media:%s", job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(displaySendMedia))
			stopAttemptHeartbeat := s.startTelegramAttemptHeartbeat(attempt.Id)
			displaySendCtx, cancelDisplaySend := context.WithTimeout(ctx, telegrambudget.Display(ctx))
			displaySendCtx = telegramPublishTraceContext(displaySendCtx, job.Id, g.Map{"attemptId": attempt.Id})
			displaySendStartedAt := time.Now()
			step = beginTelegramPublishStage(displaySendCtx, "display.api_send", g.Map{"attemptId": attempt.Id, "mediaCount": len(displayMedia)})
			messages, err = s.sendTelegramDisplayPart(displaySendCtx, bot, job.TargetChatId, displayCaption, displaySendMedia)
			step.End(err, g.Map{"messageIds": telegramSentMessageIDs(messages), "messageCount": len(messages)})
			cancelDisplaySend()
			displaySendDuration := time.Since(displaySendStartedAt)
			observeTelegramMediaSend(ctx, &job, "display", err, displaySendDuration)
			g.Log().Info(ctx, "TG展示资料Bot发送结束", g.Map{"event": "telegram_bot_send", "purpose": "display", "transport": "bot", "profileId": job.ProfileId, "operationNo": job.OperationNo, "stageTimeoutMs": telegrambudget.Display(ctx).Milliseconds(), "job_id": job.Id, "attempt_id": attempt.Id, "bot_id": job.BotId, "account_id": job.AccountId, "channel_id": job.ChannelId, "target_chat_id": job.TargetChatId, "media_count": len(displayMedia), "send_mode": "bot", "retry_count": job.RetryCount, "duration": displaySendDuration.Milliseconds(), "error_type": telegramSendErrorType(err), "error_code": telegramSendErrorCode(err), "message_ids": telegramSentMessageIDs(messages), "error": telegramPublishLogError(err)})
			if err != nil {
				if canceled, cancelErr := recordCanceledSendOutcome(ctx, job, attempt, messages, err); cancelErr != nil {
					return cancelErr
				} else if canceled {
					return errTelegramJobSuperseded
				}
			}
			if err != nil && len(messages) == 0 && s.shouldFallbackAfterBotFailure(ctx, job, err) {
				g.Log().Warningf(ctx, "Bot展示媒体发送失败，命中媒体降级条件 jobId:%d botId:%d chat:%s media:%s err:%+v", job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(displayMedia), err)
				messages, err = s.sendTelegramJobMediaByAccount(ctx, job, attempt.Id, "display", displayCaption, displayMedia, err)
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
					return s.awaitTelegramUnknownResponse(ctx, job, attempt, messages, err)
				}
				ctx, cancelFinalize := telegramFinalizeContext(ctx, "display.failed_response")
				defer cancelFinalize()
				s.abandonTelegramDeliveryAttempt(ctx, attempt, messages, err)
				_ = s.cleanupTelegramSentMessages(ctx, bot, job.TargetChatId, messages, "展示资料分片推送失败")
				return gerror.Wrapf(err, "TG展示资料推送失败，job:%d，channel:%d，chat:%s", job.Id, job.ChannelId, job.TargetChatId)
			}
			if err = validateTelegramMediaSendResult("display", displayMedia, messages); err != nil {
				return s.awaitTelegramUnknownResponse(ctx, job, attempt, messages, telegramDeliveryUncertainError(err))
			}
			// Persist authoritative response IDs before starting verification. A process
			// restart must not lose the display evidence while verification is in flight.
			step = beginTelegramPublishStage(ctx, "display.confirm_api_success", g.Map{"attemptId": attempt.Id, "messageIds": telegramSentMessageIDs(messages)})
			err = s.confirmTelegramAPISuccess(ctx, job, attempt, messages)
			step.End(err, g.Map{"apiConfirmed": true, "localCommitted": err == nil, "waitForWebhook": false})
			if err != nil {
				return gerror.Wrap(errTelegramRemoteConfirmed, err.Error())
			}
			s.cacheTelegramReturnedFileIDs(ctx, job, "display", messages)
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
	verifySendCtx, cancelVerifySend := context.WithTimeout(ctx, telegrambudget.Verify(ctx))
	verifySendCtx = telegramPublishTraceContext(verifySendCtx, job.Id, g.Map{"attemptId": verifyAttempt.Id})
	verifySendStartedAt := time.Now()
	step = beginTelegramPublishStage(verifySendCtx, "verify.api_send", g.Map{"attemptId": verifyAttempt.Id, "mediaCount": len(verifyMedia)})
	verifyMessages, err := s.sendTelegramVerifyPart(verifySendCtx, bot, job.TargetChatId, verifyCaption, verifyMedia)
	step.End(err, g.Map{"messageIds": telegramSentMessageIDs(verifyMessages), "messageCount": len(verifyMessages)})
	cancelVerifySend()
	verifySendDuration := time.Since(verifySendStartedAt)
	observeTelegramMediaSend(ctx, &job, "verify", err, verifySendDuration)
	g.Log().Info(ctx, "TG验证资料Bot发送结束", g.Map{"event": "telegram_bot_send", "purpose": "verify", "transport": "bot", "profileId": job.ProfileId, "operationNo": job.OperationNo, "stageTimeoutMs": telegrambudget.Verify(ctx).Milliseconds(), "job_id": job.Id, "attempt_id": verifyAttempt.Id, "bot_id": job.BotId, "account_id": job.AccountId, "channel_id": job.ChannelId, "target_chat_id": job.TargetChatId, "media_count": len(verifyMedia), "send_mode": "bot", "retry_count": job.RetryCount, "duration": verifySendDuration.Milliseconds(), "error_type": telegramSendErrorType(err), "error_code": telegramSendErrorCode(err), "message_ids": telegramSentMessageIDs(verifyMessages), "error": telegramPublishLogError(err)})
	if err != nil {
		if canceled, cancelErr := recordCanceledSendOutcome(ctx, job, verifyAttempt, verifyMessages, err); cancelErr != nil {
			return cancelErr
		} else if canceled {
			return errTelegramJobSuperseded
		}
	}
	if err != nil && len(verifyMessages) == 0 && s.shouldFallbackAfterBotFailure(ctx, job, err) {
		g.Log().Warningf(ctx, "Bot验证媒体发送失败，命中媒体降级条件 jobId:%d botId:%d chat:%s media:%s err:%+v", job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(verifyMedia), err)
		verifyMessages, err = s.sendTelegramJobMediaByAccount(ctx, job, verifyAttempt.Id, "verify", verifyCaption, verifyMedia, err)
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
			return s.awaitTelegramUnknownResponse(ctx, job, verifyAttempt, verifyMessages, err)
		}
		ctx, cancelFinalize := telegramFinalizeContext(ctx, "verify.failed_response")
		defer cancelFinalize()
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
	if err = validateTelegramMediaSendResult("verify", verifyMedia, verifyMessages); err != nil {
		return s.awaitTelegramUnknownResponse(ctx, job, verifyAttempt, verifyMessages, telegramDeliveryUncertainError(err))
	}
	step = beginTelegramPublishStage(ctx, "verify.confirm_api_success", g.Map{"attemptId": verifyAttempt.Id, "messageIds": telegramSentMessageIDs(verifyMessages)})
	err = s.confirmTelegramAPISuccess(ctx, job, verifyAttempt, verifyMessages)
	step.End(err, g.Map{"apiConfirmed": true, "localCommitted": err == nil, "waitForWebhook": false})
	if err != nil {
		return gerror.Wrap(errTelegramRemoteConfirmed, err.Error())
	}
	s.cacheTelegramReturnedFileIDs(ctx, job, "verify", verifyMessages)
	// Confirmation persists returned IDs even if cancellation raced the API
	// response. Check ownership only after that evidence is durable.
	finalizeCtx, cancelFinalize := telegramFinalizeContext(ctx, "verify.response_ownership")
	defer cancelFinalize()
	if stillSending, stateErr := s.telegramJobStillSending(finalizeCtx, job.Id); stateErr != nil {
		return gerror.Wrap(errTelegramRemoteConfirmed, stateErr.Error())
	} else if !stillSending {
		return errTelegramJobSuperseded
	}
	return nil
}

// profileDisplayMediaReady prevents a profile with declared media from being
// silently downgraded to a text-only Telegram post while its media queue is
// still writing rows or processing storage.
func (s *sSysPublish) profileDisplayMediaReady(ctx context.Context, profileID int64) (bool, error) {
	var profile struct {
		ImageCount           int   `json:"image_count"`
		VideoCount           int   `json:"video_count"`
		HasVerificationVideo int   `json:"has_verification_video"`
		CoverMediaId         int64 `json:"cover_media_id"`
	}
	if err := g.DB().Model("hg_content_profile").Safe().Ctx(ctx).
		Fields("image_count,video_count,has_verification_video,cover_media_id").Where("id", profileID).WhereNull("deleted_at").Scan(&profile); err != nil {
		if errors.Is(err, sql.ErrNoRows) || errors.Is(gerror.Cause(err), sql.ErrNoRows) {
			return false, errTelegramProfileMissing
		}
		return false, gerror.Wrap(err, "检查资料媒体状态失败")
	}
	if profile.ImageCount <= 0 && profile.VideoCount <= 0 && profile.HasVerificationVideo <= 0 && profile.CoverMediaId <= 0 {
		return true, nil
	}
	return false, nil
}

// Collected jobs require an actual verification video before their first external
// send. Manual display-only profiles keep their existing publishing contract.
func telegramCollectedVerificationReady(job telegramJobRecord, media []*telegramMediaItem) bool {
	if job.CollectEventId <= 0 {
		return true
	}
	for _, item := range media {
		if item != nil && strings.EqualFold(strings.TrimSpace(item.MediaType), "video") {
			return true
		}
	}
	return false
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
	// Explicit request timeouts are handed to the existing account queue.
	// Other ambiguous transport results wait for reconciliation and must not
	// be blindly duplicated; known 5xx/network failures remain retryable.
	// Rate-limit handoff is further gated by shouldFallbackAfterBotFailure,
	// which gives the channel's remaining Bot IDs a chance first.
	if isTelegramAPITimeout(err) {
		return true
	}
	// Telegram's retry_after is an authoritative Bot-side rate-limit response.
	// When no message was returned, waiting for another Bot attempt only burns
	// the publish budget; hand the media to the protocol account immediately.
	if _, rateLimited := telegramRateLimitRetryDelay(err); rateLimited {
		return true
	}
	if isTelegramAmbiguousDeliveryError(err) {
		return false
	}
	if isTelegramNetworkRetryError(err) {
		return true
	}
	return isTelegramMediaSizeLimitError(err) ||
		isTelegramMediaSourceUnavailableError(err) ||
		isTelegramBotMediaProcessingError(err)
}

// telegramBotAttemptsBeforeAccountFallback limits the Bot path to two total
// attempts. The next delivery attempt is handed to the existing protocol
// account fallback queue, so a third external send is not another Bot retry.
const telegramBotAttemptsBeforeAccountFallback = 2

func shouldFallbackTelegramMediaToAccountAfterBotRetries(err error, retryCount int) bool {
	if !shouldFallbackTelegramMediaToAccount(err) {
		return false
	}
	if _, rateLimited := telegramRateLimitRetryDelay(err); rateLimited {
		return true
	}
	// retryCount is the number of completed Bot failures persisted on the job.
	// Keep the first attempt and one Bot retry; the following attempt uses the
	// existing protocol account fallback queue.
	return retryCount+1 >= telegramBotAttemptsBeforeAccountFallback
}

// shouldFallbackAfterBotFailure gives every configured channel Bot one chance
// on an authoritative rate limit before handing the media to the protocol
// account. Other Bot errors retain the normal retry/fallback policy.
func (s *sSysPublish) shouldFallbackAfterBotFailure(ctx context.Context, job telegramJobRecord, err error) bool {
	// An interrupted request may already have reached Telegram. Persist and
	// reconcile its attempt before another transport is allowed to send it.
	if isTelegramAmbiguousDeliveryError(err) {
		return false
	}
	if _, rateLimited := telegramRateLimitRetryDelay(err); rateLimited {
		nextBotID, nextErr := s.nextTelegramChannelBotId(ctx, job)
		if nextErr == nil && nextBotID > 0 && nextBotID != job.BotId {
			return false
		}
	}
	return shouldFallbackTelegramMediaToAccountAfterBotRetries(err, job.RetryCount)
}

func telegramSendErrorCode(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	for _, code := range []string{"502", "503", "504", "429", "400", "401", "403"} {
		if strings.Contains(message, code) {
			return code
		}
	}
	return ""
}

func telegramSendErrorType(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
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
	ctx, cancelCleanup := telegramFinalizeContextWithTimeout(ctx, "response.partial_cleanup", telegrambudget.Cleanup(ctx))
	defer cancelCleanup()
	chatId = normalizeTelegramChannelChatID(chatId)
	for _, item := range messages {
		if item == nil || item.MessageId <= 0 {
			continue
		}
		requestStarted := time.Now()
		deleted, err := bot.DeleteMessage(ctx, &tgbot.DeleteMessageParams{ChatID: chatId, MessageID: int(item.MessageId)})
		publishCleanupObservability.messageRequest(ctx, deleted, time.Since(requestStarted), err)
		if err != nil {
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
	requestStarted := time.Now()
	msg, err := bot.SendMessage(ctx, &tgbot.SendMessageParams{ChatID: chatId, Text: caption, ParseMode: models.ParseModeHTML})
	observePublishTransportRequest(ctx, "sendMessage", "display", "not_applicable", requestStarted, err, msg != nil && msg.ID > 0)
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

func (s *sSysPublish) sendTelegramJobMediaByAccount(ctx context.Context, job telegramJobRecord, attemptID int64, purpose string, caption string, media []*telegramMediaItem, botErr error) (resultMessages []*telegramSentMessage, resultErr error) {
	if !telegramFallbackBotFailureKnown(botErr) {
		logTelegramFallbackHandoff(ctx, "rejected", job, purpose, 0, "bot_delivery_unresolved", job.Status, job.Status)
		return nil, telegramDeliveryUncertainError(botErr)
	}
	// Handoff only persists local intent; the Account worker starts its send
	// with its own live execution context, lease and durable job deadline.
	ctx, cancel := telegramFinalizeContext(ctx, "account_fallback.submit")
	defer cancel()
	ctx = telegramPublishJobTraceContext(ctx, job)
	stage := beginTelegramPublishStage(ctx, "account_fallback.submit", g.Map{"purpose": purpose, "mediaCount": len(media), "botError": telegramPublishLogError(botErr)})
	defer stage.Finish(&resultErr)
	if err := s.recordTelegramFallbackBotRejection(ctx, job, attemptID, purpose, botErr); err != nil {
		return nil, gerror.Wrap(err, "保存Bot明确失败事实失败，取消协议号交接")
	}
	channel, err := s.messagePushChannelFromJob(ctx, job)
	if err != nil {
		return nil, gerror.Wrap(err, "读取协议号降级发送频道失败")
	}
	if channel.TgAccountId <= 0 {
		return nil, gerror.Newf("Bot发送媒体超过限制，频道未绑定协议号，无法整组降级发送：%v", botErr)
	}
	g.Log().Warning(ctx, "Bot媒体发送触发协议号整组降级", g.Map{
		"event": "telegram_protocol_fallback", "job_id": job.Id, "attempt_id": attemptID,
		"bot_id": job.BotId, "account_id": job.AccountId, "channel_id": job.ChannelId,
		"target_chat_id": job.TargetChatId, "media_count": len(media), "send_mode": "protocol",
		"retry_count": job.RetryCount, "duration": 0, "error_type": telegramSendErrorType(botErr),
		"error_code": telegramSendErrorCode(botErr), "purpose": purpose,
		"media": telegramMediaDebugSummary(media), "error": telegramPublishLogError(botErr),
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

// Verification fallback must preserve confirmed display progress.
func telegramMediaFallbackTransition(job telegramJobRecord, purpose string) telegramJobTransition {
	phase := SendPhaseCleanupConfirmed
	expected := SendPhaseDisplaySending
	if purpose == "verify" {
		phase = SendPhaseDisplayConfirmed
		expected = SendPhaseVerifySending
	}
	return telegramJobTransition{
		JobId: job.Id, From: []JobStatus{JobStatusSending}, To: JobStatusRetryWait,
		SendPhases: []SendPhase{expected}, SendPhase: jobPhase(phase),
		DispatchStatus: jobDispatch(DispatchStatusIdle), RetryDelay: telegramMediaFallbackRecoverAfter,
		Additional: g.Map{"last_dispatch_error": telegramMediaFallbackReason(purpose), "reconcile_count": job.ReconcileCount + 1},
	}
}

func (s *sSysPublish) submitTelegramMediaFallback(ctx context.Context, job telegramJobRecord, tgAccountID int64, purpose string) error {
	return s.requestTelegramMediaFallback(ctx, job, tgAccountID, strings.TrimSpace(purpose))
}

func (s *sSysPublish) handleTelegramJobError(ctx context.Context, job telegramJobRecord, err error) (resultErr error) {
	ctx = telegramPublishJobTraceContext(ctx, job)
	ctx, cancelFinalize := telegramFinalizeContext(ctx, "job.handle_error")
	defer cancelFinalize()
	stage := beginTelegramPublishStage(ctx, "job.handle_error", g.Map{"cause": telegramPublishLogError(err)})
	defer stage.Finish(&resultErr)
	if errors.Is(err, errTelegramJobSuperseded) {
		beginTelegramPublishStage(ctx, "job.error_decision").End(nil, g.Map{"decision": "settle_superseded"})
		return s.settleSupersededTelegramSend(ctx, job)
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
	if !telegramSendPhaseIsCleanup(job.SendPhase.String()) && !isTelegramAccountBusyError(err) && isTelegramAmbiguousDeliveryError(err) {
		return s.markTelegramJobUnknown(ctx, job, err)
	}
	allowed, allowedErr := s.canSendTelegramJob(ctx, job)
	if allowedErr == nil && !allowed {
		s.appendTelegramJobLog(ctx, job, "publish", "superseded", "TG推送失败时任务已被新操作覆盖，旧任务已废弃")
		return s.supersedeTelegramJobAndCompleteOperation(ctx, job)
	}
	_, rateLimited := telegramRateLimitRetryDelay(err)
	var tooMany *tgbot.TooManyRequestsError
	if (rateLimited || errors.As(err, &tooMany)) && !relayPackageEnabled(ctx, job) {
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
	additional := g.Map{"retry_count": decision.RetryCount, "last_dispatch_error": decision.Message}
	step := beginTelegramPublishStage(ctx, "job.failure_transition", g.Map{"nextStatus": decision.Status, "retryCount": decision.RetryCount, "retryDelayMs": decision.RetryDelay.Milliseconds()})
	// Retry scheduling never rewrites the durable job deadline. Bot switches,
	// rate limits and account handoffs all consume the same original budget.
	failureTransition := telegramJobTransition{
		JobId:              job.Id,
		From:               []JobStatus{telegramJobStatusSending},
		To:                 JobStatus(decision.Status),
		RetryDelay:         decision.RetryDelay,
		ErrorMessage:       decision.Message,
		Additional:         additional,
		CleanupStartedSend: decision.Status == telegramJobStatusFailed,
	}
	var transitioned bool
	var updateErr error
	membershipFailure := decision.Status == telegramJobStatusFailed && isTelegramBotNotMemberError(err)
	if membershipFailure {
		transitioned, updateErr = s.failTelegramBotNotMember(ctx, job, failureTransition, err)
	} else {
		transitioned, updateErr = s.transitionTelegramJob(ctx, failureTransition)
	}
	step.End(updateErr, g.Map{"transitioned": transitioned})
	if updateErr != nil {
		return gerror.Wrap(updateErr, "更新TG失败任务状态失败")
	}
	if !transitioned {
		return nil
	}
	observePublishTerminalCommit(ctx, job, JobStatus(decision.Status), transitioned, nil)
	if decision.Status == "failed" && !membershipFailure {
		if circuitErr := s.recordChannelPermissionFailure(ctx, job, err); circuitErr != nil {
			g.Log().Warningf(ctx, "记录频道权限熔断失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, circuitErr)
		}
	}
	recordJobStatus, validRecordJobStatus := ParseJobStatus(decision.Status)
	if !validRecordJobStatus {
		recordJobStatus = JobStatusUnknown
	}
	if recordErr := s.upsertPublishJobRecord(ctx, job, publishRecordStatusFromJob(recordJobStatus), decision.Message); recordErr != nil {
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
	if telegramMediaFallbackJob(job.LastDispatchError) {
		return s.failOwnedTelegramFallbackJob(ctx, job, cause)
	}
	return s.failNonAccountTelegramJob(ctx, job, cause)
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
		From:  []JobStatus{telegramJobStatusSending},
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
	observePublishTransportFallback(ctx, "bot_switched", "unknown", "bot", "not_applicable", "unknown", "rate_limited")
	job.BotId = nextBotId
	if err = s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusPending); err != nil {
		return false, err
	}
	if recordErr := s.upsertPublishJobRecord(ctx, job, model.PublishRecordStatusPending, "已切换备用BOT，等待重新发送"); recordErr != nil {
		g.Log().Warningf(ctx, "更新发布待发送记录失败 jobId:%d err:%+v", job.Id, recordErr)
	}
	s.appendTelegramJobLog(ctx, job, "publish", "bot_switched", fmt.Sprintf("当前BOT限流，已切换备用BOT：%d，原因：%s", nextBotId, cause.Error()))
	return true, nil
}

func (s *sSysPublish) completeTelegramJob(ctx context.Context, job telegramJobRecord) error {
	ctx = telegramPublishJobTraceContext(ctx, job)
	ctx, cancelFinalize := telegramFinalizeContext(ctx, "job.complete")
	defer cancelFinalize()
	if status, err := s.telegramJobCurrentStatus(ctx, job.Id); err != nil {
		return err
	} else if status != telegramJobStatusSending && !(status == telegramJobStatusConfirming && (job.SendPhase == telegramSendPhaseVerifyConfirmed || job.SendPhase == telegramSendPhaseCompletedNoVerify)) {
		s.appendTelegramJobLog(ctx, job, "publish", "skipped", "TG发送完成前任务状态已变更，跳过成功标记："+status)
		return nil
	}
	allowed, err := s.canSendTelegramJob(ctx, job)
	if err != nil {
		return err
	}
	if !allowed {
		// The remote delivery is definitive even if the profile was taken down
		// concurrently. Release the channel before cleanup/projection lock work;
		// cleanup can be retried without risking a duplicate Telegram send.
		if err = s.markTelegramJobSuperseded(ctx, job.Id); err != nil {
			return err
		}
		err = s.withProfileLifecycleLock(ctx, job.TenantId, job.ProfileId, func() error {
			s.appendTelegramJobLog(ctx, job, "publish", "skipped", "TG发送完成前任务已下架，开始清理已发送消息")
			if err = s.deleteTelegramMessageSetLockedByChannel(ctx, job, "任务已下架"); err != nil {
				return err
			}
			_, completeErr := s.completeProfileTelegramOperation(ctx, job, isCycleBatchOperation(job.OperationNo))
			return completeErr
		})
		if err != nil {
			s.appendTelegramJobLog(ctx, job, "projection", "pending", "TG任务已下架，消息清理及资料状态等待自动恢复："+err.Error())
			g.Log().Warningf(ctx, "TG任务已废弃，消息清理及资料状态等待恢复 jobId:%d profileId:%d err:%+v", job.Id, job.ProfileId, err)
		}
		return nil
	}
	if err := s.validateTelegramJobCompletion(ctx, job.Id, job.ProfileId); err != nil {
		return err
	}

	// Remote delivery is already durably confirmed at this point. Persist that
	// fact before taking the profile projection lock: projection contention must
	// never leave a confirmed job holding the channel's sending slot.
	sentAt := gtime.Now()
	transition := telegramJobTransition{JobId: job.Id, From: []JobStatus{telegramJobStatusSending, telegramJobStatusConfirming}, To: telegramJobStatusSent, Additional: g.Map{"sent_at": sentAt}}
	streamEngine := s.accountPushEngine(ctx, job.TenantId, job.AccountId) == pushEngineUnifiedStream
	var transitioned bool
	confirmationStarted := time.Now()
	if streamEngine {
		transitioned, err = s.confirmTelegramJobAndCyclePlan(ctx, job, transition)
	} else {
		transitioned, err = s.confirmTelegramJobAndRecovery(ctx, transition)
	}
	observePublishTerminalProjection(ctx, transitioned, err, time.Since(confirmationStarted))
	if err != nil {
		if streamEngine {
			return gerror.Wrap(errTelegramRemoteConfirmed, "保存已确认发送及频道计划失败："+err.Error())
		}
		return gerror.Wrap(errTelegramRemoteConfirmed, "保存已确认发送及补偿状态失败："+err.Error())
	}
	if !transitioned {
		s.appendTelegramJobLog(ctx, job, "publish", "skipped", "TG发送完成时任务已不再发送中，跳过成功标记")
		current, loadErr := s.telegramJobCurrentStatus(ctx, job.Id)
		if loadErr != nil {
			return gerror.Wrap(loadErr, "读取TG任务最终状态失败")
		}
		switch current {
		case telegramJobStatusSent, telegramJobStatusFailed, telegramJobStatusSuperseded:
			return nil
		default:
			return gerror.Newf("TG发送已完成但任务状态未落库，当前状态：%s", current)
		}
	}
	observePublishTerminalCommit(ctx, job, telegramJobStatusSent, transitioned, nil)
	s.completeTelegramJobDeliveryProjection(ctx, job, sentAt)

	if s.accountPushEngine(ctx, job.TenantId, job.AccountId) == pushEngineUnifiedStream {
		err = s.enqueueStreamProfileProjection(ctx, job)
	} else {
		err = s.projectCompletedTelegramOperation(ctx, job)
	}
	if err != nil {
		s.appendTelegramJobLog(ctx, job, "projection", "pending", "TG已发送成功，资料投影等待恢复："+err.Error())
		g.Log().Warningf(ctx, "TG任务已确认，资料投影等待恢复 jobId:%d profileId:%d err:%+v", job.Id, job.ProfileId, err)
	}
	return nil
}

func (s *sSysPublish) completeTelegramJobDeliveryProjection(ctx context.Context, job telegramJobRecord, sentAt *gtime.Time) {
	if stepErr := s.projectTelegramDeliveryStep(ctx, job.Id, telegramDeliveryStepFinalize, telegramDeliveryStepSucceeded, 0, "", sentAt); stepErr != nil {
		g.Log().Warningf(ctx, "投影TG完成步骤失败 jobId:%d err:%+v", job.Id, stepErr)
	}
	job.SentAt = sentAt
	if cycleErr := s.syncProfileCycleRunJob(ctx, job); cycleErr != nil {
		g.Log().Warningf(ctx, "回写资料循环频道状态失败 job:%d err:%+v", job.Id, cycleErr)
	}
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
}

func (s *sSysPublish) completeTelegramJobLockedByProfile(ctx context.Context, job telegramJobRecord, completedNotice *bool) error {
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
	isCycle := isCycleBatchOperation(job.OperationNo)
	operationCompleted, completeErr := s.completeProfileTelegramOperation(ctx, job, isCycle)
	if completeErr != nil {
		return completeErr
	}
	if !operationCompleted {
		return nil
	}
	if completedNotice != nil {
		*completedNotice = true
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

func (s *sSysPublish) hasNewerSameChannelOperation(ctx context.Context, job telegramJobRecord) (bool, error) {
	query := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("tenant_id", job.TenantId).
		Where("profile_id", job.ProfileId).
		Where("channel_id", job.ChannelId).
		// Use an explicit expression here.  This predicate is on the hot send
		// path; keeping the operator in the expression prevents argument
		// shifting in adapters that treat Where's variadic form differently.
		Wheref("id > ?", job.Id).
		WhereIn("status", []string{"pending", "sending", "confirming", "sent", "failed_retry", "unknown"}).
		WhereNot("operation_no", job.OperationNo)
	if strings.TrimSpace(job.OperationType) == "" {
		query = query.Where("(COALESCE(operation_type,'')='' OR operation_type='publish')")
	} else {
		query = query.Where("operation_type", job.OperationType)
	}
	count, err := query.Count()
	if err != nil {
		return false, gerror.Wrap(err, "检查TG任务替换关系失败")
	}
	return count > 0, nil
}

// isCycleBatchOperation 统一识别旧批次循环和资料级循环，生命周期不再按入口分叉。
func isCycleBatchOperation(operationNo string) bool {
	operationNo = strings.ToLower(strings.TrimSpace(operationNo))
	return strings.HasPrefix(operationNo, "cycle_batch:") || strings.HasPrefix(operationNo, "profile_cycle:")
}
