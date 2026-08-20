package sys

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	gotdmessage "github.com/gotd/td/telegram/message"
	gotdhtml "github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/tg"
	_ "golang.org/x/image/webp"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	botsysin "hotgo/addons/youban_bot/model/input/sysin"
	botService "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
)

type messagePushChannel struct {
	Id           int64  `json:"id"`
	TgAccountId  int64  `json:"tgAccountId"`
	AccessHash   string `json:"accessHash"`
	ChannelTitle string `json:"channelTitle"`
	TargetChatId string `json:"targetChatId"`
	BotIdJson    string `json:"botIdJson"`
	IsBroadcast  int    `json:"isBroadcast"`
	IsMegagroup  int    `json:"isMegagroup"`
}

type messagePushTgAccountOwner struct {
	Id               int64  `json:"id"`
	TenantId         int64  `json:"tenant_id"`
	AccountId        int64  `json:"account_id"`
	DisplayName      string `json:"display_name"`
	TelegramUsername string `json:"telegram_username"`
}

type messageTemplateForwardSource struct {
	Peer       tg.InputPeerClass
	MessageIds []int
}

type messageTemplateForwardSourceJob struct {
	Id           int64  `json:"id"`
	TenantId     int64  `json:"tenantId"`
	TargetChatId string `json:"targetChatId"`
}

type messageTemplateForwardSourceMessage struct {
	MessageId int64 `json:"messageId"`
}

type messageTemplatePushTarget struct {
	Channel     *messagePushChannel
	AccountId   int64
	OperationNo string
	Delay       time.Duration
	Priority    int
	QueueName   string
}

func (s *sSysPublish) queueMessageTemplateTargets(ctx context.Context, template *sysin.MessageTemplateModel, targets []*messageTemplatePushTarget, tenantId int64, accountId int64) *sysin.MessageTemplatePushModel {
	res := &sysin.MessageTemplatePushModel{Total: len(targets), Results: make([]*sysin.MessageTemplatePushResultModel, 0, len(targets))}
	for _, target := range targets {
		if target == nil || target.Channel == nil {
			result := &sysin.MessageTemplatePushResultModel{Status: sysin.MessagePushStatusFailed, Message: "目标群聊或频道未配置"}
			res.Results = append(res.Results, result)
			res.Failed++
			continue
		}
		targetAccountId := target.AccountId
		if targetAccountId <= 0 {
			targetAccountId = accountId
		}
		priority := target.Priority
		if priority <= 0 {
			priority = tgJobPriorityBulk
		}
		queueName := strings.TrimSpace(target.QueueName)
		if queueName == "" {
			queueName = tgQueueNameBulk
		}
		result := s.queueMessageTemplateToChannelWithPriority(ctx, template, target.Channel, tenantId, targetAccountId, target.OperationNo, target.Delay, priority, queueName)
		res.Results = append(res.Results, result)
		if result.Status == sysin.MessagePushStatusPending || result.Status == sysin.MessagePushStatusSent {
			res.Success++
		} else {
			res.Failed++
		}
	}
	return res
}

func (s *sSysPublish) AdminMessageTemplatePush(ctx context.Context, in *sysin.MessageTemplatePushInp) (res *sysin.MessageTemplatePushModel, err error) {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, gerror.New("推送参数不能为空")
	}
	if err = ensureMessagePushTables(ctx); err != nil {
		return nil, err
	}
	if err = in.Filter(ctx); err != nil {
		return nil, err
	}
	in.ChannelIds = uniqueIds(in.ChannelIds)
	if err = s.ensureMessageTemplatesBelongTenant(ctx, []int64{in.TemplateId}, account.TenantId); err != nil {
		return nil, err
	}
	if err = s.ensureMessagePushTgAccountBelongTenant(ctx, in.AccountId, account.TenantId); err != nil {
		return nil, err
	}
	if len(in.ChannelIds) > 0 {
		if err = s.ensureChannelsBelongTenant(ctx, in.ChannelIds, account.TenantId); err != nil {
			return nil, err
		}
	}
	if len(in.TargetChatIds) > 0 {
		if err = s.ensureMessagePushTargetCaches(ctx, in.AccountId, in.TargetChatIds, account.TenantId); err != nil {
			return nil, err
		}
	}
	channels, err := s.messagePushTargets(ctx, in, account.TenantId)
	if err != nil {
		return nil, err
	}
	template, err := s.messageTemplate(ctx, in.TemplateId, account.TenantId)
	if err != nil {
		return nil, err
	}
	if template.Status != 1 {
		return nil, gerror.New("消息模板已停用")
	}
	targets := make([]*messageTemplatePushTarget, 0, len(channels))
	for _, channel := range channels {
		targets = append(targets, &messageTemplatePushTarget{Channel: channel, AccountId: in.AccountId, OperationNo: messagePushManualOperationNo(template, channel), Priority: tgJobPriorityUrgent, QueueName: tgQueueNameUrgent})
	}
	return s.queueMessageTemplateTargets(ctx, template, targets, account.TenantId, in.AccountId), nil
}

func (s *sSysPublish) queueMessageTemplateToChannelWithPriority(ctx context.Context, template *sysin.MessageTemplateModel, channel *messagePushChannel, tenantId int64, accountId int64, operationNo string, delay time.Duration, priority int, queueName string) *sysin.MessageTemplatePushResultModel {
	result := &sysin.MessageTemplatePushResultModel{
		Status: sysin.MessagePushStatusFailed,
	}
	if channel == nil || strings.TrimSpace(channel.TargetChatId) == "" {
		result.Message = "目标群聊或频道未配置"
		return result
	}
	result.ChannelId = channel.Id
	result.TargetChatId = channel.TargetChatId
	botId := firstMessagePushBotId(channel)
	job, err := s.createQueuedMessagePushJobWithPriority(ctx, template, channel, tenantId, accountId, botId, operationNo, priority, queueName)
	if err != nil {
		result.Message = err.Error()
		return result
	}
	result.JobId = job.Id
	if err = s.enqueueTelegramJob(ctx, job.Id, delay); err != nil {
		s.failMessagePushJob(ctx, job, err.Error())
		result.Message = err.Error()
		return result
	}
	result.Status = sysin.MessagePushStatusPending
	result.Message = "已加入全局TG队列"
	return result
}

func (s *sSysPublish) SendMessagePushJob(ctx context.Context, jobId int64) error {
	targetJob, err := s.telegramJobById(ctx, jobId)
	if err != nil {
		return err
	}
	lease, ok, err := s.tryTelegramChannelLease(ctx, targetJob.TargetChatId)
	if err != nil {
		return err
	}
	if !ok {
		delay := s.telegramChannelBusyDelay(ctx, jobId, targetJob.DispatchCount)
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"next_retry_at":       gtime.Now().Add(delay),
			"last_dispatch_error": "频道正在发送其他任务，已等待重新调度",
			"updated_at":          gtime.Now(),
		}).Update()
		return s.enqueueTelegramJobDirectWithUnique(ctx, jobId, delay, false)
	}
	defer s.releaseTelegramChannelLease(ctx, lease)
	job, locked, err := s.lockTelegramJob(ctx, jobId)
	if err != nil || !locked {
		return err
	}
	templateId, err := messagePushTemplateIdFromOperationNo(job.OperationNo)
	if err != nil {
		return s.handleMessagePushQueuedJobError(ctx, job, err)
	}
	template, err := s.messageTemplate(ctx, templateId, job.TenantId)
	if err != nil {
		return s.handleMessagePushQueuedJobError(ctx, job, err)
	}
	if template.Status != 1 {
		return s.handleMessagePushQueuedJobError(ctx, job, gerror.New("消息模板已停用"))
	}
	channel, err := s.messagePushChannelFromJob(ctx, job)
	if err != nil {
		return s.handleMessagePushQueuedJobError(ctx, job, err)
	}
	if err = s.sendMessagePushJob(ctx, job, template, channel); err != nil {
		return s.handleMessagePushQueuedJobError(ctx, job, err)
	}
	return nil
}

