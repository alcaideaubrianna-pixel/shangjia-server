package sys

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	publishconsts "hotgo/addons/youban_publish/consts"
	pdao "hotgo/addons/youban_publish/internal/dao"
	pdo "hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/model/input/sysin"
	idao "hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/utility/dbtime"
)

type publishCollectorDeliveryHandler struct {
	publish *sSysPublish
}

type publishCollectorAccountTaskHandler struct{ publish *sSysPublish }

type publishCollectorAccountMediaProvider struct{ publish *sSysPublish }

func (h *publishCollectorAccountTaskHandler) HandleAccountTaskFailure(ctx context.Context, task *collectorin.AccountTask, cause error) error {
	if h == nil || h.publish == nil || task == nil {
		return nil
	}
	var jobID int64
	var err error
	switch task.TaskType {
	case collectorin.AccountTaskTypeMessageReconcile:
		jobID, _, err = parseMessageReconcileTaskKey(task.TaskKey)
	case collectorin.AccountTaskTypeMessageMediaFallback:
		jobID, _, err = parseMessageMediaFallbackTaskKey(task.TaskKey)
	default:
		return nil
	}
	if err != nil {
		return err
	}
	job, err := h.publish.telegramJobById(ctx, jobID)
	if err != nil {
		return err
	}
	return h.publish.failTelegramMediaFallbackJob(ctx, job, cause)
}

func (h *publishCollectorAccountTaskHandler) HandleAccountTaskCompletion(ctx context.Context, task *collectorin.AccountTask, result *collectorin.AccountMediaDownloadResult) error {
	if h == nil || h.publish == nil || task == nil || result == nil || task.TaskType != collectorin.AccountTaskTypeMediaDownload {
		return nil
	}
	mediaID := parseCollectMediaTaskKey(task.TaskKey)
	if mediaID <= 0 {
		return nil
	}
	mediaCols := pdao.YoubanPublishCollectEventMedia.Columns()
	mediaRow, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Fields(mediaCols.EventId).
		Where(mediaCols.Id, mediaID).
		One()
	if err != nil {
		return gerror.Wrapf(err, "读取异步账号媒体归属事件失败 mediaId:%d", mediaID)
	}
	if mediaRow.IsEmpty() {
		return gerror.Newf("异步账号媒体记录不存在 mediaId:%d", mediaID)
	}
	eventID := mediaRow[mediaCols.EventId].Int64()
	if eventID <= 0 {
		return gerror.Newf("异步账号媒体缺少归属事件 mediaId:%d", mediaID)
	}
	if result.ErrorCode == "source_gone" {
		return h.publish.discardCollectEventGroup(ctx, eventID, firstNonEmpty(result.ErrorMessage, "TG原消息已删除或已无可用媒体"))
	}
	if strings.TrimSpace(result.StoragePath) == "" && strings.TrimSpace(result.FileURL) == "" {
		return gerror.Newf("异步账号媒体下载未返回存储地址 mediaId:%d", mediaID)
	}
	data := g.Map{
		"cache_status": "ready", "file_url": result.FileURL, "storage_path": result.StoragePath,
		"source_file_id": result.Media.FileID, "source_kind": result.Media.SourceKind,
		"source_media_id": result.Media.SourceMediaID, "source_access_hash": result.Media.SourceAccessHash,
		"source_file_reference": result.Media.SourceFileReference, "source_thumb_size": result.Media.SourceThumbSize,
		"source_mime_type": result.Media.SourceMimeType, "source_dc_id": result.Media.SourceDCID,
		"source_size": result.Media.SourceSize, "error_message": "", "updated_at": gtime.Now(),
	}
	if posterURL := strings.TrimSpace(result.PosterURL); posterURL != "" {
		data["poster_url"] = posterURL
	}
	if posterPath := normalizeStoredMediaPath(result.PosterPath); posterPath != "" {
		data["poster_storage_path"] = posterPath
	}
	if _, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).Where(mediaCols.Id, mediaID).Update(data); err != nil {
		return gerror.Wrapf(err, "异步账号媒体结果回写失败 mediaId:%d", mediaID)
	}
	pendingCount, err := pdao.YoubanPublishCollectEventMedia.Ctx(ctx).
		Where(mediaCols.EventId, eventID).
		WhereNot(mediaCols.CacheStatus, collectMediaCacheReady).
		Count()
	if err != nil {
		return gerror.Wrapf(err, "检查异步账号媒体组完成状态失败 mediaId:%d eventId:%d", mediaID, eventID)
	}
	if pendingCount > 0 {
		return nil
	}
	event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).WherePri(eventID).One()
	if err != nil {
		return gerror.Wrapf(err, "读取异步账号媒体事件失败 mediaId:%d eventId:%d", mediaID, eventID)
	}
	if event.IsEmpty() {
		return gerror.Newf("异步账号媒体事件不存在 mediaId:%d eventId:%d", mediaID, eventID)
	}
	if cache.Initialized() && g.Cfg().MustGet(ctx, "cache.adapter").String() == "redis" {
		key := fmt.Sprintf("%s%d", publishconsts.CollectMediaCompletionKeyPrefix, eventID)
		reserved, reserveErr := cache.Instance().SetIfNotExist(ctx, key, 1, 5*time.Minute)
		if reserveErr != nil {
			g.Log().Warningf(ctx, "采集媒体完成投递去重缓存不可用，放行任务 eventId:%d err:%+v", eventID, reserveErr)
		} else if !reserved {
			return nil
		}
	}
	// The original media-cache task can still hold its Asynq unique key when the
	// final account download finishes. Resume once after the whole group is ready.
	payload := collectMediaQueuePayloadFromEvent(event)
	payload.Reason = "account_completion"
	if _, err = h.publish.enqueueCollectMediaCacheDeferred(ctx, payload, 0); err != nil {
		if cache.Initialized() && g.Cfg().MustGet(ctx, "cache.adapter").String() == "redis" {
			key := fmt.Sprintf("%s%d", publishconsts.CollectMediaCompletionKeyPrefix, eventID)
			_, _ = cache.Instance().Remove(ctx, key)
		}
		return gerror.Wrapf(err, "异步账号媒体完成后重新投递事件失败 mediaId:%d eventId:%d", mediaID, eventID)
	}
	return nil
}

func parseCollectMediaTaskKey(key string) int64 {
	const prefix = "collect-media:"
	if !strings.HasPrefix(key, prefix) {
		return 0
	}
	id, _ := strconv.ParseInt(strings.TrimPrefix(key, prefix), 10, 64)
	return id
}

func (h *publishCollectorAccountTaskHandler) HandleAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (*collectorin.AccountMediaDownloadResult, error) {
	if h == nil || h.publish == nil || task == nil {
		return nil, gerror.New("Telegram账号任务处理参数无效")
	}
	switch task.TaskType {
	case collectorin.AccountTaskTypeChannelBotAttach:
		const prefix = "channel-bot-attach:"
		encoded := strings.TrimPrefix(task.TaskKey, prefix)
		if !strings.HasPrefix(task.TaskKey, prefix) {
			return nil, gerror.New("频道Bot管理任务参数无效")
		}
		body, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, gerror.Wrap(err, "解析频道Bot管理任务失败")
		}
		var payload channelBotAttachTaskPayload
		if err = json.Unmarshal(body, &payload); err != nil {
			return nil, gerror.Wrap(err, "解析频道Bot管理参数失败")
		}
		if payload.TenantID != task.TenantID || payload.TGAccountID != task.AccountID || len(payload.BotIDs) == 0 {
			return nil, gerror.New("频道Bot管理任务归属不一致")
		}
		bots, err := h.publish.channelCheckBots(ctx, payload.BotIDs, payload.TenantID)
		if err != nil {
			return nil, err
		}
		return nil, h.publish.attachChannelBotsWithClient(ctx, client, payload.TenantID, payload.TGAccountID, &sysin.ChannelCacheModel{ChannelId: payload.ChannelID, AccessHash: payload.AccessHash}, bots)
	case collectorin.AccountTaskTypeHistoryPage:
		return nil, h.publish.handleCollectHistoryAccountTask(ctx, client, task.HistoryTaskID)
	case collectorin.AccountTaskTypeMaterialImportHistoryPage:
		const prefix = "material-import:"
		parts := strings.Split(strings.TrimPrefix(task.TaskKey, prefix), ":")
		taskID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil || taskID <= 0 || !strings.HasPrefix(task.TaskKey, prefix) {
			return nil, gerror.New("资料导入账号任务参数无效")
		}
		importTask, err := h.publish.materialImportTaskByPrimary(ctx, taskID)
		if err != nil {
			return nil, err
		}
		cache, err := h.publish.tgChannelCacheByChannelId(ctx, importTask.TenantId, importTask.TgAccountId, importTask.SourceChatId)
		if err != nil {
			return nil, err
		}
		peer, err := collectInputPeerChannel(cache)
		if err != nil {
			return nil, err
		}
		if _, err = client.Self(ctx); err != nil {
			return nil, err
		}
		return nil, h.publish.pullMaterialImportPages(ctx, client, importTask, peer, cache)
	case collectorin.AccountTaskTypeDialogCacheRefresh:
		return nil, h.publish.handleDialogCacheRefreshAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeMessagePushInline:
		return nil, h.publish.handleMessagePushInlineAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeMessageReconcile:
		return nil, h.publish.handleMessageReconcileAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeMessageMediaFallback:
		return nil, h.publish.handleMessageMediaFallbackAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeMessageDeleteFallback:
		return nil, h.publish.handleMessageDeleteFallbackAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeManagedBotUsernameCheck:
		return h.publish.handleManagedBotUsernameCheckAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeManagedBotCreate:
		return h.publish.handleManagedBotCreateAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeChannelMemberSync:
		return nil, h.publish.handleChannelMemberSyncAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeTgAccountRefresh:
		return nil, h.publish.handleTgAccountRefreshTask(ctx, client, task)
	case collectorin.AccountTaskTypeMessageRepair:
		return nil, h.publish.handleMessageRepairAccountTask(ctx, client, task)
	case collectorin.AccountTaskTypeMessageRepairScan:
		return h.publish.handleMessageRepairScanAccountTask(ctx, client, task)
	default:
		return nil, gerror.Newf("不支持的Telegram账号任务类型：%s", task.TaskType)
	}
}

