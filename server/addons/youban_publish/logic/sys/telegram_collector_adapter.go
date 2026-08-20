package sys

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

type publishCollectorDeliveryHandler struct {
	publish *sSysPublish
}

type publishCollectorAccountTaskHandler struct{ publish *sSysPublish }

type publishCollectorAccountMediaProvider struct{ publish *sSysPublish }

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

func (s *sSysPublish) handleMessageMediaFallbackAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) error {
	startedAt := time.Now()
	stageStartedAt := startedAt
	var jobID int64
	var err error
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
	const prefix = "message-media-fallback:"
	parts := strings.Split(strings.TrimPrefix(task.TaskKey, prefix), ":")
	if len(parts) != 2 || !strings.HasPrefix(task.TaskKey, prefix) {
		return gerror.New("媒体降级发送账号任务参数无效")
	}
	jobID, err = strconv.ParseInt(parts[0], 10, 64)
	if err != nil || jobID <= 0 || (parts[1] != "display" && parts[1] != "verify") {
		return gerror.New("媒体降级发送账号任务参数无效")
	}
	job, err := s.telegramJobById(ctx, jobID)
	if err != nil {
		return err
	}
	stage("读取任务与Job", "purpose", parts[1], "jobStatus", job.Status)
	if job.Status != "sending" {
		messageCount, countErr := s.telegramJobSentMessageCount(ctx, job.Id, parts[1])
		if countErr != nil {
			return countErr
		}
		if mediaFallbackTaskCanSkip(job.Status, messageCount) {
			g.Log().Infof(ctx, "协议号媒体降级任务幂等完成 taskId:%d jobId:%d tgAccountId:%d purpose:%s messageCount:%d", task.ID, job.Id, task.AccountID, parts[1], messageCount)
			return nil
		}
		// Reconciliation may conservatively move a Bot-failed job to unknown
		// before the account fallback starts. With no messages for this phase,
		// it is safe to claim the job for the fallback instead of abandoning it.
		if job.Status == "unknown" && messageCount == 0 {
			result, updateErr := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
				Where("id", job.Id).Where("status", "unknown").
				Data(g.Map{"status": "sending", "dispatch_status": tgDispatchStatusProcessing, "updated_at": gtime.Now()}).Update()
			if updateErr != nil {
				return gerror.Wrap(updateErr, "恢复协议号媒体降级任务状态失败")
			}
			rowsAffected, rowsErr := result.RowsAffected()
			if rowsErr != nil {
				return gerror.Wrap(rowsErr, "确认协议号媒体降级任务状态失败")
			}
			if rowsAffected == 0 {
				return gerror.New("协议号媒体降级任务正在被其他发送流程处理")
			}
			job.Status = "sending"
			g.Log().Infof(ctx, "协议号媒体降级任务从未知状态恢复发送 taskId:%d jobId:%d purpose:%s", task.ID, job.Id, parts[1])
		}
		if job.Status != "sending" {
			stateErr := gerror.Newf("协议号媒体降级任务状态异常，拒绝静默完成：jobId=%d status=%s sendPhase=%s purpose=%s messageCount=%d", job.Id, job.Status, job.SendPhase, parts[1], messageCount)
			g.Log().Warningf(ctx, "协议号媒体降级任务拒绝静默完成 taskId:%d tgAccountId:%d err:%+v", task.ID, task.AccountID, stateErr)
			s.appendTelegramJobLog(ctx, job, "account_fallback", "failed", stateErr.Error())
			return stateErr
		}
	}
	g.Log().Infof(ctx, "协议号媒体降级任务开始执行 taskId:%d jobId:%d tgAccountId:%d purpose:%s attempt:%d/%d", task.ID, job.Id, task.AccountID, parts[1], task.AttemptCount, task.MaxAttempts)
	channel, err := s.messagePushChannelFromJob(ctx, job)
	if err != nil {
		return err
	}
	if task.AccountID != channel.TgAccountId {
		return gerror.New("媒体降级发送账号任务与目标频道账号不一致")
	}
	stage("读取频道")
	media, err := s.telegramJobMedia(ctx, job, parts[1])
	if err != nil {
		return err
	}
	if parts[1] == "display" {
		media, err = s.selectTelegramDisplayMediaForTenant(ctx, job, media)
		if err != nil {
			return err
		}
	}
	stage("读取并选择媒体", "mediaCount", len(media))
	var verifyMedia []*telegramMediaItem
	if parts[1] == "display" {
		verifyMedia, err = s.telegramJobMedia(ctx, job, "verify")
		if err != nil {
			return err
		}
	}
	stage("读取验证媒体", "verifyMediaCount", len(verifyMedia))
	caption := ""
	if parts[1] == "display" {
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
	caption = telegramCaptionWithJobMarker(caption, job.Id, parts[1])
	peer, err := messagePushInputPeer(channel)
	if err != nil {
		return err
	}
	messages, err := s.sendMessageTemplateWithTgClient(ctx, client, peer, caption, media, nil, task.AccountID, "")
	if err != nil {
		return gerror.Wrap(err, "协议号媒体降级发送失败")
	}
	stage("发送展示媒体", "messageCount", len(messages))
	for _, message := range messages {
		if message != nil {
			message.Purpose = parts[1]
		}
	}
	if err = s.saveTelegramSentMessages(ctx, job, messages); err != nil {
		return err
	}
	stage("保存展示消息")
	if err = s.updateTelegramMediaFileIds(ctx, messages); err != nil {
		return err
	}
	if parts[1] == "display" {
		if err = s.updateTelegramJobSendPhase(ctx, job.Id, telegramSendPhaseDisplayConfirmed); err != nil {
			return err
		}
		if len(verifyMedia) == 0 {
			g.Log().Infof(ctx, "协议号媒体降级任务发送成功 taskId:%d jobId:%d tgAccountId:%d displayMessages:%d verifyMessages:0", task.ID, job.Id, task.AccountID, len(messages))
			return s.completeTelegramJob(ctx, job)
		}
		verifyCaption := telegramCaptionWithJobMarker("", job.Id, "verify")
		verifyMessages, err := s.sendMessageTemplateWithTgClient(ctx, client, peer, verifyCaption, verifyMedia, nil, task.AccountID, "")
		if err != nil {
			return gerror.Wrap(err, "协议号验证媒体降级发送失败")
		}
		stage("发送验证媒体", "messageCount", len(verifyMessages))
		for _, message := range verifyMessages {
			if message != nil {
				message.Purpose = "verify"
			}
		}
		if err = s.saveTelegramSentMessages(ctx, job, verifyMessages); err != nil {
			return err
		}
		stage("保存验证消息")
		if err = s.updateTelegramMediaFileIds(ctx, verifyMessages); err != nil {
			return err
		}
		if err = s.updateTelegramJobSendPhase(ctx, job.Id, telegramSendPhaseVerifyConfirmed); err != nil {
			return err
		}
		g.Log().Infof(ctx, "协议号媒体降级任务发送成功 taskId:%d jobId:%d tgAccountId:%d displayMessages:%d verifyMessages:%d", task.ID, job.Id, task.AccountID, len(messages), len(verifyMessages))
		return s.completeTelegramJob(ctx, job)
	}
	if err = s.updateTelegramJobSendPhase(ctx, job.Id, telegramSendPhaseVerifyConfirmed); err != nil {
		return err
	}
	g.Log().Infof(ctx, "协议号验证媒体降级任务发送成功 taskId:%d jobId:%d tgAccountId:%d verifyMessages:%d", task.ID, job.Id, task.AccountID, len(messages))
	return s.completeTelegramJob(ctx, job)
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

func (s *sSysPublish) handleMessageReconcileAccountTask(ctx context.Context, client *telegram.Client, task *collectorin.AccountTask) error {
	const prefix = "message-reconcile:"
	jobID, err := strconv.ParseInt(strings.TrimPrefix(task.TaskKey, prefix), 10, 64)
	if err != nil || jobID <= 0 || !strings.HasPrefix(task.TaskKey, prefix) {
		return gerror.New("消息对账账号任务参数无效")
	}
	job, err := s.telegramJobById(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Status != "unknown" {
		return nil
	}
	channel, err := s.telegramReconcileChannel(ctx, job)
	if err != nil {
		return err
	}
	if task.AccountID != channel.TgAccountId {
		return gerror.New("消息对账账号任务与目标频道账号不一致")
	}
	return s.reconcileUnknownTelegramJobWithClient(ctx, client, job)
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
	s.appendTelegramJobLog(ctx, job, "inline_send", "started", fmt.Sprintf("开始Inline发布 accountTaskId:%d tgAccountId:%d", task.ID, task.AccountID))
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
	if cached, ok, cacheErr := lookupTelegramCollectorMediaCache(ctx, task.Media.Type, md5Value, mediaSize, mimeType); cacheErr != nil {
		return nil, cacheErr
	} else if ok {
		task.Media.FileURL = telegramCollectorMediaCacheURL(cached)
		task.Media.StoragePath = cached.StoragePath
		task.Media.FileMD5 = md5Value
		task.Media.SourceSize = mediaSize
		task.Media.FilePHash = cached.PHash
		task.Media.PosterURL = firstNonEmpty(cached.PosterURL, cached.PosterStoragePath)
		return &collectorin.AccountMediaDownloadResult{FileURL: task.Media.FileURL, StoragePath: task.Media.StoragePath, Media: task.Media}, nil
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
	blocked, err := s.botCollectMessageFromPublishChannel(ctx, source["tenant_id"].Int64(), g.NewVar(delivery.SourceChatID).Int64())
	if err != nil {
		return gerror.Wrap(err, "检查Bot采集上架频道过滤失败")
	}
	if blocked {
		return nil
	}
	message := collectorDeliveryMessage(delivery, source["tenant_id"].Int64(), source["account_id"].Int64(), source["id"].Int64(), sysin.CollectSourceTypeBot)
	message.BotId = source["bot_id"].Int64()
	_, err = s.ingestAndProcessCollectMessage(ctx, message)
	return err
}

func (s *sSysPublish) ingestCollectorAccountDelivery(ctx context.Context, delivery *collectorin.CollectorDelivery) error {
	message := collectorDeliveryMessage(delivery, delivery.TenantID, delivery.AccountID, delivery.SourceID, sysin.CollectSourceTypeAccount)
	message.TgAccountId = delivery.TgAccountID
	if groupedID := strings.TrimSpace(delivery.SourceGroupedID); groupedID != "" {
		message.SourceUniqueKey = accountCollectMaterialGroupKey(delivery, groupedID)
	}
	_, err := s.ingestAndProcessCollectMessage(ctx, message)
	return err
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