func (s *sSysPublish) sendMessagePushJob(ctx context.Context, job telegramJobRecord, template *sysin.MessageTemplateModel, channel *messagePushChannel) error {
	media := messageTemplateTelegramMedia(template)
	if recordErr := s.upsertPublishJobRecord(ctx, job, "sending", ""); recordErr != nil {
		g.Log().Warningf(ctx, "更新快速推送发送记录失败 jobId:%d err:%+v", job.Id, recordErr)
	}
	inlineValidationErr := validateInlinePublishTemplate(template)
	validationReason := "ok"
	if inlineValidationErr != nil {
		validationReason = inlineValidationErr.Error()
	}
	g.Log().Infof(ctx, "快速推送决策 jobId:%d operationNo:%s targetChatId:%s channelId:%d tgAccountId:%d sourceBotId:%d templateSerial:%s mediaCount:%d hasText:%t inlineValidation:%s", job.Id, job.OperationNo, job.TargetChatId, job.ChannelId, channelTgAccountId(channel), job.BotId, templateSerialNo(template), len(media), template != nil && strings.TrimSpace(template.Text) != "", validationReason)
	// Inline only supports a single media item. Multi-media pushes use the
	// account client directly so they do not pass through the Bot fallback path.
	if len(media) > 1 {
		if channel != nil && channel.TgAccountId > 0 {
			s.appendTelegramJobLog(ctx, job, "inline_send", "skipped", "多媒体不进入Inline链路，改由协议号账号推送")
			return s.submitMessagePushAccountTask(ctx, job, channel, "account")
		}
		return gerror.New("多媒体推送目标未配置TG账号")
	}
	// Quick push Inline always uses the configured official Bot. job.BotId is
	// the source/profile Bot and is commonly zero for group quick-push jobs;
	// requiring it incorrectly skips Inline and sends the task through the
	// Bot/account fallback path even when the template has no media.
	if inlineValidationErr == nil && channel != nil && channel.TgAccountId > 0 {
		return s.submitMessagePushAccountTask(ctx, job, channel, "inline")
	}
	inlineSkipReason := "目标缺少TG账号或可用Bot"
	if inlineValidationErr != nil {
		inlineSkipReason = inlineValidationErr.Error()
	}
	s.appendTelegramJobLog(ctx, job, "inline_send", "skipped", "未进入Inline账号任务，继续由Bot直接发送："+inlineSkipReason)
	if !messageTemplateRequiresSanitizedUpload(media) {
		if messages, copied, copyErr := s.copyOriginalMessageTemplateByBot(ctx, job, template); copied {
			if copyErr == nil {
				if err := s.completeMessagePushJob(ctx, job, messages, "更新原消息复制任务状态失败"); err != nil {
					return err
				}
				s.appendTelegramJobLog(ctx, job, "source_copy", sysin.MessagePushStatusSent, "Inline不可用，已复制原Telegram消息，完整保留格式、自定义Emoji和媒体组")
				return nil
			}
			s.appendTelegramJobLog(ctx, job, "source_copy", "fallback", "Bot复制原消息失败，改用Bot本地媒体上传："+copyErr.Error())
		}
	}
	if !s.botCanAccessChat(ctx, job.TargetChatId) {
		if channel != nil && channel.TgAccountId > 0 {
			s.appendTelegramJobLog(ctx, job, "bot_upload", "skipped", "官方Bot不在目标群或不具备发送权限，提交协议号降级任务")
			return s.submitMessagePushAccountTask(ctx, job, channel, "account")
		}
		return gerror.New("官方Bot不在目标群或不具备发送权限，且目标未配置TG账号")
	}
	messages, botErr := s.sendMessageTemplateByBot(ctx, job, template, media)
	if botErr == nil {
		if err := s.completeMessagePushJob(ctx, job, messages, "更新Bot消息推送任务状态失败"); err != nil {
			return err
		}
		s.appendTelegramJobLog(ctx, job, "bot_upload", sysin.MessagePushStatusSent, "原消息复制失败，已由Bot重新上传本地媒体")
		return nil
	} else {
		s.appendTelegramJobLog(ctx, job, "bot_upload", "fallback", "Bot本地上传失败，准备进入受控降级："+botErr.Error())
	}
	if channel != nil && channel.TgAccountId > 0 {
		return s.submitMessagePushAccountTask(ctx, job, channel, "account")
	}
	return gerror.Wrap(botErr, "Bot上传消息失败且目标未配置TG账号，无法执行协议号降级")
}

func channelTgAccountId(channel *messagePushChannel) int64 {
	if channel == nil {
		return 0
	}
	return channel.TgAccountId
}

func templateSerialNo(template *sysin.MessageTemplateModel) string {
	if template == nil {
		return ""
	}
	return strings.TrimSpace(template.SerialNo)
}

func (s *sSysPublish) submitMessagePushAccountTask(ctx context.Context, job telegramJobRecord, channel *messagePushChannel, mode string) error {
	if channel == nil || channel.TgAccountId <= 0 {
		return gerror.New("消息推送目标未配置TG账号")
	}
	taskKey := fmt.Sprintf("message-push-inline:%d", job.Id)
	stage := "inline_send"
	description := "Inline推送链路"
	if mode == "account" {
		taskKey += ":account"
		stage = "account_send"
		description = "协议号降级链路"
	}
	accountTaskId, err := collectorservice.AccountTasks().Submit(ctx, &collectorin.AccountTaskSubmit{
		TenantID: job.TenantId, AccountID: channel.TgAccountId,
		TaskType: collectorin.AccountTaskTypeMessagePushInline,
		TaskKey:  taskKey, Priority: tgJobPriorityUrgent, MaxAttempts: 5,
	})
	if err != nil {
		return gerror.Wrapf(err, "提交%s账号任务失败", description)
	}
	s.appendTelegramJobLog(ctx, job, stage, "queued", fmt.Sprintf("%s已提交到账号服务 accountTaskId:%d tgAccountId:%d", description, accountTaskId, channel.TgAccountId))
	return nil
}

func (s *sSysPublish) sendMessageTemplateByBot(ctx context.Context, job telegramJobRecord, template *sysin.MessageTemplateModel, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	token, err := botService.SysBot().OfficialBotToken(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := s.telegramBot(ctx, token)
	if err != nil {
		return nil, err
	}
	caption := telegramRichTextHTML(template.Text)
	markup := messageTemplateButtonMarkup(template, len(media))
	if len(media) == 0 {
		if strings.TrimSpace(caption) == "" {
			return nil, gerror.New("展示资料和推送文案不能同时为空")
		}
		msg, sendErr := bot.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:      normalizeTelegramChannelChatID(job.TargetChatId),
			Text:        caption,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: markup,
		})
		if sendErr != nil {
			return nil, sendErr
		}
		if msg == nil || msg.ID <= 0 {
			return nil, gerror.New("Bot发送文本未返回Telegram消息记录")
		}
		return []*telegramSentMessage{{MessageId: int64(msg.ID), Purpose: "display"}}, nil
	}
	return s.sendTelegramMediaSet(ctx, bot, normalizeTelegramChannelChatID(job.TargetChatId), "display", caption, media, markup)
}