func (s *sSysPublish) handleManagedBotUsernameCheckAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (*collectorin.AccountMediaDownloadResult, error) {
	accountID, username, err := parseManagedBotUsernameCheckTaskKey(task.TaskKey)
	if err != nil {
		return nil, err
	}
	if accountID != task.AccountID {
		return nil, gerror.New("Managed Bot用户名检测任务账号不一致")
	}
	available, err := client.API().BotsCheckUsername(ctx, username)
	if err != nil {
		return nil, gerror.Wrap(err, "检查Managed Bot用户名失败")
	}
	resultCode := managedBotUsernameOccupiedResult
	if available {
		resultCode = managedBotUsernameAvailableResult
	}
	return &collectorin.AccountMediaDownloadResult{ErrorCode: resultCode}, nil
}

func (s *sSysPublish) handleManagedBotCreateAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (*collectorin.AccountMediaDownloadResult, error) {
	payload, err := parseManagedBotCreateTaskKey(task.TaskKey)
	if err != nil {
		return nil, err
	}
	if payload.AccountID != task.AccountID {
		return nil, gerror.New("Managed Bot创建任务账号不一致")
	}
	manager, err := resolveManagedBotUser(ctx, client, payload.ManagerUsername)
	if err != nil {
		return nil, err
	}
	if !manager.BotCanManageBots {
		return nil, gerror.New("官方Bot尚未开启Bot Management Mode")
	}
	available, err := client.API().BotsCheckUsername(ctx, payload.Username)
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, gerror.New("该 Bot 用户名已被占用")
	}
	createdUser, err := client.API().BotsCreateBot(ctx, &tg.BotsCreateBotRequest{
		Name: payload.Name, Username: payload.Username,
		ManagerID: &tg.InputUser{UserID: manager.ID, AccessHash: manager.AccessHash},
	})
	if err != nil {
		return nil, err
	}
	created, ok := createdUser.(*tg.User)
	if !ok || created == nil || created.ID <= 0 || created.AccessHash == 0 {
		return nil, gerror.New("Telegram未返回新Bot的有效身份信息")
	}
	return &collectorin.AccountMediaDownloadResult{
		FileURL: firstNonEmpty(created.Username, payload.Username),
		Media:   collectorin.CollectorMediaItem{SourceMediaID: created.ID, SourceAccessHash: created.AccessHash},
	}, nil
}

func (s *sSysPublish) handleMessageMediaFallbackAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (err error) {
	startedAt := time.Now()
	stageStartedAt := startedAt
	var jobID int64
	stage := func(name string, fields ...interface{}) {
		duration := time.Since(stageStartedAt)
		g.Log().Infof(ctx, "协议号媒体降级阶段完成 taskId:%d jobId:%d tgAccountId:%d stage:%s stageDuration:%s totalDuration:%s fields:%v", task.ID, jobID, task.AccountID, name, duration.Round(time.Millisecond), time.Since(startedAt).Round(time.Millisecond), fields)
		stageStartedAt = time.Now()
	}
	defer func() {
		if err := ctx.Err(); err != nil {
			g.Log().Warningf(ctx, "协议号媒体降级任务上下文结束 taskId:%d tgAccountId:%d totalDuration:%s err:%+v", task.ID, task.AccountID, time.Since(startedAt).Round(time.Millisecond), err)
		}
	}()
	jobID, purpose, err := parseMessageMediaFallbackTaskKey(task.TaskKey)
	if err != nil {
		return err
	}
	job, err := s.telegramJobById(ctx, jobID)
	if err != nil {
		return err
	}
	stage("读取任务与Job", "purpose", purpose, "jobStatus", job.Status)
	if job.Status == telegramJobStatusSuperseded {
		return nil
	}
	if job.Status != "sending" {
		messageCount, countErr := s.telegramJobSentMessageCount(ctx, job.Id, purpose)
		if countErr != nil {
			return countErr
		}
		if mediaFallbackTaskCanSkip(job.Status, messageCount) {
			g.Log().Infof(ctx, "协议号媒体降级任务幂等完成 taskId:%d jobId:%d tgAccountId:%d purpose:%s messageCount:%d", task.ID, job.Id, task.AccountID, purpose, messageCount)
			return nil
		}
		// Reconciliation may conservatively move a Bot-failed job to unknown
		// before the account fallback starts. With no messages for this phase,
		// it is safe to claim the job for the fallback instead of abandoning it.
		if job.Status == "unknown" && messageCount == 0 {
			transitioned, updateErr := s.transitionTelegramJob(ctx, telegramJobTransition{
				JobId: job.Id,
				From:  []string{telegramJobStatusConfirming},
				To:    telegramJobStatusSending,
			})
			if updateErr != nil {
				return gerror.Wrap(updateErr, "恢复协议号媒体降级任务状态失败")
			}
			if !transitioned {
				return gerror.New("协议号媒体降级任务正在被其他发送流程处理")
			}
			job.Status = "sending"
			g.Log().Infof(ctx, "协议号媒体降级任务从未知状态恢复发送 taskId:%d jobId:%d purpose:%s", task.ID, job.Id, purpose)
		}
		if job.Status != "sending" {
			stateErr := gerror.Newf("协议号媒体降级任务状态异常，拒绝静默完成：jobId=%d status=%s sendPhase=%s purpose=%s messageCount=%d", job.Id, job.Status, job.SendPhase, purpose, messageCount)
			g.Log().Warningf(ctx, "协议号媒体降级任务拒绝静默完成 taskId:%d tgAccountId:%d err:%+v", task.ID, task.AccountID, stateErr)
			s.appendTelegramJobLog(ctx, job, "account_fallback", "failed", stateErr.Error())
			return stateErr
		}
	}
	bounded, cancelBudget, budgetErr := s.telegramSendBudgetContext(ctx, job)
	if budgetErr != nil {
		return budgetErr
	}
	defer cancelBudget()
	ctx = bounded
	g.Log().Infof(ctx, "协议号媒体降级任务开始执行 taskId:%d jobId:%d tgAccountId:%d purpose:%s attempt:%d/%d", task.ID, job.Id, task.AccountID, purpose, task.AttemptCount, task.MaxAttempts)
	channel, err := s.messagePushChannelFromJob(ctx, job)
	if err != nil {
		return err
	}
	if task.AccountID != channel.TgAccountId {
		return gerror.New("媒体降级发送账号任务与目标频道账号不一致")
	}
	stage("读取频道")
	media, err := s.telegramJobMedia(ctx, job, purpose)
	if err != nil {
		return err
	}
	if purpose == "display" {
		media, err = s.selectTelegramDisplayMediaForTenant(ctx, job, media)
		if err != nil {
			return err
		}
	}
	stage("读取并选择媒体", "mediaCount", len(media))
	var verifyMedia []*telegramMediaItem
	if purpose == "display" {
		verifyMedia, err = s.telegramJobMedia(ctx, job, "verify")
		if err != nil {
			return err
		}
	}
	stage("读取验证媒体", "verifyMediaCount", len(verifyMedia))
	caption := ""
	if purpose == "display" {
		caption, err = s.telegramJobCaption(ctx, job)
		if err != nil {
			return err
		}
	}
	caption, err = s.applyTelegramJobContentProtection(ctx, job, caption, media, verifyMedia)
	if err != nil {
		return err
	}
	stage("媒体内容保护与缩略图准备", "mediaCount", len(media), "verifyMediaCount", len(verifyMedia))
	var messages []*telegramSentMessage
	phaseConfirmed := accountFallbackPhaseConfirmed(job.SendPhase, purpose)
	if !phaseConfirmed {
		messages, err = s.sendAccountFallbackPhase(ctx, client, channel, job, task.AccountID, purpose, caption, media)
		if err != nil {
			return err
		}
		stage("发送并确认媒体", "purpose", purpose, "messageCount", len(messages))
	} else {
		stage("跳过已确认媒体阶段", "purpose", purpose, "sendPhase", job.SendPhase)
	}
	if purpose == "display" {
		if len(verifyMedia) == 0 {
			g.Log().Infof(ctx, "协议号媒体降级任务发送成功 taskId:%d jobId:%d tgAccountId:%d displayMessages:%d verifyMessages:0", task.ID, job.Id, task.AccountID, len(messages))
			return s.finalizeTelegramMediaDelivery(ctx, job, false, true)
		}
		var verifyMessages []*telegramSentMessage
		if job.SendPhase != telegramSendPhaseVerifyConfirmed {
			verifyMessages, err = s.sendAccountFallbackPhase(ctx, client, channel, job, task.AccountID, "verify", "", verifyMedia)
			if err != nil {
				return err
			}
			stage("发送并确认验证媒体", "messageCount", len(verifyMessages))
		}
		g.Log().Infof(ctx, "协议号媒体降级任务发送成功 taskId:%d jobId:%d tgAccountId:%d displayMessages:%d verifyMessages:%d", task.ID, job.Id, task.AccountID, len(messages), len(verifyMessages))
		return s.finalizeTelegramMediaDelivery(ctx, job, true, true)
	}
	g.Log().Infof(ctx, "协议号验证媒体降级任务发送成功 taskId:%d jobId:%d tgAccountId:%d verifyMessages:%d", task.ID, job.Id, task.AccountID, len(messages))
	return s.finalizeTelegramMediaDelivery(ctx, job, true, true)
}

