package sys

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"golang.org/x/sync/errgroup"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_publish/model/input/sysin"
)

var (
	errTelegramJobSuperseded       = errors.New("TG推送任务已废弃")
	errTelegramDeliveryUncertain   = errors.New("Telegram发送结果待对账")
	errTelegramMediaFallbackQueued = errors.New("Telegram媒体降级任务已提交")
)

func (s *sSysPublish) SendTelegramJob(ctx context.Context, jobId int64) error {
	targetJob, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	if isMessagePushOperationNo(targetJob.OperationNo) {
		return s.SendMessagePushJob(ctx, jobId)
	}
	if ready, readyErr := s.profileMediaReady(ctx, targetJob.ProfileId); readyErr != nil {
		return readyErr
	} else if !ready {
		return s.postponeTelegramJobUntilMediaReady(ctx, jobId)
	}
	if targetJob.CollectSourceId > 0 && !s.collectPushEnabled(ctx) {
		return s.postponeTelegramJobForCollectPushPause(ctx, targetJob)
	}
	// 预热媒体文件缓存不改变发送顺序，只把远程下载和视频预览图准备
	// 提前到频道发送锁之前，允许同频道后续任务形成有限流水线。
	if err = s.prewarmTelegramJobMedia(ctx, targetJob); err != nil {
		g.Log().Warningf(ctx, "TG任务媒体缓存预热失败，继续走发送流程 jobId:%d err:%+v", jobId, err)
	}
	lease, ok, err := s.tryTelegramChannelLease(ctx, targetJob.TargetChatId)
	if err != nil {
		return err
	}
	if !ok {
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
		return s.enqueueTelegramJobDirectWithUnique(ctx, jobId, delay, false)
	}
	defer s.releaseTelegramChannelLease(ctx, lease)
	targetJob, err = s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	if targetJob.Status == "unknown" {
		return s.reconcileUnknownTelegramJob(ctx, targetJob)
	}
	if isDownTelegramOperationNo(targetJob.OperationNo) {
		return s.sendQueuedDownChannelTelegramJob(ctx, jobId)
	}
	waitingOrder, err := s.telegramChannelHasEarlierActiveJob(ctx, targetJob)
	if err != nil {
		return err
	}
	if waitingOrder {
		return s.postponeTelegramJobForChannelOrder(ctx, targetJob)
	}
	return s.sendTelegramJobLockedByChannel(ctx, jobId)
}

func (s *sSysPublish) prewarmTelegramJobMedia(ctx context.Context, job telegramJobRecord) error {
	displayMedia, err := s.telegramJobMedia(ctx, job, "display")
	if err != nil {
		return err
	}
	displayMedia, err = s.selectTelegramDisplayMediaForTenant(ctx, job, displayMedia)
	if err != nil {
		return err
	}
	verifyMedia, err := s.telegramJobMedia(ctx, job, "verify")
	if err != nil {
		return err
	}
	policy, err := s.telegramChannelSendPolicy(ctx, job)
	if err != nil {
		return err
	}
	if policy.AntiScanEnabled {
		for _, media := range append(displayMedia, verifyMedia...) {
			if media == nil || (!isTelegramImageMedia(media.MediaType) && !isTelegramVideoMedia(media.MediaType)) {
				continue
			}
			media.AntiScanEnabled = true
			media.AntiScanSeed = telegramProtectionSeed(job.Id, media.Id, media.Purpose)
			if isTelegramImageMedia(media.MediaType) {
				media.TgFileId = ""
				media.AssetHash = "anti-scan:" + telegramAntiScanCacheKey(media)
			}
		}
	}
	media := append(displayMedia, verifyMedia...)
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(telegramMediaPrepareConcurrency)
	for _, item := range media {
		item := item
		if item == nil || strings.TrimSpace(item.TgFileId) != "" {
			continue
		}
		group.Go(func() error {
			if _, cleanup, err := cachedTelegramMediaFile(groupCtx, item); err != nil {
				return err
			} else if cleanup != nil {
				cleanup()
			}
			if item.MediaType == "video" && (strings.TrimSpace(item.PosterUrl) != "" || strings.TrimSpace(item.PosterStoragePath) != "") {
				_, cleanup, posterErr := cachedTelegramVideoPosterFile(groupCtx, item)
				if cleanup != nil {
					cleanup()
				}
				return posterErr
			}
			return nil
		})
	}
	return group.Wait()
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

func (s *sSysPublish) sendTelegramJobLockedByChannel(ctx context.Context, jobId int64) error {
	job, locked, err := s.lockTelegramJob(ctx, jobId)
	if err != nil || !locked {
		return err
	}
	allowed, err := s.canSendTelegramJob(ctx, job)
	if err != nil {
		return err
	}
	if !allowed {
		s.appendTelegramJobLog(ctx, job, "publish", "skipped", "上架任务已下架或不可发布，跳过TG推送")
		return s.supersedeTelegramJobAndCompleteOperation(ctx, job)
	}
	allowed, err = s.prepareProfileChannelPublish(ctx, job)
	if err != nil {
		return s.handleTelegramJobError(ctx, job, err)
	}
	if !allowed {
		s.appendTelegramJobLog(ctx, job, "publish", "skipped", "频道已有同资料有效消息，跳过重复TG推送")
		return s.supersedeTelegramJobAndCompleteOperation(ctx, job)
	}
	if err = s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusPublishing); err != nil {
		return err
	}
	if recordErr := s.upsertPublishJobRecord(ctx, job, "sending", ""); recordErr != nil {
		g.Log().Warningf(ctx, "更新发布发送中记录失败 jobId:%d err:%+v", job.Id, recordErr)
	}
	s.appendTelegramJobLog(ctx, job, "publish", "started", s.telegramJobPublishMessage(ctx, job, "开始推送TG资料"))
	if err = s.sendLockedTelegramJob(ctx, job); err != nil {
		return s.handleTelegramJobError(ctx, job, err)
	}
	if err = s.completeTelegramJob(ctx, job); err != nil {
		return s.handleTelegramJobError(ctx, job, gerror.Wrap(err, "完成TG推送任务失败"))
	}
	return nil
}