func messageTemplateButtonMarkup(template *sysin.MessageTemplateModel, mediaCount int) models.ReplyMarkup {
	if template == nil || mediaCount > 1 || strings.TrimSpace(template.ButtonConfig) == "" {
		return nil
	}
	var config sysin.MessageTemplateButtonConfig
	if json.Unmarshal([]byte(template.ButtonConfig), &config) != nil {
		return nil
	}
	if config.Mode == "reply" {
		rows := make([][]models.KeyboardButton, 0, len(config.Rows))
		for _, row := range config.Rows {
			buttons := make([]models.KeyboardButton, 0, len(row))
			for _, button := range row {
				if strings.TrimSpace(button.Text) != "" {
					buttons = append(buttons, models.KeyboardButton{Text: button.Text})
				}
			}
			if len(buttons) > 0 {
				rows = append(rows, buttons)
			}
		}
		if len(rows) > 0 {
			return &models.ReplyKeyboardMarkup{Keyboard: rows, ResizeKeyboard: true, IsPersistent: true}
		}
		return nil
	}
	if config.Mode != "inline" {
		return nil
	}
	rows := make([][]models.InlineKeyboardButton, 0, len(config.Rows))
	for _, row := range config.Rows {
		buttons := make([]models.InlineKeyboardButton, 0, len(row))
		for _, button := range row {
			if strings.TrimSpace(button.Text) == "" || strings.TrimSpace(button.URL) == "" {
				continue
			}
			buttons = append(buttons, models.InlineKeyboardButton{Text: button.Text, URL: inlinePublishButtonURL(button.URL), Style: sysin.TelegramButtonStyle(button.Color)})
		}
		if len(buttons) > 0 {
			rows = append(rows, buttons)
		}
	}
	if len(rows) == 0 {
		return nil
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func (s *sSysPublish) completeMessagePushJob(ctx context.Context, job telegramJobRecord, messages []*telegramSentMessage, errorMessage string) error {
	if len(messages) == 0 {
		return gerror.New("Telegram未返回发送消息记录，任务不能标记为成功")
	}
	if err := s.saveTelegramSentMessages(ctx, job, messages); err != nil {
		return err
	}
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		WhereIn("status", messagePushActiveStatuses()).
		Data(g.Map{
			"status":          sysin.MessagePushStatusSent,
			"dispatch_status": tgDispatchStatusDone,
			"error_message":   "",
			"sent_at":         gtime.Now(),
			"updated_at":      gtime.Now(),
		}).Update()
	if err != nil {
		return gerror.Wrap(err, errorMessage)
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected == 0 {
		return nil
	}
	if recordErr := s.appendPublishSuccessRecord(ctx, job); recordErr != nil {
		g.Log().Warningf(ctx, "保存快速推送成功记录失败 jobId:%d err:%+v", job.Id, recordErr)
	}
	return nil
}

func (s *sSysPublish) copyOriginalMessageTemplateByBot(ctx context.Context, job telegramJobRecord, template *sysin.MessageTemplateModel) ([]*telegramSentMessage, bool, error) {
	recordIds := messageTemplateSourceRecordIds(template)
	if len(recordIds) == 0 {
		return nil, false, nil
	}
	s.appendTelegramJobLog(ctx, job, "source_copy", sysin.MessagePushStatusSending, fmt.Sprintf("尝试复制Telegram原消息 records:%v", recordIds))
	result, err := botService.SysBot().CopyStoredMessages(ctx, &botsysin.StoredMessageCopyInp{
		MessageRecordIds: recordIds,
		TargetChatId:     normalizeTelegramChannelChatID(job.TargetChatId),
	})
	if err != nil {
		return nil, true, err
	}
	if result == nil || len(result.MessageIds) == 0 {
		return nil, true, gerror.New("复制Telegram原消息未返回消息ID")
	}
	messages := make([]*telegramSentMessage, 0, len(result.MessageIds))
	for index, messageId := range result.MessageIds {
		sent := &telegramSentMessage{MessageId: messageId, Purpose: "display"}
		if index < len(template.Media) && template.Media[index] != nil {
			sent.MediaId = template.Media[index].Id
			sent.TgFileId = template.Media[index].TgFileId
			sent.AssetHash = template.Media[index].AssetHash
		}
		messages = append(messages, sent)
	}
	return messages, true, nil
}

func messageTemplateSourceRecordIds(template *sysin.MessageTemplateModel) []int64 {
	if template == nil {
		return nil
	}
	if len(template.Media) == 0 {
		if template.SourceMessageRecordId > 0 {
			return []int64{template.SourceMessageRecordId}
		}
		return nil
	}
	ids := make([]int64, 0, len(template.Media))
	for _, media := range template.Media {
		if media == nil || media.SourceMessageRecordId <= 0 {
			return nil
		}
		ids = append(ids, media.SourceMessageRecordId)
	}
	return ids
}

func messageTemplateRequiresSourcePreservation(template *sysin.MessageTemplateModel) bool {
	if template == nil {
		return false
	}
	text := strings.ToLower(template.Text)
	return strings.Contains(text, "<tg-emoji") || strings.Contains(text, "emoji-id=")
}

func messageTemplateUsesInline(template *sysin.MessageTemplateModel) bool {
	if template == nil || strings.TrimSpace(template.SerialNo) == "" {
		return false
	}
	media := messageTemplateTelegramMedia(template)
	if len(media) == 0 {
		return true
	}
	return len(media) == 1 && media[0].MediaType == "image"
}

func (s *sSysPublish) sendMessageTemplateWithTgClient(ctx context.Context, client *telegram.Client, peer tg.InputPeerClass, caption string, media []*telegramMediaItem, source *messageTemplateForwardSource, tgAccountId int64, templateHash string) ([]*telegramSentMessage, error) {
	if source != nil && len(source.MessageIds) > 0 {
		messages, forwardErr := forwardMessageTemplateWithGotd(ctx, client, source.Peer, peer, source.MessageIds, media)
		if forwardErr == nil && len(messages) > 0 {
			return messages, nil
		}
		if forwardErr != nil {
			if delay, ok := collectMediaFloodWaitDelay(forwardErr); ok {
				g.Log().Warningf(ctx, "复用TG历史消息触发Telegram限流，交给队列等待重试 tgAccountId:%d templateHash:%s wait:%s err:%v", tgAccountId, templateHash, delay, forwardErr)
				return nil, forwardErr
			}
			if isTelegramChannelPermissionError(forwardErr) {
				g.Log().Errorf(ctx, "TG目标频道权限异常，停止历史消息回退上传 tgAccountId:%d templateHash:%s err:%v", tgAccountId, templateHash, forwardErr)
				return nil, forwardErr
			}
			g.Log().Warningf(ctx, "复用TG历史消息转发失败，回退上传 tgAccountId:%d templateHash:%s err:%v", tgAccountId, templateHash, forwardErr)
		}
	}
	sender := gotdmessage.NewSender(client.API())
	return s.sendMessageTemplateWithGotd(ctx, sender.To(peer), caption, media)
}

func messageTemplateRequiresSanitizedUpload(media []*telegramMediaItem) bool {
	for _, item := range media {
		if item != nil && item.AntiScanEnabled {
			return true
		}
	}
	return false
}

func (s *sSysPublish) sendMessageTemplateWithGotd(ctx context.Context, builder *gotdmessage.RequestBuilder, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	if len(media) == 0 {
		if strings.TrimSpace(caption) == "" {
			return nil, gerror.New("消息模板文案和媒体不能同时为空")
		}
		updates, err := builder.StyledText(ctx, gotdhtml.String(nil, caption))
		if err != nil {
			return nil, err
		}
		return gotdSentMessagesFromUpdates(updates, nil), nil
	}
	if len(media) == 1 {
		updates, err := s.sendSingleMessageTemplateMediaWithGotd(ctx, builder, caption, media[0])
		if err != nil {
			return nil, err
		}
		return gotdSentMessagesFromUpdates(updates, media), nil
	}
	if len(media) > telegramMediaGroupMaxItems {
		messages := make([]*telegramSentMessage, 0, len(media))
		for chunkIndex, chunk := range splitTelegramMediaItems(media, telegramMediaGroupMaxItems) {
			chunkCaption := ""
			if chunkIndex == 0 {
				chunkCaption = caption
			}
			chunkMessages, err := s.sendMessageTemplateWithGotd(ctx, builder, chunkCaption, chunk)
			if err != nil {
				return messages, err
			}
			messages = append(messages, chunkMessages...)
		}
		return messages, nil
	}
	album := make([]gotdmessage.MultiMediaOption, 0, len(media))
	for index, item := range media {
		itemCaption := ""
		if index == 0 {
			itemCaption = caption
		}
		option, err := s.gotdMessageMediaAlbumOption(ctx, builder, itemCaption, item)
		if err != nil {
			return nil, err
		}
		album = append(album, option)
	}
	updates, err := builder.Album(ctx, album[0], album[1:]...)
	if err != nil {
		return nil, err
	}
	return gotdSentMessagesFromUpdates(updates, media), nil
}

func (s *sSysPublish) sendSingleMessageTemplateMediaWithGotd(ctx context.Context, builder *gotdmessage.RequestBuilder, caption string, media *telegramMediaItem) (tg.UpdatesClass, error) {
	upload, cleanup, err := gotdMessageUploadOption(ctx, media)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}
	captionOptions := []gotdmessage.StyledTextOption{}
	if strings.TrimSpace(caption) != "" {
		captionOptions = append(captionOptions, gotdhtml.String(nil, caption))
	}
	if media != nil && media.MediaType == "video" {
		return s.sendGotdVideoWithPreview(ctx, builder, upload, cleanup, media, captionOptions...)
	}
	return builder.Upload(upload).Photo(ctx, captionOptions...)
}

func (s *sSysPublish) sendGotdVideoWithPreview(ctx context.Context, builder *gotdmessage.RequestBuilder, upload gotdmessage.UploadOption, cleanup func(), media *telegramMediaItem, caption ...gotdmessage.StyledTextOption) (tg.UpdatesClass, error) {
	posterPath, posterCleanup, posterErr := cachedTelegramVideoPosterFile(ctx, media)
	if posterErr != nil {
		return nil, posterErr
	}
	if posterPath == "" {
		return builder.Upload(upload).Video(ctx, caption...)
	}
	if media.AntiScanEnabled {
		protectedPath, protectedCleanup, protectErr := prepareTelegramAntiScanUploadFile(ctx, media, posterPath, posterCleanup, "thumbnail")
		if protectErr != nil {
			return nil, gerror.Wrap(protectErr, "处理TG视频缩略图防扫图失败")
		}
		posterPath, posterCleanup = protectedPath, protectedCleanup
	}
	if posterCleanup != nil {
		defer posterCleanup()
	}
	file, err := builder.Upload(upload).AsInputFile(ctx)
	if err != nil {
		return nil, err
	}
	thumb, err := builder.Upload(gotdmessage.FromPath(posterPath)).AsInputFile(ctx)
	if err != nil {
		return nil, err
	}
	meta := s.telegramVideoMeta(ctx, media)
	videoAttribute := &tg.DocumentAttributeVideo{SupportsStreaming: true}
	if meta.Width > 0 {
		videoAttribute.W = meta.Width
	}
	if meta.Height > 0 {
		videoAttribute.H = meta.Height
	}
	if meta.Duration > 0 {
		videoAttribute.Duration = float64(meta.Duration)
	}
	video := gotdmessage.UploadedDocument(file, caption...).
		MIME("video/mp4").
		Thumb(thumb).
		Attributes(videoAttribute)
	return builder.Media(ctx, video)
}

func (s *sSysPublish) gotdMessageMediaAlbumOption(ctx context.Context, builder *gotdmessage.RequestBuilder, caption string, media *telegramMediaItem) (gotdmessage.MultiMediaOption, error) {
	upload, cleanup, err := gotdMessageUploadOption(ctx, media)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}
	file, err := builder.Upload(upload).AsInputFile(ctx)
	if err != nil {
		return nil, err
	}
	captionOptions := []gotdmessage.StyledTextOption{}
	if strings.TrimSpace(caption) != "" {
		captionOptions = append(captionOptions, gotdhtml.String(nil, caption))
	}
	if media != nil && media.MediaType == "video" {
		posterPath, posterCleanup, posterErr := cachedTelegramVideoPosterFile(ctx, media)
		if posterErr != nil {
			return nil, posterErr
		}
		if posterPath != "" {
			if media.AntiScanEnabled {
				protectedPath, protectedCleanup, protectErr := prepareTelegramAntiScanUploadFile(ctx, media, posterPath, posterCleanup, "thumbnail")
				if protectErr != nil {
					return nil, protectErr
				}
				posterPath, posterCleanup = protectedPath, protectedCleanup
			}
			if posterCleanup != nil {
				defer posterCleanup()
			}
			thumb, err := builder.Upload(gotdmessage.FromPath(posterPath)).AsInputFile(ctx)
			if err != nil {
				return nil, err
			}
			meta := s.telegramVideoMeta(ctx, media)
			videoAttribute := &tg.DocumentAttributeVideo{SupportsStreaming: true}
			if meta.Width > 0 {
				videoAttribute.W = meta.Width
			}
			if meta.Height > 0 {
				videoAttribute.H = meta.Height
			}
			if meta.Duration > 0 {
				videoAttribute.Duration = float64(meta.Duration)
			}
			return gotdmessage.UploadedDocument(file, captionOptions...).
				MIME("video/mp4").Thumb(thumb).
				Attributes(videoAttribute), nil
		}
		return gotdmessage.Video(file, captionOptions...).SupportsStreaming(), nil
	}
	return gotdmessage.UploadedPhoto(file, captionOptions...), nil
}

func gotdMessageUploadOption(ctx context.Context, media *telegramMediaItem) (gotdmessage.UploadOption, func(), error) {
	if media == nil {
		return nil, nil, gerror.New("媒体文件为空")
	}
	path, cleanup, err := cachedTelegramMediaFile(ctx, media)
	if err != nil {
		return nil, nil, err
	}
	if path != "" {
		return gotdMessageUploadOptionFromPath(ctx, media, path, cleanup)
	}
	return nil, nil, gerror.New("账号推送媒体文件不存在，请重新上传媒体文件")
}

func gotdMessageUploadOptionFromPath(ctx context.Context, media *telegramMediaItem, path string, cleanup func()) (gotdmessage.UploadOption, func(), error) {
	uploadPath := path
	finalCleanup := cleanup
	var err error
	uploadPath, finalCleanup, err = prepareTelegramMediaUploadFile(ctx, media, uploadPath, finalCleanup)
	if err != nil {
		if finalCleanup != nil {
			finalCleanup()
		}
		return nil, nil, err
	}
	if shouldConvertMessagePushImageToJPEG(media, uploadPath) {
		jpegPath, err := convertMessagePushImageToJPEG(ctx, uploadPath)
		if err != nil {
			if finalCleanup != nil {
				finalCleanup()
			}
			return nil, nil, err
		}
		previousCleanup := finalCleanup
		finalCleanup = func() {
			if previousCleanup != nil {
				previousCleanup()
			}
			_ = os.Remove(jpegPath)
		}
		uploadPath = jpegPath
	}
	return gotdmessage.FromPath(uploadPath), finalCleanup, nil
}

func shouldConvertMessagePushImageToJPEG(media *telegramMediaItem, path string) bool {
	if media == nil || !strings.EqualFold(strings.TrimSpace(media.MediaType), "image") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".webp"
}

func convertMessagePushImageToJPEG(ctx context.Context, path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", gerror.Wrap(err, "打开账号推送图片失败")
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return "", gerror.Wrap(err, "解析账号推送图片失败")
	}
	output, err := os.CreateTemp("", "ybp-message-push-*.jpg")
	if err != nil {
		return "", gerror.Wrap(err, "创建账号推送JPEG临时文件失败")
	}
	outputPath := output.Name()
	if err = jpeg.Encode(output, img, &jpeg.Options{Quality: 90}); err != nil {
		_ = output.Close()
		_ = os.Remove(outputPath)
		return "", gerror.Wrap(err, "转换账号推送图片为JPEG失败")
	}
	if err = output.Close(); err != nil {
		_ = os.Remove(outputPath)
		return "", gerror.Wrap(err, "关闭账号推送JPEG临时文件失败")
	}
	g.Log().Infof(ctx, "账号推送图片已转换为JPEG path:%s", path)
	return outputPath, nil
}