func accountFallbackPhaseConfirmed(sendPhase, purpose string) bool {
	return purpose == "display" && telegramSendPhaseHasDisplay(sendPhase) ||
		purpose == "verify" && sendPhase == telegramSendPhaseVerifyConfirmed
}

func (s *sSysPublish) sendAccountFallbackPhase(ctx context.Context, client *telegram.Client, channel *messagePushChannel, job telegramJobRecord, accountID int64, purpose, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	attempt, messages, err := s.prepareAccountFallbackAttempt(ctx, client, channel, job, purpose, len(media))
	if err != nil {
		return nil, err
	}
	if len(messages) == 0 {
		peer, peerErr := messagePushInputPeer(channel)
		if peerErr != nil {
			return nil, peerErr
		}
		caption = telegramCaptionWithJobMarker(caption, job.Id, purpose) + telegramAttemptMarker(attempt.AttemptToken)
		messages, err = s.sendMessageTemplateWithTgClient(ctx, client, peer, caption, media, nil, accountID, "")
		if err != nil {
			if cleanupErr := s.cleanupPartialAccountMediaSend(ctx, client, job, channel, purpose, messages, err); cleanupErr != nil {
				return nil, cleanupErr
			}
			return nil, gerror.Wrap(err, "协议号媒体降级发送失败")
		}
	}
	bindAccountFallbackMessagesToMedia(messages, media)
	if err = validateTelegramMediaSendResult(purpose, media, messages); err != nil {
		s.appendTelegramJobLog(ctx, job, "account", "incomplete", err.Error())
		return nil, err
	}
	if err = s.confirmTelegramAPISuccess(ctx, job, attempt, messages); err != nil {
		return nil, telegramDeliveryUncertainError(err)
	}
	if err = s.updateTelegramMediaFileIds(ctx, messages); err != nil {
		g.Log().Warningf(ctx, "协议号媒体file_id更新失败 jobId:%d purpose:%s err:%+v", job.Id, purpose, err)
	}
	s.appendTelegramJobLog(ctx, job, "account", "confirmed", fmt.Sprintf("媒体阶段确认完成 purpose:%s expected:%d confirmed:%d", purpose, len(media), len(messages)))
	return messages, nil
}

func (s *sSysPublish) prepareAccountFallbackAttempt(ctx context.Context, client *telegram.Client, channel *messagePushChannel, job telegramJobRecord, purpose string, expectedCount int) (telegramDeliveryAttempt, []*telegramSentMessage, error) {
	attempt, err := s.activeTelegramDeliveryAttempt(ctx, job.Id, purpose)
	if err != nil {
		return attempt, nil, err
	}
	if attempt.Id <= 0 {
		attempt, err = s.beginTelegramDeliveryAttempt(ctx, job, purpose, expectedCount)
		if err != nil {
			return attempt, nil, err
		}
	}
	messages, err := findTelegramAttemptMessages(ctx, client, channel, attempt)
	if err != nil {
		return attempt, nil, err
	}
	if len(messages) > 0 {
		g.Log().Infof(ctx, "协议号发送前对账命中已有消息，跳过重复发送 jobId:%d attemptId:%d purpose:%s messages:%d", job.Id, attempt.Id, purpose, len(messages))
	}
	return attempt, messages, nil
}

func bindAccountFallbackMessagesToMedia(messages []*telegramSentMessage, media []*telegramMediaItem) {
	for index, message := range messages {
		if message == nil || message.MediaId > 0 || index >= len(media) || media[index] == nil {
			continue
		}
		message.MediaId = media[index].Id
	}
}

func parseMessageMediaFallbackTaskKey(taskKey string) (int64, string, error) {
	const prefix = "message-media-fallback:"
	parts := strings.Split(strings.TrimPrefix(taskKey, prefix), ":")
	if !strings.HasPrefix(taskKey, prefix) || len(parts) != 2 {
		return 0, "", gerror.New("媒体降级发送账号任务参数无效")
	}
	jobID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || jobID <= 0 || (parts[1] != "display" && parts[1] != "verify") {
		return 0, "", gerror.New("媒体降级发送账号任务参数无效")
	}
	return jobID, parts[1], nil
}

func validateTelegramMediaSendResult(purpose string, media []*telegramMediaItem, messages []*telegramSentMessage) error {
	if len(media) == 0 {
		return nil
	}
	mediaIDs := make(map[int64]struct{}, len(media))
	for _, item := range media {
		if item != nil && item.Id > 0 {
			mediaIDs[item.Id] = struct{}{}
		}
	}
	sentIDs := make(map[int64]struct{}, len(messages))
	for _, item := range messages {
		if item != nil && item.MessageId > 0 && item.MediaId > 0 {
			sentIDs[item.MediaId] = struct{}{}
		}
	}
	for mediaID := range mediaIDs {
		if _, ok := sentIDs[mediaID]; !ok {
			return gerror.Newf("Telegram%s媒体发送结果不完整：期望%d个媒体，确认%d个，缺少mediaId=%d", purpose, len(mediaIDs), len(sentIDs), mediaID)
		}
	}
	if len(mediaIDs) == 0 && len(messages) < len(media) {
		return gerror.Newf("Telegram%s媒体发送结果不完整：期望%d个媒体，确认%d条消息", purpose, len(media), len(messages))
	}
	return nil
}

func mediaFallbackTaskCanSkip(jobStatus string, sentMessageCount int) bool {
	return jobStatus == "sent" && sentMessageCount > 0
}

func (s *sSysPublish) telegramJobSentMessageCount(ctx context.Context, jobID int64, purpose string) (int, error) {
	count, err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Where("job_id", jobID).
		Where("purpose", purpose).
		Where("status", "sent").
		Count()
	if err != nil {
		return 0, gerror.Wrap(err, "读取协议号媒体降级消息数量失败")
	}
	return count, nil
}