func (s *sSysPublish) sendLockedTelegramJob(ctx context.Context, job telegramJobRecord) error {
	if job.SendPhase == telegramSendPhaseVerifyConfirmed {
		return nil
	}
	job.TargetChatId = normalizeTelegramChannelChatID(job.TargetChatId)
	if strings.TrimSpace(job.TargetChatId) == "" {
		return gerror.New("TG目标频道未配置")
	}
	botToken, err := s.telegramJobBotToken(ctx, job.BotId, job.TenantId)
	if err != nil {
		return err
	}
	bot, err := s.telegramBot(ctx, botToken)
	if err != nil {
		return err
	}
	caption, err := s.telegramJobCaption(ctx, job)
	if err != nil {
		return err
	}
	displayMedia, err := s.telegramJobMedia(ctx, job, "display")
	if err != nil {
		return err
	}
	displayMedia, err = s.selectTelegramDisplayMediaForTenant(ctx, job, displayMedia)
	if err != nil {
		return err
	}
	verifyMedia, err := s.telegramJobMedia(ctx, job, "verify")
	if err != nil {
		return err
	}
	caption, err = s.applyTelegramJobContentProtection(ctx, job, caption, displayMedia, verifyMedia)
	if err != nil {
		return err
	}
	if stillSending, err := s.telegramJobStillSending(ctx, job.Id); err != nil {
		return err
	} else if !stillSending {
		s.appendTelegramJobLog(ctx, job, "publish", "superseded", "TG展示资料发送前任务已废弃，停止推送")
		return errTelegramJobSuperseded
	}
	messages := make([]*telegramSentMessage, 0)
	if !telegramSendPhaseHasDisplay(job.SendPhase) {
		if err = s.updateTelegramJobSendPhase(ctx, job.Id, telegramSendPhaseDisplaySending); err != nil {
			return err
		}
		displayCaption := telegramCaptionWithJobMarker(caption, job.Id, "display")
		g.Log().Infof(ctx, "TG展示资料开始Bot发送 jobId:%d botId:%d chat:%s media:%s", job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(displayMedia))
		messages, err = s.sendTelegramDisplayPart(ctx, bot, job.TargetChatId, displayCaption, displayMedia)
		if err != nil && len(messages) == 0 && isTelegramMediaSizeLimitError(err) {
			g.Log().Warningf(ctx, "Bot展示媒体发送失败，命中大小降级条件 jobId:%d botId:%d chat:%s media:%s err:%+v", job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(displayMedia), err)
			messages, err = s.sendTelegramJobMediaByAccount(ctx, job, "display", displayCaption, displayMedia, err)
		}
		if err != nil {
			if errors.Is(err, errTelegramMediaFallbackQueued) {
				return nil
			}
			if !isTelegramMediaSizeLimitError(err) {
				logTelegramBotMediaSendFailure(ctx, job, "展示", displayMedia, err)
			}
			_ = s.cleanupTelegramSentMessages(ctx, bot, job.TargetChatId, messages, "展示资料分片推送失败")
			return gerror.Wrapf(err, "TG展示资料推送失败，job:%d，channel:%d，chat:%s", job.Id, job.ChannelId, job.TargetChatId)
		}
		if err = s.saveTelegramSentMessages(ctx, job, messages); err != nil {
			return telegramDeliveryUncertainError(err)
		}
		if err = s.updateTelegramMediaFileIds(ctx, messages); err != nil {
			g.Log().Warningf(ctx, "更新TG展示媒体file_id失败 job:%d err:%+v", job.Id, err)
		}
		if err = s.updateTelegramJobSendPhase(ctx, job.Id, telegramSendPhaseDisplayConfirmed); err != nil {
			return telegramDeliveryUncertainError(err)
		}
		job.SendPhase = telegramSendPhaseDisplayConfirmed
	}
	if stillSending, err := s.telegramJobStillSending(ctx, job.Id); err != nil {
		return err
	} else if !stillSending {
		s.appendTelegramJobLog(ctx, job, "publish", "superseded", "TG展示资料已发送但任务已废弃，停止推送验证资料")
		return errTelegramJobSuperseded
	}
	if err = s.updateTelegramJobSendPhase(ctx, job.Id, telegramSendPhaseVerifySending); err != nil {
		return err
	}
	verifyCaption := telegramCaptionWithJobMarker("", job.Id, "verify")
	g.Log().Infof(ctx, "TG验证资料开始Bot发送 jobId:%d botId:%d chat:%s media:%s", job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(verifyMedia))
	verifyMessages, err := s.sendTelegramVerifyPart(ctx, bot, job.TargetChatId, verifyCaption, verifyMedia)
	if err != nil && len(verifyMessages) == 0 && isTelegramMediaSizeLimitError(err) {
		g.Log().Warningf(ctx, "Bot验证媒体发送失败，命中大小降级条件 jobId:%d botId:%d chat:%s media:%s err:%+v", job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(verifyMedia), err)
		verifyMessages, err = s.sendTelegramJobMediaByAccount(ctx, job, "verify", verifyCaption, verifyMedia, err)
	}
	if err != nil {
		if errors.Is(err, errTelegramMediaFallbackQueued) {
			return nil
		}
		if !isTelegramMediaSizeLimitError(err) {
			logTelegramBotMediaSendFailure(ctx, job, "验证", verifyMedia, err)
		}
		if !isTelegramAmbiguousDeliveryError(err) {
			if len(verifyMessages) > 0 {
				if saveErr := s.saveTelegramSentMessages(ctx, job, verifyMessages); saveErr != nil {
					return telegramDeliveryUncertainError(saveErr)
				}
			}
			if cleanupErr := s.deleteTelegramMessageSetLockedByChannel(ctx, job, "验证资料推送失败，清理本次已发送消息"); cleanupErr != nil {
				return cleanupErr
			}
			if phaseErr := s.updateTelegramJobSendPhase(ctx, job.Id, ""); phaseErr != nil {
				return phaseErr
			}
		}
		return gerror.Wrapf(err, "TG验证资料推送失败，job:%d，channel:%d，chat:%s", job.Id, job.ChannelId, job.TargetChatId)
	}
	if err = s.saveTelegramSentMessages(ctx, job, verifyMessages); err != nil {
		return telegramDeliveryUncertainError(err)
	}
	if err = s.updateTelegramJobSendPhase(ctx, job.Id, telegramSendPhaseVerifyConfirmed); err != nil {
		return telegramDeliveryUncertainError(err)
	}
	return s.updateTelegramMediaFileIds(ctx, verifyMessages)
}