func messagePushInputPeer(channel *messagePushChannel) (tg.InputPeerClass, error) {
	if channel == nil {
		return nil, gerror.New("推送目标不能为空")
	}
	channelId, err := strconv.ParseInt(strings.TrimSpace(channel.TargetChatId), 10, 64)
	if err != nil {
		return nil, gerror.New("推送目标Chat ID无效")
	}
	if channel.IsBroadcast != 1 && channel.IsMegagroup != 1 {
		if channelId < 0 {
			channelId = -channelId
		}
		if channelId <= 0 {
			return nil, gerror.New("推送目标群聊ID无效")
		}
		return &tg.InputPeerChat{ChatID: channelId}, nil
	}
	channelId, err = messagePushInputChannelID(channel.TargetChatId)
	if err != nil {
		return nil, err
	}
	accessHash, err := strconv.ParseInt(strings.TrimSpace(channel.AccessHash), 10, 64)
	if err != nil {
		return nil, gerror.New("推送目标AccessHash无效，请刷新群聊 / 频道缓存")
	}
	return &tg.InputPeerChannel{ChannelID: channelId, AccessHash: accessHash}, nil
}

func messagePushInputChannelID(targetChatId string) (int64, error) {
	value := strings.TrimSpace(targetChatId)
	value = strings.TrimPrefix(value, "-")
	value = strings.TrimPrefix(value, "100")
	channelId, err := strconv.ParseInt(value, 10, 64)
	if err != nil || channelId <= 0 {
		return 0, gerror.New("推送目标频道ID无效")
	}
	return channelId, nil
}