func (s *sSysPublish) handleMessageReconcileAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (err error) {
	jobID, attemptID, err := parseMessageReconcileTaskKey(task.TaskKey)
	if err != nil {
		return gerror.New("消息对账账号任务参数无效")
	}
	job, err := s.telegramJobById(ctx, jobID)
	if err != nil {
		return err
	}
	attempt, err := s.telegramAttemptById(ctx, attemptID)
	if err != nil || attempt.Id <= 0 || attempt.JobId != job.Id || (attempt.Phase != "display" && attempt.Phase != "verify") {
		return gerror.New("Telegram消息对账Attempt不存在或不匹配")
	}
	if job.Status == telegramJobStatusSending &&
		job.SendPhase == telegramSendPhaseVerifyConfirmed &&
		attempt.Status == telegramAttemptStatusConfirmed {
		return s.finalizeTelegramMediaDelivery(ctx, job, true, true)
	}
	if job.Status != telegramJobStatusConfirming {
		return nil
	}
	channel, err := s.messagePushChannelFromJob(ctx, job)
	if err != nil {
		return err
	}
	if task.AccountID != channel.TgAccountId {
		return gerror.New("Telegram消息对账账号与目标频道不一致")
	}
	messages, err := findTelegramAttemptMessages(ctx, client, channel, attempt)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		if attempt.ResponseReceived == 0 && attempt.CreatedAt != nil && time.Since(dbtime.FromTimestamp(attempt.CreatedAt).Time) >= 5*time.Minute {
			changed, err := s.transitionTelegramJob(ctx, telegramJobTransition{
				JobId: job.Id, From: []string{telegramJobStatusConfirming}, To: telegramJobStatusSending,
			})
			if err != nil {
				return err
			}
			if !changed {
				return nil
			}
			g.Log().Warningf(ctx, "TG结果不明确超过5分钟，按补偿策略切换协议号 jobId:%d attemptId:%d phase:%s", job.Id, attempt.Id, attempt.Phase)
			return s.submitTelegramMediaFallback(ctx, job, channel.TgAccountId, attempt.Phase)
		}
		if reconcileErr := telegramMissingReconcileMessagesError(attempt.Phase, accountTaskFinalAttempt(task)); !errors.Is(reconcileErr, errTelegramMediaFallbackUnavailable) {
			return reconcileErr
		}
		// An ambiguous Bot API call cannot be proved unsuccessful merely because
		// the account history window no longer contains its marker. Blindly
		// falling back here can duplicate an already delivered media group and
		// break channel ordering. Stop this job and release the channel instead;
		// operators may explicitly republish it after inspecting Telegram.
		return s.failTelegramMediaFallbackJob(ctx, job, telegramMissingReconcileMessagesError(attempt.Phase, true))
	}
	if err = s.saveTelegramSentMessages(ctx, job, messages); err != nil {
		return err
	}
	if err = s.confirmTelegramDeliveryAttempt(ctx, attempt, messages, false); err != nil {
		return err
	}
	confirmedPhase := telegramSendPhaseDisplayConfirmed
	nextStatus := telegramJobStatusRetryWait
	if attempt.Phase == "verify" {
		confirmedPhase = telegramSendPhaseVerifyConfirmed
		nextStatus = telegramJobStatusSending
	}
	transitioned, err := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId: job.Id, From: []string{telegramJobStatusConfirming}, To: nextStatus,
		Additional: g.Map{"send_phase": confirmedPhase, "error_message": ""},
	})
	if err != nil {
		return err
	}
	if !transitioned {
		// A late Webhook may have advanced this job concurrently. The persisted
		// messages and confirmed Attempt make this reconciliation idempotent.
		return nil
	}
	job.Status = nextStatus
	job.SendPhase = confirmedPhase
	if attempt.Phase == "display" {
		return s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false)
	}
	return s.finalizeTelegramMediaDelivery(ctx, job, true, true)
}

func telegramMissingReconcileMessagesError(phase string, finalAttempt bool) error {
	if !finalAttempt {
		return gerror.New("协议号暂未找到Bot发送消息，等待后重试对账")
	}
	return gerror.Wrapf(errTelegramMediaFallbackUnavailable,
		"协议号最终未找到Bot%s消息，结果仍不确定，已禁止自动补发", telegramMediaPurposeLabel(phase))
}

func telegramMediaPurposeLabel(purpose string) string {
	if purpose == "verify" {
		return "验证视频"
	}
	return "展示资料"
}

func parseMessageReconcileTaskKey(taskKey string) (int64, int64, error) {
	const prefix = "message-reconcile:"
	parts := strings.Split(strings.TrimPrefix(taskKey, prefix), ":")
	if !strings.HasPrefix(taskKey, prefix) || len(parts) != 2 {
		return 0, 0, gerror.New("消息对账账号任务参数无效")
	}
	jobID, jobErr := strconv.ParseInt(parts[0], 10, 64)
	attemptID, attemptErr := strconv.ParseInt(parts[1], 10, 64)
	if jobErr != nil || attemptErr != nil || jobID <= 0 || attemptID <= 0 {
		return 0, 0, gerror.New("消息对账账号任务参数无效")
	}
	return jobID, attemptID, nil
}

func findTelegramAttemptMessages(ctx context.Context, client *telegram.Client, channel *messagePushChannel, attempt telegramDeliveryAttempt) ([]*telegramSentMessage, error) {
	peer, err := messagePushInputPeer(channel)
	if err != nil {
		return nil, err
	}
	res, err := client.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer: peer, Limit: 100})
	if err != nil {
		return nil, gerror.Wrap(err, "协议号读取频道最近消息失败")
	}
	return selectTelegramAttemptMessages(tgHistoryMessages(res), attempt)
}

func selectTelegramAttemptMessages(history []*tg.Message, attempt telegramDeliveryAttempt) ([]*telegramSentMessage, error) {
	marker := telegramAttemptMarker(attempt.AttemptToken)
	var groupedID int64
	for _, message := range history {
		if message != nil && strings.Contains(message.Message, marker) {
			groupedID, _ = message.GetGroupedID()
			if groupedID == 0 {
				if attempt.ExpectedCount > 1 {
					return nil, gerror.Newf("协议号找到的验证消息不完整：expected=%d actual=1", attempt.ExpectedCount)
				}
				return []*telegramSentMessage{{MessageId: int64(message.ID), Purpose: attempt.Phase}}, nil
			}
			break
		}
	}
	if groupedID == 0 {
		return nil, nil
	}
	messages := make([]*telegramSentMessage, 0, attempt.ExpectedCount)
	for _, message := range history {
		if message == nil {
			continue
		}
		id, ok := message.GetGroupedID()
		if ok && id == groupedID {
			messages = append(messages, &telegramSentMessage{MessageId: int64(message.ID), MediaGroupId: strconv.FormatInt(groupedID, 10), Purpose: attempt.Phase})
		}
	}
	if len(messages) != attempt.ExpectedCount {
		return nil, gerror.Newf("协议号找到的验证媒体组不完整：expected=%d actual=%d", attempt.ExpectedCount, len(messages))
	}
	return messages, nil
}

func (s *sSysPublish) handleMessagePushInlineAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) (err error) {
	startedAt := time.Now()
	jobId, mode, err := parseMessagePushAccountTaskKey(task.TaskKey)
	if err != nil {
		return err
	}
	accountOnly := mode == "account"
	job, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	defer func() {
		if err == nil || !accountTaskFinalAttempt(task) {
			return
		}
		message := "Telegram消息推送连续失败，账号任务已停止：" + err.Error()
		s.failMessagePushJob(ctx, job, message)
		g.Log().Errorf(ctx, "消息推送账号任务最终失败 taskId:%d jobId:%d tgAccountId:%d attempt:%d/%d err:%+v", task.ID, job.Id, task.AccountID, task.AttemptCount, task.MaxAttempts, err)
	}()
	if job.Status == sysin.MessagePushStatusSent {
		return nil
	}
	if accountOnly {
		s.appendTelegramJobLog(ctx, job, "account_send", "started", fmt.Sprintf("开始协议号发送 accountTaskId:%d tgAccountId:%d", task.ID, task.AccountID))
	} else {
		s.appendTelegramJobLog(ctx, job, "inline_send", "started", fmt.Sprintf("开始Inline发布 accountTaskId:%d tgAccountId:%d", task.ID, task.AccountID))
	}
	templateId, err := messagePushTemplateIdFromOperationNo(job.OperationNo)
	if err != nil {
		return err
	}
	template, err := s.messageTemplate(ctx, templateId, job.TenantId)
	if err != nil {
		return err
	}
	channel, err := s.messagePushChannelFromJob(ctx, job)
	if err != nil {
		return err
	}
	if task.AccountID != channel.TgAccountId {
		return gerror.New("Inline账号任务与目标频道账号不一致")
	}
	if accountOnly {
		s.appendTelegramJobLog(ctx, job, "account_send", "started", fmt.Sprintf("开始执行协议号降级 accountTaskId:%d tgAccountId:%d", task.ID, task.AccountID))
		return s.sendMessageTemplateByAccountClient(ctx, client, channel, job, template)
	}
	if validationErr := validateInlinePublishTemplate(template); validationErr != nil {
		g.Log().Errorf(ctx, "Inline发布协议校验失败，进入Bot与协议号降级链路 jobId:%d serial:%s err:%+v", job.Id, template.SerialNo, validationErr)
		s.appendTelegramJobLog(ctx, job, "inline_send", "skipped", "Inline协议校验失败，进入降级链路："+validationErr.Error())
		return s.fallbackMessagePushInline(ctx, client, channel, job, template, validationErr)
	}
	peer, err := messagePushInputPeer(channel)
	if err != nil {
		return err
	}
	botUsername, err := inlineBotUsername(ctx)
	if err != nil {
		return err
	}
	messages, err := sendInlineTemplateWithClient(ctx, client, peer, botUsername, template.SerialNo)
	if err != nil {
		inlineErr := gerror.Wrapf(err, "Inline推送失败 serial:%s", template.SerialNo)
		s.appendTelegramJobLog(ctx, job, "inline_send", "failed", fmt.Sprintf("Inline请求失败 duration:%s err:%v", time.Since(startedAt), inlineErr))
		return s.fallbackMessagePushInline(ctx, client, channel, job, template, inlineErr)
	}
	if len(messages) == 0 {
		return s.fallbackMessagePushInline(ctx, client, channel, job, template, gerror.New("Inline推送未返回Telegram消息记录"))
	}
	if err = s.completeMessagePushJob(ctx, job, messages, "更新Inline消息推送任务状态失败"); err != nil {
		return err
	}
	s.appendTelegramJobLog(ctx, job, "inline_send", sysin.MessagePushStatusSent, fmt.Sprintf("账号服务Inline机器人消息模板推送成功 duration:%s", time.Since(startedAt)))
	return nil
}