func logTelegramBotMediaSendFailure(ctx context.Context, job telegramJobRecord, purpose string, media []*telegramMediaItem, err error) {
	if isTelegramAccountBusyError(err) || isTelegramNetworkRetryError(err) {
		g.Log().Warningf(ctx, "Bot%s媒体发送遇到可恢复错误，等待队列重试 jobId:%d botId:%d chat:%s media:%s err:%v", purpose, job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(media), err)
		return
	}
	g.Log().Errorf(ctx, "Bot%s媒体发送失败，未进入协议号降级 jobId:%d botId:%d chat:%s media:%s err:%+v", purpose, job.Id, job.BotId, job.TargetChatId, telegramMediaDebugSummary(media), err)
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
	return s.sendTelegramMediaSet(ctx, bot, chatId, "verify", caption, media)
}

func (s *sSysPublish) sendTelegramJobMediaByAccount(ctx context.Context, job telegramJobRecord, purpose string, caption string, media []*telegramMediaItem, botErr error) ([]*telegramSentMessage, error) {
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
	_, err = collectorservice.AccountTasks().Submit(ctx, &collectorin.AccountTaskSubmit{
		TenantID: job.TenantId, AccountID: channel.TgAccountId,
		TaskType: collectorin.AccountTaskTypeMessageMediaFallback,
		TaskKey:  fmt.Sprintf("message-media-fallback:%d:%s", job.Id, purpose),
		Priority: collectorin.EventPriorityUrgent, MaxAttempts: 5,
	})
	if err != nil {
		return nil, gerror.Wrap(err, "提交协议号媒体降级任务失败")
	}
	s.appendTelegramJobLog(ctx, job, "account_fallback", "queued", "Bot媒体组超过请求大小限制，已提交账号服务常驻客户端发送")
	return nil, errTelegramMediaFallbackQueued
}