func gotdSentMessagesFromUpdates(updates tg.UpdatesClass, media []*telegramMediaItem) []*telegramSentMessage {
	if short, ok := updates.(*tg.UpdateShortSentMessage); ok && short.ID > 0 {
		msg := &telegramSentMessage{
			MessageId: int64(short.ID),
			Purpose:   "display",
		}
		if len(media) > 0 && media[0] != nil {
			msg.MediaId = media[0].Id
			msg.AssetHash = media[0].AssetHash
		}
		return []*telegramSentMessage{msg}
	}
	updatesList := collectUpdatesList(updates)
	messages := make([]*telegramSentMessage, 0, len(updatesList))
	for _, update := range updatesList {
		var msg *tg.Message
		switch item := update.(type) {
		case *tg.UpdateNewChannelMessage:
			msg, _ = item.Message.(*tg.Message)
		case *tg.UpdateNewMessage:
			msg, _ = item.Message.(*tg.Message)
		}
		if msg == nil || msg.ID <= 0 {
			continue
		}
		index := len(messages)
		mediaId := int64(0)
		assetHash := ""
		if index < len(media) && media[index] != nil {
			mediaId = media[index].Id
			assetHash = media[index].AssetHash
		}
		groupedId := ""
		if value, ok := msg.GetGroupedID(); ok && value > 0 {
			groupedId = strconv.FormatInt(value, 10)
		}
		messages = append(messages, &telegramSentMessage{
			MessageId:    int64(msg.ID),
			MediaGroupId: groupedId,
			Purpose:      "display",
			MediaId:      mediaId,
			AssetHash:    assetHash,
		})
	}
	return messages
}

func forwardMessageTemplateWithGotd(ctx context.Context, client *telegram.Client, fromPeer tg.InputPeerClass, toPeer tg.InputPeerClass, messageIds []int, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	if client == nil || fromPeer == nil || toPeer == nil || len(messageIds) == 0 {
		return nil, gerror.New("TG历史消息转发参数不完整")
	}
	updates, err := client.API().MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
		FromPeer:   fromPeer,
		ToPeer:     toPeer,
		ID:         messageIds,
		RandomID:   collectForwardRandomIds(messageIds),
		Silent:     true,
		DropAuthor: true,
	})
	if err != nil {
		return nil, err
	}
	return gotdSentMessagesFromUpdates(updates, media), nil
}

func (s *sSysPublish) messageTemplateForwardSource(ctx context.Context, tgAccountId int64, templateHash string) (*messageTemplateForwardSource, error) {
	templateHash = strings.TrimSpace(templateHash)
	if tgAccountId <= 0 || templateHash == "" {
		return nil, nil
	}
	var jobs []*messageTemplateForwardSourceJob
	err := g.DB().Model(publishTgJobTable+" j").Safe().Ctx(ctx).
		Fields("j.id,j.tenant_id,j.target_chat_id").
		Where("j.account_id", tgAccountId).
		Where("j.status", sysin.MessagePushStatusSent).
		Where("j.operation_no LIKE ?", "message_push%:"+templateHash).
		Where("EXISTS (SELECT 1 FROM "+publishTgJobLogTable+" l WHERE l.job_id=j.id AND l.action=? AND l.status=?)", "message_push", sysin.MessagePushStatusSent).
		OrderDesc("j.id").
		Limit(10).
		Scan(&jobs)
	if err != nil {
		return nil, gerror.Wrap(err, "读取消息模板复用任务失败")
	}
	for _, job := range jobs {
		if job == nil || job.Id <= 0 {
			continue
		}
		cache, cacheErr := s.tgChannelCacheByChannelId(ctx, job.TenantId, tgAccountId, job.TargetChatId)
		if cacheErr != nil || cache == nil {
			continue
		}
		sourceChannel := &messagePushChannel{
			TgAccountId:  tgAccountId,
			AccessHash:   cache.AccessHash,
			TargetChatId: cache.ChannelId,
			IsBroadcast:  cache.IsBroadcast,
			IsMegagroup:  cache.IsMegagroup,
		}
		peer, peerErr := messagePushInputPeer(sourceChannel)
		if peerErr != nil {
			continue
		}
		var rows []*messageTemplateForwardSourceMessage
		if err = g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
			Fields("tg_message_id AS message_id").
			Where("job_id", job.Id).
			Where("status", sysin.MessagePushStatusSent).
			Where("tg_message_id>?", 0).
			OrderAsc("id").
			Scan(&rows); err != nil {
			return nil, gerror.Wrap(err, "读取消息模板复用消息失败")
		}
		messageIds := make([]int, 0, len(rows))
		for _, row := range rows {
			if row == nil || row.MessageId <= 0 {
				continue
			}
			messageIds = append(messageIds, int(row.MessageId))
		}
		if len(messageIds) == 0 {
			continue
		}
		return &messageTemplateForwardSource{
			Peer:       peer,
			MessageIds: messageIds,
		}, nil
	}
	return nil, nil
}