func parseMessagePushAccountTaskKey(taskKey string) (jobId int64, mode string, err error) {
	const prefix = "message-push-inline:"
	if !strings.HasPrefix(taskKey, prefix) {
		return 0, "", gerror.New("消息推送账号任务参数无效")
	}
	parts := strings.Split(strings.TrimPrefix(taskKey, prefix), ":")
	if len(parts) == 0 || len(parts) > 2 {
		return 0, "", gerror.New("消息推送账号任务参数无效")
	}
	jobId, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil || jobId <= 0 {
		return 0, "", gerror.New("消息推送账号任务Job ID无效")
	}
	mode = "inline"
	if len(parts) == 2 {
		mode = parts[1]
	}
	if mode != "inline" && mode != "account" {
		return 0, "", gerror.New("消息推送账号任务模式无效")
	}
	return jobId, mode, nil
}

func accountTaskFinalAttempt(task *collectorin.AccountTask) bool {
	return task != nil && task.MaxAttempts > 0 && task.AttemptCount >= task.MaxAttempts
}

func validateInlinePublishTemplate(template *sysin.MessageTemplateModel) error {
	if template == nil {
		return gerror.New("Inline模板为空")
	}
	if strings.TrimSpace(template.SerialNo) == "" {
		return gerror.New("Inline模板编号为空")
	}
	media := inlineDisplayMedia(template)
	if len(media) > 1 {
		return gerror.New("仅支持单个媒体，当前媒体数量超过1")
	}
	if len(media) == 1 {
		item := media[0]
		if item == nil || !strings.EqualFold(strings.TrimSpace(item.MediaType), "image") {
			return gerror.New("仅支持单张图片，视频或媒体组改用协议号")
		}
		if strings.TrimSpace(item.TgFileId) == "" && !isInlineHTTPSURL(firstNonEmpty(item.FileUrl, item.StoragePath)) {
			return gerror.New("图片缺少有效Telegram文件ID或公网HTTPS地址")
		}
		if len([]rune(template.Text)) > 1024 {
			return gerror.New("图片文案超过 Telegram BOT 限制，已自动使用协议号发送")
		}
	} else if len([]rune(template.Text)) > 4096 {
		return gerror.New("Inline文案超过Telegram允许的4096字符")
	}
	if strings.TrimSpace(template.ButtonConfig) != "" {
		var config sysin.MessageTemplateButtonConfig
		if err := json.Unmarshal([]byte(template.ButtonConfig), &config); err != nil {
			return gerror.Wrap(err, "Inline按钮配置不是有效JSON")
		}
		if config.Mode != "inline" {
			return gerror.New("Inline按钮配置模式必须是inline")
		}
		for rowIndex, row := range config.Rows {
			for buttonIndex, button := range row {
				if strings.TrimSpace(button.Text) == "" {
					return gerror.Newf("Inline按钮文本为空，位置：%d-%d", rowIndex+1, buttonIndex+1)
				}
				if !isInlineHTTPSURL(inlinePublishButtonURL(button.URL)) {
					return gerror.Newf("Inline按钮链接不是有效HTTPS地址，位置：%d-%d，原值：%s", rowIndex+1, buttonIndex+1, button.URL)
				}
			}
		}
	}
	return nil
}

func isInlineHTTPSURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

func inlinePublishButtonURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "@") && len(raw) > 1 {
		return "https://t.me/" + strings.TrimPrefix(raw, "@")
	}
	return raw
}

func (s *sSysPublish) fallbackMessagePushInline(ctx context.Context, client *telegram.Client, channel *messagePushChannel, job telegramJobRecord, template *sysin.MessageTemplateModel, inlineErr error) error {
	s.appendTelegramJobLog(ctx, job, "inline_send", "fallback", "Inline推送失败，改用官方Bot上传："+inlineErr.Error())
	if !s.botCanAccessChat(ctx, job.TargetChatId) {
		s.appendTelegramJobLog(ctx, job, "bot_upload", "skipped", "官方Bot未加入目标群，跳过Bot发送，改用协议号发送")
		return s.sendMessageTemplateByAccountClient(ctx, client, channel, job, template)
	}
	fallbackMessages, fallbackErr := s.sendMessageTemplateByBot(ctx, job, template, messageTemplateTelegramMedia(template))
	if fallbackErr == nil {
		if completeErr := s.completeMessagePushJob(ctx, job, fallbackMessages, "更新Bot降级推送任务状态失败"); completeErr != nil {
			return completeErr
		}
		s.appendTelegramJobLog(ctx, job, "bot_upload", sysin.MessagePushStatusSent, "Inline推送超时，已由官方Bot降级发送成功")
		return nil
	}
	s.appendTelegramJobLog(ctx, job, "bot_upload", "failed", "官方Bot发送失败，改用协议号发送："+fallbackErr.Error())
	return s.sendMessageTemplateByAccountClient(ctx, client, channel, job, template)
}

func (s *sSysPublish) botCanAccessChat(ctx context.Context, chatID string) bool {
	cacheKey := officialBotChatUnavailableCacheKey(chatID)
	if value, cacheErr := cache.Instance().Get(ctx, cacheKey); cacheErr == nil && !value.IsNil() {
		return false
	}
	token, err := botService.SysBot().OfficialBotToken(ctx)
	if err != nil {
		return false
	}
	bot, err := s.telegramBot(ctx, token)
	if err != nil {
		return false
	}
	me, err := bot.GetMe(ctx)
	if err != nil {
		return false
	}
	member, err := bot.GetChatMember(ctx, &tgbot.GetChatMemberParams{ChatID: normalizeTelegramChannelChatID(chatID), UserID: me.ID})
	if err != nil || member == nil {
		return false
	}
	canSend := member.Type != models.ChatMemberTypeLeft && member.Type != models.ChatMemberTypeBanned && telegramBotCanSendMessage(member)
	if !canSend {
		_ = cache.Instance().Set(ctx, cacheKey, 1, 24*time.Hour)
	}
	return canSend
}

func officialBotChatUnavailableCacheKey(chatID string) string {
	return "youban_publish:official_bot_chat_unavailable:" + normalizeTelegramChannelChatID(chatID)
}

func (s *sSysPublish) sendMessageTemplateByAccountClient(ctx context.Context, client *telegram.Client, channel *messagePushChannel, job telegramJobRecord, template *sysin.MessageTemplateModel) error {
	if client == nil || channel == nil {
		return gerror.New("协议号降级发送客户端或目标无效")
	}
	peer, err := messagePushInputPeer(channel)
	if err != nil {
		return err
	}
	media := messageTemplateTelegramMedia(template)
	templateHash := messageTemplateHash(template)
	var source *messageTemplateForwardSource
	if !messageTemplateRequiresSanitizedUpload(media) {
		source, err = s.messageTemplateForwardSource(ctx, channel.TgAccountId, templateHash)
		if err != nil {
			g.Log().Warningf(ctx, "读取消息模板复用源失败 jobId:%d tgAccountId:%d templateHash:%s err:%+v", job.Id, channel.TgAccountId, templateHash, err)
			source = nil
		}
	}
	sent, err := s.sendMessageTemplateWithTgClient(ctx, client, peer, telegramRichTextHTML(template.Text), media, source, channel.TgAccountId, templateHash)
	if err != nil {
		return gerror.Wrap(err, "协议号降级发送失败")
	}
	if err = s.completeMessagePushJob(ctx, job, sent, "更新协议号降级任务状态失败"); err != nil {
		return err
	}
	s.appendTelegramJobLog(ctx, job, "account_send", sysin.MessagePushStatusSent, "Inline和Bot均不可用，已由协议号发送成功")
	return nil
}