func (s *sSysPublish) handleTelegramJobError(ctx context.Context, job telegramJobRecord, err error) error {
	if errors.Is(err, errTelegramJobSuperseded) {
		return nil
	}
	if !isTelegramAccountBusyError(err) && isTelegramAmbiguousDeliveryError(err) {
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
	decision := telegramJobFailureNextState(err, job.RetryCount)
	result, updateErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).Where("status", "sending").
		Data(telegramJobFailureUpdateData(decision, gtime.Now())).Update()
	if updateErr != nil {
		return gerror.Wrap(updateErr, "更新TG失败任务状态失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	if recordErr := s.upsertPublishJobRecord(ctx, job, decision.Status, decision.Message); recordErr != nil {
		g.Log().Warningf(ctx, "更新发布失败记录失败 jobId:%d status:%s err:%+v", job.Id, decision.Status, recordErr)
	}
	s.appendTelegramJobLog(ctx, job, "publish", decision.Status, decision.Message)
	if decision.Status == "failed" && job.CollectEventId > 0 {
		_ = s.markCollectDispatchFailedByProfile(ctx, job.ProfileId, job.CollectEventId, decision.Message)
	}
	projectedStatus := sysin.PublishTaskStatusPending
	if decision.Status == "failed" {
		projectedStatus = sysin.PublishTaskStatusFailed
	}
	if stateErr := s.updateProfilePublishOperationState(ctx, job, projectedStatus); stateErr != nil {
		return stateErr
	}
	if decision.Status == "failed" {
		if wakeErr := s.wakeNextTelegramChannelJob(ctx, job); wakeErr != nil {
			g.Log().Warningf(ctx, "永久失败后唤醒频道下一条TG任务失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, wakeErr)
		}
	}
	return nil
}

func (s *sSysPublish) switchTelegramJobToNextBot(ctx context.Context, job telegramJobRecord, cause error) (bool, error) {
	nextBotId, err := s.nextTelegramChannelBotId(ctx, job)
	if err != nil {
		return false, err
	}
	if nextBotId <= 0 || nextBotId == job.BotId {
		return false, nil
	}
	now := gtime.Now()
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
		"bot_id":          nextBotId,
		"status":          "pending",
		"dispatch_status": tgDispatchStatusIdle,
		"retry_count":     job.RetryCount + 1,
		"next_retry_at":   nil,
		"error_message":   "",
		"updated_at":      now,
	}).Update()
	if err != nil {
		return false, gerror.Wrap(err, "切换备用BOT失败")
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
	sentAt := gtime.Now()
	data := telegramJobStateUpdateData("sent", 0, sentAt)
	data["error_message"] = ""
	data["sent_at"] = sentAt
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		Where("status", "sending").
		Data(data).Update()
	if err != nil {
		return gerror.Wrap(err, "更新TG任务状态失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		s.appendTelegramJobLog(ctx, job, "publish", "skipped", "TG发送完成时任务已不再发送中，跳过成功标记")
		return nil
	}
	job.SentAt = sentAt
	s.appendTelegramJobLog(ctx, job, "publish", "sent", s.telegramJobPublishMessage(ctx, job, "TG资料推送成功"))
	if recordErr := s.appendPublishSuccessRecord(ctx, job); recordErr != nil {
		g.Log().Warningf(ctx, "保存成功发布记录失败 jobId:%d err:%+v", job.Id, recordErr)
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
		if wakeErr := s.wakeNextTelegramChannelJob(ctx, job); wakeErr != nil {
			g.Log().Warningf(ctx, "唤醒频道下一条TG任务失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, wakeErr)
		}
		return nil
	}
	if wakeErr := s.wakeNextTelegramChannelJob(ctx, job); wakeErr != nil {
		g.Log().Warningf(ctx, "唤醒频道下一条TG任务失败 jobId:%d channelId:%d err:%+v", job.Id, job.ChannelId, wakeErr)
	}
	if job.CollectEventId > 0 {
		return s.markCollectDispatchSentByProfile(ctx, job.ProfileId, job.CollectEventId)
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