func (s *sSysPublish) createMessagePushJob(ctx context.Context, template *sysin.MessageTemplateModel, channel *messagePushChannel, tenantId int64, accountId int64, botId int64) (telegramJobRecord, error) {
	now := gtime.Now()
	targetKey := strings.NewReplacer("-", "", ":", "", "@", "").Replace(channel.TargetChatId)
	operationNo := fmt.Sprintf("message_push:%d:%d:%d:%s:%s", template.Id, now.TimestampNano(), channel.Id, targetKey, messageTemplateHash(template))
	return s.createMessagePushJobWithOperation(ctx, template, channel, tenantId, accountId, botId, operationNo, tgJobPriorityUrgent, tgQueueNameUrgent, tgDispatchStatusProcessing, now)
}

func messagePushManualOperationNo(template *sysin.MessageTemplateModel, channel *messagePushChannel) string {
	targetChatId := ""
	channelId := int64(0)
	if channel != nil {
		targetChatId = channel.TargetChatId
		channelId = channel.Id
	}
	templateId := int64(0)
	if template != nil {
		templateId = template.Id
	}
	targetKey := strings.NewReplacer("-", "", ":", "", "@", "").Replace(normalizeTelegramChannelChatID(targetChatId))
	return fmt.Sprintf("message_push:%d:%d:%d:%s:%s", templateId, gtime.Now().TimestampNano(), channelId, targetKey, messageTemplateHash(template))
}

func (s *sSysPublish) createQueuedMessagePushJob(ctx context.Context, template *sysin.MessageTemplateModel, channel *messagePushChannel, tenantId int64, accountId int64, botId int64, operationNo string) (telegramJobRecord, error) {
	return s.createQueuedMessagePushJobWithPriority(ctx, template, channel, tenantId, accountId, botId, operationNo, tgJobPriorityBulk, tgQueueNameBulk)
}

func (s *sSysPublish) createQueuedMessagePushJobWithPriority(ctx context.Context, template *sysin.MessageTemplateModel, channel *messagePushChannel, tenantId int64, accountId int64, botId int64, operationNo string, priority int, queueName string) (telegramJobRecord, error) {
	return s.createMessagePushJobWithOperation(ctx, template, channel, tenantId, accountId, botId, operationNo, priority, queueName, tgDispatchStatusIdle, gtime.Now())
}

func (s *sSysPublish) createMessagePushJobWithOperation(ctx context.Context, template *sysin.MessageTemplateModel, channel *messagePushChannel, tenantId int64, accountId int64, botId int64, operationNo string, priority int, queueName string, dispatchStatus string, now *gtime.Time) (telegramJobRecord, error) {
	if strings.TrimSpace(operationNo) == "" {
		return telegramJobRecord{}, gerror.New("消息推送操作号不能为空")
	}
	queueName = telegramQueueNameByPriorityAndChannel(ctx, priority, channel.Id)
	if existing, err := s.messagePushJobByOperation(ctx, operationNo, channel.Id); err != nil {
		return telegramJobRecord{}, err
	} else if existing.Id > 0 {
		return existing, nil
	}
	data := g.Map{
		"tenant_id":       tenantId,
		"merchant_id":     tenantId,
		"account_id":      accountId,
		"profile_id":      0,
		"channel_id":      channel.Id,
		"bot_id":          botId,
		"target_chat_id":  normalizeTelegramChannelChatID(channel.TargetChatId),
		"status":          sysin.MessagePushStatusPending,
		"operation_no":    operationNo,
		"priority":        priority,
		"queue_name":      queueName,
		"dispatch_status": dispatchStatus,
		"created_at":      now,
		"updated_at":      now,
	}
	id, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		if existing, findErr := s.messagePushJobByOperation(ctx, operationNo, channel.Id); findErr == nil && existing.Id > 0 {
			return existing, nil
		}
		return telegramJobRecord{}, gerror.Wrap(err, "创建消息推送任务失败")
	}
	job := telegramJobRecord{
		Id:             id,
		TaskId:         0,
		OperationNo:    operationNo,
		TenantId:       tenantId,
		AccountId:      accountId,
		ChannelId:      channel.Id,
		BotId:          botId,
		TargetChatId:   normalizeTelegramChannelChatID(channel.TargetChatId),
		Status:         sysin.MessagePushStatusPending,
		Priority:       priority,
		QueueName:      queueName,
		DispatchStatus: dispatchStatus,
	}
	if supersedeErr := s.supersedeOlderMessagePushJobs(ctx, job); supersedeErr != nil {
		g.Log().Warningf(ctx, "废弃旧快速推送任务失败 jobId:%d err:%+v", job.Id, supersedeErr)
	}
	if recordErr := s.upsertPublishJobRecord(ctx, job, "pending", ""); recordErr != nil {
		g.Log().Warningf(ctx, "保存快速推送待发送记录失败 jobId:%d err:%+v", job.Id, recordErr)
	}
	return job, nil
}

func (s *sSysPublish) messagePushJobByOperation(ctx context.Context, operationNo string, channelId int64) (telegramJobRecord, error) {
	var job telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("operation_no", operationNo).
		Where("channel_id", channelId).
		Scan(&job)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return job, nil
		}
		return job, gerror.Wrap(err, "读取消息推送任务失败")
	}
	return job, nil
}

func (s *sSysPublish) handleMessagePushQueuedJobError(ctx context.Context, job telegramJobRecord, err error) error {
	if isTelegramNetworkRetryError(err) {
		s.clearTelegramBotCache()
	}
	decision := telegramJobFailureNextState(err, job.RetryCount)
	result, _ := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).
		WhereIn("status", messagePushActiveStatuses()).
		Data(telegramJobFailureUpdateData(decision, gtime.Now())).Update()
	if result != nil {
		if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected == 0 {
			return nil
		}
	}
	if recordErr := s.upsertPublishJobRecord(ctx, job, decision.Status, decision.Message); recordErr != nil {
		g.Log().Warningf(ctx, "更新快速推送失败记录失败 jobId:%d status:%s err:%+v", job.Id, decision.Status, recordErr)
	}
	s.appendTelegramJobLog(ctx, job, "message_push", decision.Status, decision.Message)
	if isTelegramPermanentAccountAuthError(err) {
		s.handleMessagePushPermanentAuthError(ctx, job, decision.Message, err)
	}
	return nil
}