func (p *publishCollectorAccountMediaProvider) ResolvePeer(ctx context.Context, tenantID, accountID int64, chatID string, client *telegram.Client) (tg.InputPeerClass, error) {
	if p == nil || p.publish == nil {
		return nil, gerror.New("Telegram账号媒体 Peer 解析器未初始化")
	}
	return p.publish.collectMediaInputPeer(ctx, tenantID, accountID, client, chatID)
}

func (p *publishCollectorAccountMediaProvider) StoreMedia(ctx context.Context, task *collectorin.AccountTask, localPath string) (*collectorin.AccountMediaDownloadResult, error) {
	if p == nil || p.publish == nil || task == nil {
		return nil, gerror.New("Telegram账号媒体存储参数无效")
	}
	ctx = collectMediaRuntimeContext(ctx, task.MediaOwnerAccountID)
	md5Value, mediaSize, mimeType, err := calculateTelegramMediaFingerprint(localPath, task.Media.Type, task.Media.SourceMimeType)
	if err != nil {
		return nil, err
	}
	forceRefresh := strings.HasPrefix(task.TaskKey, "profile-media-refresh:")
	if cached, ok, cacheErr := lookupTelegramCollectorMediaCache(ctx, task.Media.Type, md5Value, mediaSize, mimeType); !forceRefresh && cacheErr != nil {
		return nil, cacheErr
	} else if !forceRefresh && ok {
		task.Media.FileURL = telegramCollectorMediaCacheURL(cached)
		task.Media.StoragePath = cached.StoragePath
		task.Media.FileMD5 = md5Value
		task.Media.SourceSize = mediaSize
		task.Media.FilePHash = cached.PHash
		task.Media.PosterURL = cached.PosterURL
		return &collectorin.AccountMediaDownloadResult{FileURL: task.Media.FileURL, StoragePath: task.Media.StoragePath, PosterURL: cached.PosterURL, PosterPath: cached.PosterStoragePath, Media: task.Media}, nil
	}
	attachment, err := p.publish.uploadCollectMediaToStorage(ctx, task.Media.Type, localPath)
	if err != nil {
		return nil, err
	}
	item := task.Media
	item.FileURL = strings.TrimSpace(attachment.FileUrl)
	item.StoragePath = normalizeStoredMediaPath(attachment.Path)
	item.FileMD5 = md5Value
	item.SourceSize = mediaSize
	item.SourceMimeType = mimeType
	return &collectorin.AccountMediaDownloadResult{
		AttachmentID: attachment.Id, FileURL: item.FileURL, StoragePath: item.StoragePath, Media: item,
	}, nil
}

func (p *publishCollectorAccountMediaProvider) StoreMediaPoster(ctx context.Context, task *collectorin.AccountTask, localPath string) (string, string, error) {
	if p == nil || p.publish == nil || task == nil {
		return "", "", gerror.New("Telegram账号媒体封面存储参数无效")
	}
	ctx = collectMediaRuntimeContext(ctx, task.MediaOwnerAccountID)
	md5Value, mediaSize, mimeType, err := calculateTelegramMediaFingerprint(localPath, "image", "image/jpeg")
	if err != nil {
		return "", "", err
	}
	if cached, ok, cacheErr := lookupTelegramCollectorMediaCache(ctx, "image", md5Value, mediaSize, mimeType); cacheErr != nil {
		return "", "", cacheErr
	} else if ok {
		return telegramCollectorMediaCacheURL(cached), normalizeStoredMediaPath(cached.StoragePath), nil
	}
	attachment, err := p.publish.uploadCollectMediaToStorage(ctx, "image", localPath)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(attachment.FileUrl), normalizeStoredMediaPath(attachment.Path), nil
}

func collectorMediaItemFromCollect(item collectMediaItem) collectorin.CollectorMediaItem {
	return collectorin.CollectorMediaItem{
		Type: item.Type, Purpose: item.Purpose, FileID: item.FileId, FileURL: item.FileUrl,
		StoragePath: item.StoragePath, PosterURL: item.PosterUrl, FileMD5: item.FileMd5,
		FilePHash: item.FilePhash, SourceKind: item.SourceKind, SourceMediaID: item.SourceMediaId,
		SourceAccessHash: item.SourceAccessHash, SourceFileReference: append([]byte(nil), item.SourceFileReference...),
		SourceThumbSize: item.SourceThumbSize, SourceMimeType: item.SourceMimeType,
		SourceDCID: item.SourceDCId, SourceSize: item.SourceSize, DebugMetaJSON: item.DebugMetaJson,
	}
}

func collectMediaItemFromCollector(item collectorin.CollectorMediaItem) collectMediaItem {
	return collectMediaItem{
		Type: item.Type, Purpose: item.Purpose, FileId: item.FileID, FileUrl: item.FileURL,
		StoragePath: item.StoragePath, PosterUrl: item.PosterURL, FileMd5: item.FileMD5,
		FilePhash: item.FilePHash, SourceKind: item.SourceKind, SourceMediaId: item.SourceMediaID,
		SourceAccessHash: item.SourceAccessHash, SourceFileReference: append([]byte(nil), item.SourceFileReference...),
		SourceThumbSize: item.SourceThumbSize, SourceMimeType: item.SourceMimeType,
		SourceDCId: item.SourceDCID, SourceSize: item.SourceSize, DebugMetaJson: item.DebugMetaJSON,
	}
}

func (h *publishCollectorDeliveryHandler) HandleCollectorDelivery(ctx context.Context, delivery *collectorin.CollectorDelivery) error {
	if h == nil || h.publish == nil || delivery == nil {
		return gerror.New("Telegram采集交付处理参数无效")
	}
	switch delivery.SourceType {
	case collectorin.SourceTypeBot:
		return h.publish.ingestCollectorBotDelivery(ctx, delivery)
	case collectorin.SourceTypeAccount:
		return h.publish.ingestCollectorAccountDelivery(ctx, delivery)
	default:
		return gerror.Newf("暂不支持的Telegram采集来源类型：%s", delivery.SourceType)
	}
}

func (s *sSysPublish) ingestCollectorBotDelivery(ctx context.Context, delivery *collectorin.CollectorDelivery) error {
	if !s.collectGlobalEnabled(ctx) {
		return nil
	}
	source, err := g.DB().Model(publishCollectSourceTable+" s").Safe().Ctx(ctx).
		InnerJoin("hg_youban_publish_tenant_vip vip", "vip.tenant_id=s.tenant_id AND vip.status=1 AND vip.level>0 AND vip.deleted_at IS NULL").
		Fields("s.id,s.tenant_id,s.account_id,s.bot_id").
		Where("s.id", delivery.SourceID).
		Where("s.source_type", sysin.CollectSourceTypeBot).
		Where("s.collect_enabled", 1).
		Where("s.status", 1).
		Where("(vip.expired_at IS NULL OR vip.expired_at>?)", gtime.Now()).
		WhereNull("s.deleted_at").
		One()
	if err != nil {
		return gerror.Wrap(err, "读取Bot采集源失败")
	}
	if source.IsEmpty() {
		return nil
	}
	blocked, err := s.collectMessageFromAccountPublishChannel(ctx, source["tenant_id"].Int64(), 0, delivery.SourceChatID)
	if err != nil {
		return gerror.Wrap(err, "检查Bot采集上架频道过滤失败")
	}
	if blocked {
		return nil
	}
	allowed, err := s.ensureBotCollectChannelApproval(ctx, source, delivery)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	message := collectorDeliveryMessage(delivery, source["tenant_id"].Int64(), source["account_id"].Int64(), source["id"].Int64(), sysin.CollectSourceTypeBot)
	message.BotId = source["bot_id"].Int64()
	if collectorDeliveryIsEdit(delivery.RawUpdate) {
		return s.updateCollectedBotMessage(ctx, message)
	}
	_, err = s.ingestAndProcessCollectMessage(ctx, message)
	return err
}

func collectorDeliveryIsEdit(raw []byte) bool {
	var update struct {
		EditedChannelPost json.RawMessage `json:"edited_channel_post"`
		EditedMessage     json.RawMessage `json:"edited_message"`
	}
	if json.Unmarshal(raw, &update) != nil {
		return false
	}
	return (len(update.EditedChannelPost) > 0 && string(update.EditedChannelPost) != "null") ||
		(len(update.EditedMessage) > 0 && string(update.EditedMessage) != "null")
}