func (s *sSysPublish) handleMessagePushPermanentAuthError(ctx context.Context, job telegramJobRecord, message string, cause error) {
	account, err := s.messagePushTgAccountOwner(ctx, job.AccountId, job.TenantId)
	if err != nil {
		g.Log().Warningf(ctx, "读取掉线TG账号失败 jobId:%d tgAccountId:%d err:%+v", job.Id, job.AccountId, err)
		return
	}
	if account.Id <= 0 {
		return
	}
	if strings.TrimSpace(message) == "" {
		message = telegramPermanentAccountAuthMessage(cause)
	}
	s.expireTgAccountSession(ctx, account.Id, account.TenantId, account.AccountId, message)
	if account.AccountId <= 0 {
		return
	}
	text := fmt.Sprintf("TG账号已自动停用。\n\nTG账号：%s", html.EscapeString(firstNonEmpty(account.DisplayName, account.TelegramUsername, fmt.Sprintf("ID:%d", account.Id))))
	if strings.TrimSpace(message) != "" {
		text += "\n原因：" + html.EscapeString(message)
	}
	if notifyErr := botService.SysBot().NotifyAccount(ctx, &botsysin.NotifyAccountInp{
		App:       consts.AppApi,
		AccountId: account.AccountId,
		Text:      text,
		ParseMode: "HTML",
	}); notifyErr != nil {
		g.Log().Warningf(ctx, "发送TG账号掉线通知失败 tgAccountId:%d accountId:%d err:%+v", account.Id, account.AccountId, notifyErr)
	}
}

func (s *sSysPublish) messagePushTgAccountOwner(ctx context.Context, tgAccountId int64, tenantId int64) (*messagePushTgAccountOwner, error) {
	if tgAccountId <= 0 {
		return &messagePushTgAccountOwner{}, nil
	}
	var account *messagePushTgAccountOwner
	mod := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Fields("id,tenant_id,account_id,display_name,telegram_username").
		Where("id", tgAccountId).
		WhereNull("deleted_at")
	if tenantId > 0 {
		mod = mod.Where("tenant_id", tenantId)
	}
	if err := mod.Scan(&account); err != nil {
		return nil, gerror.Wrap(err, "读取TG账号失败")
	}
	if account == nil {
		return &messagePushTgAccountOwner{}, nil
	}
	return account, nil
}

func (s *sSysPublish) messagePushChannelFromJob(ctx context.Context, job telegramJobRecord) (*messagePushChannel, error) {
	if strings.TrimSpace(job.TargetChatId) != "" {
		channels, err := s.messagePushCachedTargets(ctx, job.AccountId, []string{normalizeTelegramChannelChatID(job.TargetChatId)}, job.TenantId)
		if err == nil && len(channels) > 0 {
			return channels[0], nil
		}
	}
	if job.ChannelId > 0 {
		channels, err := s.messagePushChannels(ctx, []int64{job.ChannelId}, job.TenantId)
		if err != nil {
			return nil, err
		}
		if len(channels) > 0 {
			return channels[0], nil
		}
	}
	channels, err := s.messagePushCachedTargets(ctx, job.AccountId, []string{normalizeTelegramChannelChatID(job.TargetChatId)}, job.TenantId)
	if err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return nil, gerror.New("推送目标不存在")
	}
	return channels[0], nil
}

func isMessagePushOperationNo(operationNo string) bool {
	return strings.HasPrefix(operationNo, "message_push:") || strings.HasPrefix(operationNo, "message_push_plan:")
}

func messagePushTemplateIdFromOperationNo(operationNo string) (int64, error) {
	parts := strings.Split(operationNo, ":")
	if len(parts) >= 2 && parts[0] == "message_push" {
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err == nil && id > 0 {
			return id, nil
		}
	}
	if len(parts) >= 4 && parts[0] == "message_push_plan" {
		id, err := strconv.ParseInt(parts[3], 10, 64)
		if err == nil && id > 0 {
			return id, nil
		}
	}
	return 0, gerror.New("消息推送操作号无效")
}

func (s *sSysPublish) failMessagePushJob(ctx context.Context, job telegramJobRecord, message string) {
	updated := true
	if job.Id > 0 {
		result, _ := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
			Where("id", job.Id).
			WhereIn("status", messagePushActiveStatuses()).
			Data(g.Map{
				"status":          sysin.MessagePushStatusFailed,
				"dispatch_status": tgDispatchStatusDone,
				"error_message":   message,
				"updated_at":      gtime.Now(),
			}).Update()
		if result != nil {
			if affected, affectedErr := result.RowsAffected(); affectedErr == nil {
				updated = affected > 0
			}
		}
	}
	if !updated {
		return
	}
	if recordErr := s.upsertPublishJobRecord(ctx, job, sysin.MessagePushStatusFailed, message); recordErr != nil {
		g.Log().Warningf(ctx, "更新快速推送失败记录失败 jobId:%d err:%+v", job.Id, recordErr)
	}
	s.appendTelegramJobLog(ctx, job, "message_push", sysin.MessagePushStatusFailed, message)
}

func messagePushActiveStatuses() []string {
	return []string{sysin.MessagePushStatusPending, "sending", "failed_retry"}
}

func (s *sSysPublish) supersedeOlderMessagePushJobs(ctx context.Context, job telegramJobRecord) error {
	if job.Id <= 0 || job.TenantId <= 0 || job.AccountId <= 0 || strings.TrimSpace(job.TargetChatId) == "" {
		return nil
	}
	templateId, err := messagePushTemplateIdFromOperationNo(job.OperationNo)
	if err != nil {
		return nil
	}
	var oldJobs []telegramJobRecord
	if err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Fields("id,task_id,operation_no,tenant_id,account_id,profile_id,channel_id,bot_id,target_chat_id,status").
		Where("tenant_id", job.TenantId).
		Where("account_id", job.AccountId).
		Where("target_chat_id", normalizeTelegramChannelChatID(job.TargetChatId)).
		Where("id < ?", job.Id).
		WhereLike("operation_no", fmt.Sprintf("message_push:%d:%%", templateId)).
		WhereIn("status", messagePushActiveStatuses()).
		Scan(&oldJobs); err != nil {
		return gerror.Wrap(err, "读取旧快速推送任务失败")
	}
	if len(oldJobs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(oldJobs))
	for _, oldJob := range oldJobs {
		ids = append(ids, oldJob.Id)
	}
	message := "已由新的快速推送任务替代"
	data := telegramJobStateUpdateData("superseded", 0, gtime.Now())
	data["error_message"] = message
	data["last_dispatch_error"] = message
	if _, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("id", ids).
		WhereIn("status", messagePushActiveStatuses()).
		Data(data).Update(); err != nil {
		return gerror.Wrap(err, "废弃旧快速推送任务失败")
	}
	if _, err = g.DB().Model(publishSuccessRecordTable).Safe().Ctx(ctx).
		WhereIn("job_id", ids).
		Data(g.Map{"status": "superseded", "message": message}).
		Update(); err != nil {
		return gerror.Wrap(err, "更新旧快速推送记录失败")
	}
	return nil
}

func (s *sSysPublish) messagePushChannels(ctx context.Context, ids []int64, tenantId int64) ([]*messagePushChannel, error) {
	var list []*messagePushChannel
	err := g.DB().Model(publishChannelTable).Safe().Ctx(ctx).
		Fields("id,tg_account_id,channel_title,target_chat_id,bot_id_json").
		WhereIn("id", uniqueIds(ids)).
		Where("tenant_id", tenantId).
		WhereNull("deleted_at").
		OrderAsc("id").
		Scan(&list)
	if err != nil {
		return nil, gerror.Wrap(err, "读取推送目标失败")
	}
	if len(list) == 0 {
		return nil, gerror.New("推送目标不存在")
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		cache, cacheErr := s.tgChannelCacheByChannelId(ctx, tenantId, item.TgAccountId, item.TargetChatId)
		if cacheErr != nil {
			return nil, cacheErr
		}
		item.AccessHash = cache.AccessHash
		item.TargetChatId = cache.ChannelId
		item.IsBroadcast = cache.IsBroadcast
		item.IsMegagroup = cache.IsMegagroup
	}
	if botId, botErr := s.defaultMessagePushBotId(ctx); botErr == nil {
		for _, item := range list {
			if item != nil && firstMessagePushBotId(item) <= 0 {
				item.BotIdJson = fmt.Sprintf("[%d]", botId)
			}
		}
	}
	return list, nil
}