// Edits are updates, never new collection input. In particular, an edited
// member of a historical album must not replace its complete media set.
func (s *sSysPublish) updateCollectedBotMessage(ctx context.Context, message *CollectMessage) error {
	event, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).
		Where("tenant_id", message.TenantId).Where("account_id", message.AccountId).
		Where("source_id", message.SourceId).Where("source_unique_key", message.SourceUniqueKey).One()
	if err != nil {
		return gerror.Wrap(err, "查找编辑消息对应采集事件失败")
	}
	if event.IsEmpty() {
		g.Log().Infof(ctx, "忽略无来源资料的编辑消息 sourceId:%d chat:%s messageId:%d", message.SourceId, message.SourceChatId, message.SourceMessageId)
		return nil
	}
	dispatches, err := pdao.YoubanPublishCollectDispatch.Ctx(ctx).
		Where("event_id", event["id"].Int64()).WhereGT("profile_id", 0).All()
	if err != nil {
		return gerror.Wrap(err, "查找编辑消息对应资料失败")
	}
	if len(dispatches) == 0 {
		g.Log().Infof(ctx, "忽略未生成资料的编辑消息 eventId:%d sourceId:%d messageId:%d", event["id"].Int64(), message.SourceId, message.SourceMessageId)
		return nil
	}
	// Refresh identity only; do not replay pairing, reset its waiting window,
	// replace album members, or enqueue a new publication.
	if err = s.upsertCollectEventMedia(ctx, event, normalizeCollectMediaItems(message.Media)); err != nil {
		return err
	}
	if strings.TrimSpace(message.RawText) == "" {
		return nil
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, dispatch := range dispatches {
			rule, ruleErr := pdao.YoubanPublishCollectRule.Ctx(ctx).WherePri(dispatch["rule_id"].Int64()).One()
			if ruleErr != nil {
				return ruleErr
			}
			if rule.IsEmpty() {
				return gerror.New("编辑来源资料的采集规则不存在")
			}
			cols := idao.ContentProfile.Columns()
			if _, updateErr := tx.Model(idao.ContentProfile.Table()).
				Where(cols.Id, dispatch["profile_id"].Int64()).
				Where("EXISTS (SELECT 1 FROM "+publishProfileStateTable+" ps WHERE ps.profile_id=? AND ps.tenant_id=? AND ps.account_id=? AND ps.deleted_at IS NULL)", dispatch["profile_id"].Int64(), message.TenantId, message.AccountId).
				Data(g.Map{cols.PlainText: normalizeCollectRuleBody(message.RawText, rule)}).Update(); updateErr != nil {
				return updateErr
			}
		}
		_, updateErr := tx.Model(pdao.YoubanPublishCollectEvent.Table()).Where("id", event["id"].Int64()).
			Data(g.Map{"raw_text": message.RawText, "text_hash": collectHash(message.RawText)}).Update()
		return updateErr
	})
	if err != nil {
		return err
	}
	for _, dispatch := range dispatches {
		if err := s.syncProfileNoteIndex(ctx, dispatch["profile_id"].Int64()); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) ensureBotCollectChannelApproval(ctx context.Context, source gdb.Record, delivery *collectorin.CollectorDelivery) (bool, error) {
	tenantID, accountID, botID := source["tenant_id"].Int64(), source["account_id"].Int64(), source["bot_id"].Int64()
	chatID := normalizeTelegramChannelChatID(delivery.SourceChatID)
	if tenantID <= 0 || accountID <= 0 || botID <= 0 || chatID == "" {
		return false, nil
	}
	now := gtime.Now()
	var channel struct {
		Id    int64  `json:"id"`
		Title string `json:"title"`
	}
	if err := pdao.YoubanPublishCollectChannel.Ctx(ctx).Fields("id,title").Where("tenant_id", tenantID).WhereIn("telegram_chat_id", tgChannelCacheLookupIds(chatID)).Scan(&channel); !collectOptionalRecordScanSucceeded(err) {
		return false, err
	}
	if channel.Id <= 0 {
		if err := s.upsertCollectChannel(ctx, tenantID, chatID, "channel", chatID, "", 1, 0, delivery.SourceMessageID, now, now); err != nil {
			return false, gerror.Wrap(err, "记录BOT采集频道失败")
		}
		if err := pdao.YoubanPublishCollectChannel.Ctx(ctx).Fields("id,title").Where("tenant_id", tenantID).Where("telegram_chat_id", chatID).Scan(&channel); err != nil || channel.Id <= 0 {
			return false, err
		}
	} else {
		_, _ = pdao.YoubanPublishCollectChannel.Ctx(ctx).Where("id", channel.Id).Data(g.Map{"last_message_id": delivery.SourceMessageID, "last_message_at": now, "last_seen_at": now, "updated_at": now}).Update()
	}
	_ = s.touchCollectBotChannel(ctx, tenantID, botID, chatID, now)
	var relation struct {
		Id             int64  `json:"id"`
		CollectEnabled int    `json:"collectEnabled"`
		Status         string `json:"status"`
		ApprovalStatus string `json:"approvalStatus"`
	}
	if err := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).Fields("id,collect_enabled,status,approval_status").Where("tenant_id", tenantID).Where("account_id", accountID).Where("channel_id", channel.Id).Scan(&relation); !collectOptionalRecordScanSucceeded(err) {
		return false, gerror.Wrap(err, "读取BOT采集频道审批状态失败")
	}
	if relation.Id > 0 {
		return botCollectChannelRelationEnabled(relation.CollectEnabled, relation.Status, relation.ApprovalStatus), nil
	}
	// Queue before creating the discovery relation: a crash or send failure
	// must not permanently lose the first-channel notification.
	if err := s.enqueueCollectNotification(ctx, collectNotificationPayload{
		ID:    fmt.Sprintf("discovery:%d:%d:%d:%s", tenantID, accountID, source["id"].Int64(), chatID),
		BotID: botID, TenantID: tenantID, AccountID: accountID, SourceID: source["id"].Int64(), ChannelID: chatID,
	}); err != nil {
		return false, err
	}
	_, err := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).Data(pdo.YoubanPublishCollectAccountChannel{
		TenantId: tenantID, AccountId: accountID, ChannelId: channel.Id, SourceId: source["id"].Int64(),
		CollectEnabled: 1, Status: sysin.CollectAccountChannelEnabled, ApprovalStatus: "approved", ApprovedAt: now,
	}).InsertAndGetId()
	if err != nil {
		// A concurrent delivery may have created the relation; respect its decision.
		lookupErr := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).Fields("id,collect_enabled,status,approval_status").Where("tenant_id", tenantID).Where("account_id", accountID).Where("channel_id", channel.Id).Scan(&relation)
		if lookupErr == nil && relation.Id > 0 {
			return botCollectChannelRelationEnabled(relation.CollectEnabled, relation.Status, relation.ApprovalStatus), nil
		}
		return false, gerror.Wrap(err, "创建BOT采集频道审批记录失败")
	}
	return true, nil
}

func (s *sSysPublish) sendBotCollectChannelDiscovery(ctx context.Context, source gdb.Record, chatID string, relationID int64, notificationID string) error {
	tenantID, accountID, botID := source["tenant_id"].Int64(), source["account_id"].Int64(), source["bot_id"].Int64()
	rules, ruleErr := pdao.YoubanPublishCollectRule.Ctx(ctx).As("r").
		InnerJoin(pdao.YoubanPublishCollectSourceRule.Table()+" sr", "sr.rule_id=r.id AND sr.tenant_id=r.tenant_id").
		Fields("r.id,r.name,r.intro_fee_suffix,r.bot_id").Where("sr.source_id", source["id"].Int64()).
		Where("r.tenant_id", tenantID).Where("r.account_id", accountID).
		WhereNull("r.deleted_at").Where("r.intro_fee_suffix_enabled", 1).All()
	if ruleErr != nil {
		return gerror.Wrap(ruleErr, "读取新频道默认后缀失败")
	}
	title := firstNonEmpty(s.collectChannelDisplayName(ctx, tenantID, chatID), chatID)
	linkLine := collectChannelLinkLine(s.collectChannelDisplayLink(ctx, tenantID, chatID))
	if len(rules) == 0 {
		text := "发现新的 BOT 采集频道\n频道：" + html.EscapeString(title) + "\n频道 ID：" + html.EscapeString(chatID) + linkLine + "\n采集状态：默认开启"
		buttons := [][]botsysin.NotifyButton{{{Label: "允许采集", Data: fmt.Sprintf("cm:channelapprove:%d", relationID)}, {Label: "禁用", Data: fmt.Sprintf("cm:channelreject:%d", relationID)}}}
		if notifyErr := botService.SysBot().NotifyCollectEvent(ctx, botID, "", notificationID+":plain", text, buttons); notifyErr != nil {
			return notifyErr
		}
	}
	for _, rule := range rules {
		suffix := strings.TrimSpace(rule["intro_fee_suffix"].String())
		if configured, found, suffixErr := s.resolveIntroFeeChannelSuffix(ctx, gdb.Record{"tenant_id": source["tenant_id"], "account_id": source["account_id"], "bot_id": source["bot_id"], "source_chat_id": gvar.New(chatID)}, rule); suffixErr != nil {
			return suffixErr
		} else if found {
			suffix = configured
		}
		if suffix == "" {
			suffix = "未设置"
		}
		text := fmt.Sprintf("发现新的代理采集频道\n频道：%s\n频道 ID：%s%s\n采集规则：%s\n频道后缀：%s\n采集状态：默认开启\n后续新采集资料将使用上述后缀，可通过下方按钮禁用采集或修改后缀。", html.EscapeString(title), html.EscapeString(chatID), linkLine, html.EscapeString(rule["name"].String()), html.EscapeString(suffix))
		buttons := [][]botsysin.NotifyButton{{{Label: "禁用", Data: fmt.Sprintf("cm:channelreject:%d", relationID)}, {Label: "修改后缀", Data: fmt.Sprintf("cm:channeledit:%d:%s:0", rule["id"].Int64(), chatID)}}}
		if notifyErr := botService.SysBot().NotifyCollectEvent(ctx, botID, "", fmt.Sprintf("%s:rule:%d", notificationID, rule["id"].Int64()), text, buttons); notifyErr != nil {
			return notifyErr
		}
	}
	return nil
}

func collectOptionalRecordScanSucceeded(err error) bool {
	return err == nil || errors.Is(err, sql.ErrNoRows)
}

func botCollectChannelRelationEnabled(collectEnabled int, status, approvalStatus string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	approvalStatus = strings.ToLower(strings.TrimSpace(approvalStatus))
	if status == sysin.CollectAccountChannelDisabled || approvalStatus == "rejected" {
		return false
	}
	// Missing and legacy pending relations follow the default-enabled policy.
	if status == "" || status == sysin.CollectAccountChannelPending {
		return true
	}
	return collectEnabled == 1 && (status == sysin.CollectAccountChannelEnabled || status == "approved")
}

func (s *sSysPublish) ingestCollectorAccountDelivery(ctx context.Context, delivery *collectorin.CollectorDelivery) error {
	if err := s.ensureAccountCollectChannelRelation(ctx, delivery); err != nil {
		return err
	}
	blocked, err := s.collectMessageFromAccountPublishChannel(ctx, delivery.TenantID, delivery.TgAccountID, delivery.SourceChatID)
	if err != nil {
		return gerror.Wrap(err, "检查协议号采集上架频道过滤失败")
	}
	if blocked {
		return nil
	}
	message := collectorDeliveryMessage(delivery, delivery.TenantID, delivery.AccountID, delivery.SourceID, sysin.CollectSourceTypeAccount)
	message.TgAccountId = delivery.TgAccountID
	if groupedID := strings.TrimSpace(delivery.SourceGroupedID); groupedID != "" {
		message.SourceUniqueKey = accountCollectMaterialGroupKey(delivery, groupedID)
	}
	_, err = s.ingestAndProcessCollectMessage(ctx, message)
	return err
}

func (s *sSysPublish) ensureAccountCollectChannelRelation(ctx context.Context, delivery *collectorin.CollectorDelivery) error {
	if delivery == nil || delivery.SourceID <= 0 || delivery.TenantID <= 0 || delivery.AccountID <= 0 {
		return nil
	}
	chatID := normalizeTelegramChannelChatID(delivery.SourceChatID)
	if chatID == "" {
		return nil
	}
	var source struct {
		Id             int64  `json:"id"`
		Title          string `json:"title"`
		SourceUsername string `json:"sourceUsername"`
		CollectEnabled int    `json:"collectEnabled"`
		Status         int    `json:"status"`
	}
	if err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Fields("id,title,source_username,collect_enabled,status").
		Where("id", delivery.SourceID).Where("tenant_id", delivery.TenantID).
		Where("account_id", delivery.AccountID).Scan(&source); err != nil {
		return gerror.Wrap(err, "读取账户采集源频道信息失败")
	}
	if source.Id <= 0 {
		return nil
	}
	now := gtime.Now()
	if err := s.upsertCollectChannel(ctx, delivery.TenantID, chatID, "channel", strings.TrimSpace(source.Title), strings.TrimSpace(source.SourceUsername), 0, 0, delivery.SourceMessageID, now, now); err != nil {
		return gerror.Wrap(err, "同步账户采集频道信息失败")
	}
	var channel struct {
		Id int64 `json:"id"`
	}
	channelCols := pdao.YoubanPublishCollectChannel.Columns()
	if err := pdao.YoubanPublishCollectChannel.Ctx(ctx).Fields(channelCols.Id).
		Where(channelCols.TenantId, delivery.TenantID).
		WhereIn(channelCols.TelegramChatId, tgChannelCacheLookupIds(chatID)).Scan(&channel); err != nil {
		return gerror.Wrap(err, "读取账户采集频道关系失败")
	}
	if channel.Id <= 0 {
		return gerror.New("账户采集频道关系创建失败")
	}
	sourceCols := pdao.YoubanPublishCollectSource.Columns()
	if _, err := pdao.YoubanPublishCollectSource.Ctx(ctx).
		Where(sourceCols.Id, source.Id).Where(sourceCols.ChannelId, 0).
		Data(pdo.YoubanPublishCollectSource{ChannelId: channel.Id}).Update(); err != nil {
		return gerror.Wrap(err, "回写账户采集频道关系失败")
	}
	enabled := source.CollectEnabled == 1 && source.Status == 1
	relation := pdo.YoubanPublishCollectAccountChannel{
		TenantId: delivery.TenantID, AccountId: delivery.AccountID, ChannelId: channel.Id, SourceId: source.Id,
		CollectEnabled: introFeeBoolToInt(enabled), Status: sysin.CollectAccountChannelDisabled, ApprovalStatus: "approved",
	}
	if enabled {
		relation.Status = sysin.CollectAccountChannelEnabled
		relation.ApprovedAt = now
	} else {
		relation.DisabledAt = now
	}
	if _, err := pdao.YoubanPublishCollectAccountChannel.Ctx(ctx).Data(relation).
		OnConflict("tenant_id,account_id,channel_id").OnDuplicate("source_id,updated_at").Save(); err != nil {
		return gerror.Wrap(err, "同步账户采集频道状态失败")
	}
	return nil
}

func collectorDeliveryMessage(delivery *collectorin.CollectorDelivery, tenantID, accountID, sourceID int64, sourceType string) *CollectMessage {
	message := &CollectMessage{
		TenantId: tenantID, AccountId: accountID, SourceId: sourceID, SourceType: sourceType,
		SourceChatId: delivery.SourceChatID, SourceMessageId: delivery.SourceMessageID,
		SourceGroupedId: delivery.SourceGroupedID, SourceUniqueKey: delivery.SourceUniqueKey, RawText: delivery.RawText,
	}
	if !delivery.ReceivedAt.IsZero() {
		message.ReceivedAt = gtime.New(delivery.ReceivedAt)
	}
	message.Media = make([]collectMediaItem, 0, len(delivery.Media))
	for _, item := range delivery.Media {
		message.Media = append(message.Media, collectMediaItem{
			Type: item.Type, Purpose: item.Purpose, FileId: item.FileID, FileUrl: item.FileURL,
			StoragePath: item.StoragePath, PosterUrl: item.PosterURL, FileMd5: item.FileMD5,
			FilePhash: item.FilePHash, SourceKind: item.SourceKind, SourceMediaId: item.SourceMediaID,
			SourceAccessHash: item.SourceAccessHash, SourceFileReference: append([]byte(nil), item.SourceFileReference...),
			SourceThumbSize: item.SourceThumbSize, SourceMimeType: item.SourceMimeType,
			SourceDCId: item.SourceDCID, SourceSize: item.SourceSize, DebugMetaJson: item.DebugMetaJSON,
		})
	}
	return message
}

func accountCollectMaterialGroupKey(delivery *collectorin.CollectorDelivery, groupedID string) string {
	if delivery == nil {
		return ""
	}
	groupedID = strings.TrimSpace(groupedID)
	if groupedID == "" {
		return strings.TrimSpace(delivery.SourceUniqueKey)
	}
	return fmt.Sprintf(
		"account:%d:source:%d:%s:group:%s",
		delivery.TgAccountID,
		delivery.SourceID,
		strings.TrimSpace(delivery.SourceChatID),
		groupedID,
	)
}