func (s *sSysPublish) messagePushTargets(ctx context.Context, in *sysin.MessageTemplatePushInp, tenantId int64) ([]*messagePushChannel, error) {
	channels := make([]*messagePushChannel, 0, len(in.ChannelIds)+len(in.TargetChatIds))
	if len(in.ChannelIds) > 0 {
		list, err := s.messagePushChannels(ctx, in.ChannelIds, tenantId)
		if err != nil {
			return nil, err
		}
		channels = append(channels, list...)
	}
	if len(in.TargetChatIds) > 0 {
		list, err := s.messagePushCachedTargets(ctx, in.AccountId, in.TargetChatIds, tenantId)
		if err != nil {
			return nil, err
		}
		channels = append(channels, list...)
	}
	if len(channels) == 0 {
		return nil, gerror.New("推送目标不存在")
	}
	return channels, nil
}

func (s *sSysPublish) messagePushCachedTargets(ctx context.Context, tgAccountId int64, targetChatIds []string, tenantId int64) ([]*messagePushChannel, error) {
	var rows []struct {
		Id           int64  `json:"id"`
		ChannelId    string `json:"channelId"`
		AccessHash   string `json:"accessHash"`
		ChannelTitle string `json:"channelTitle"`
		IsBroadcast  int    `json:"isBroadcast"`
		IsMegagroup  int    `json:"isMegagroup"`
	}
	lookupIds := messagePushTargetLookupIds(targetChatIds)
	err := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
		Fields("id,channel_id,access_hash,channel_title,is_broadcast,is_megagroup").
		Where("tenant_id", tenantId).
		Where("tg_account_id", tgAccountId).
		WhereIn("channel_id", lookupIds).
		OrderAsc("id").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取缓存推送目标失败")
	}
	rowMap := make(map[string]*messagePushChannel, len(rows))
	botId, _ := s.defaultMessagePushBotId(ctx)
	for _, row := range rows {
		channel := &messagePushChannel{
			Id:           row.Id,
			TgAccountId:  tgAccountId,
			AccessHash:   row.AccessHash,
			ChannelTitle: row.ChannelTitle,
			IsBroadcast:  row.IsBroadcast,
			IsMegagroup:  row.IsMegagroup,
			TargetChatId: row.ChannelId,
			BotIdJson:    fmt.Sprintf("[%d]", botId),
		}
		for _, id := range tgChannelCacheLookupIds(row.ChannelId) {
			rowMap[id] = channel
		}
	}
	channels := make([]*messagePushChannel, 0, len(targetChatIds))
	for _, targetChatId := range targetChatIds {
		var channel *messagePushChannel
		for _, id := range tgChannelCacheLookupIds(targetChatId) {
			if found := rowMap[id]; found != nil {
				channel = found
				break
			}
		}
		if channel == nil {
			return nil, gerror.New("存在不属于当前TG账号的推送目标，请刷新缓存后重新选择")
		}
		channels = append(channels, channel)
	}
	return channels, nil
}

func (s *sSysPublish) defaultMessagePushBotId(ctx context.Context) (int64, error) {
	var row struct {
		Id int64 `json:"id"`
	}
	if err := g.DB().Model("hg_youban_bot_bot").Safe().Ctx(ctx).
		Fields("id").Where("is_default", 1).Where("status", 1).WhereNull("deleted_at").
		OrderAsc("id").Scan(&row); err != nil {
		return 0, gerror.Wrap(err, "读取默认推送机器人失败")
	}
	if row.Id <= 0 {
		return 0, gerror.New("未配置可用推送机器人")
	}
	return row.Id, nil
}

func (s *sSysPublish) ensureMessagePushTgAccountBelongTenant(ctx context.Context, accountId int64, tenantId int64) error {
	count, err := g.DB().Model(publishTgAccountTable).Safe().Ctx(ctx).
		Where("id", accountId).
		Where("tenant_id", tenantId).
		Where("status", sysin.PublishTgAccountStatusAuthorized).
		WhereNull("deleted_at").
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查TG账号失败")
	}
	if count != 1 {
		return gerror.New("TG账号不存在或已停用")
	}
	return nil
}

func (s *sSysPublish) ensureMessagePushTargetCaches(ctx context.Context, tgAccountId int64, targetChatIds []string, tenantId int64) error {
	channels, err := s.messagePushCachedTargets(ctx, tgAccountId, targetChatIds, tenantId)
	if err != nil {
		return gerror.Wrap(err, "检查推送目标失败")
	}
	if len(channels) != len(targetChatIds) {
		return gerror.New("存在不属于当前TG账号的推送目标")
	}
	return nil
}

func messagePushTargetLookupIds(targetChatIds []string) []string {
	ids := make([]string, 0, len(targetChatIds)*2)
	for _, targetChatId := range targetChatIds {
		ids = append(ids, tgChannelCacheLookupIds(targetChatId)...)
	}
	return uniqueStrings(ids)
}

func firstMessagePushBotId(channel *messagePushChannel) int64 {
	if channel == nil {
		return 0
	}
	ids := decodeBotIds(channel.BotIdJson)
	if len(ids) == 0 {
		return 0
	}
	return ids[0]
}

func messageTemplateTelegramMedia(template *sysin.MessageTemplateModel) []*telegramMediaItem {
	if template == nil || len(template.Media) == 0 {
		return nil
	}
	media := make([]*telegramMediaItem, 0, len(template.Media))
	for _, item := range template.Media {
		if item == nil {
			continue
		}
		media = append(media, &telegramMediaItem{
			Id:                item.Id,
			MediaType:         item.MediaType,
			Purpose:           "display",
			FileUrl:           normalizeMediaFileURL(item.FileUrl, item.StoragePath),
			PosterUrl:         normalizeMediaFileURL(item.PosterUrl, item.PosterStoragePath),
			StoragePath:       item.StoragePath,
			PosterStoragePath: item.PosterStoragePath,
			TgFileId:          item.TgFileId,
			TgThumbFileId:     item.TgThumbFileId,
			AssetHash:         item.AssetHash,
			SortIndex:         item.SortIndex,
		})
	}
	return media
}

func messageTemplateHash(template *sysin.MessageTemplateModel) string {
	if template == nil {
		return ""
	}
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(template.Text))
	builder.WriteByte('\n')
	for _, item := range template.Media {
		if item == nil {
			continue
		}
		builder.WriteString(strconv.FormatInt(item.Id, 10))
		builder.WriteByte('|')
		builder.WriteString(strings.TrimSpace(item.MediaType))
		builder.WriteByte('|')
		builder.WriteString(strings.TrimSpace(item.FileUrl))
		builder.WriteByte('|')
		builder.WriteString(strings.TrimSpace(item.StoragePath))
		builder.WriteByte('|')
		builder.WriteString(strings.TrimSpace(item.PosterUrl))
		builder.WriteByte('|')
		builder.WriteString(strings.TrimSpace(item.PosterStoragePath))
		builder.WriteByte('|')
		builder.WriteString(strings.TrimSpace(item.TgFileId))
		builder.WriteByte('|')
		builder.WriteString(strings.TrimSpace(item.TgThumbFileId))
		builder.WriteByte('|')
		builder.WriteString(strings.TrimSpace(item.AssetHash))
		builder.WriteByte('|')
		builder.WriteString(strconv.Itoa(item.SortIndex))
		builder.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])[:16]
}
