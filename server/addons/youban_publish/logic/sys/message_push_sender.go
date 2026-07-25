package sys

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gotd/td/telegram"
	gotdmessage "github.com/gotd/td/telegram/message"
	gotdhtml "github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/tg"
	_ "golang.org/x/image/webp"

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

func (s *sSysPublish) pushMessageTemplateTargetsWithSeed(ctx context.Context, template *sysin.MessageTemplateModel, targets []*messageTemplatePushTarget, tenantId int64, accountId int64) *sysin.MessageTemplatePushModel {
	res := &sysin.MessageTemplatePushModel{Total: len(targets), Results: make([]*sysin.MessageTemplatePushResultModel, 0, len(targets))}
	seedSentByAccount := make(map[int64]bool)
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
		var result *sysin.MessageTemplatePushResultModel
		if !seedSentByAccount[targetAccountId] {
			result = s.sendMessageTemplateToChannel(ctx, template, target.Channel, tenantId, targetAccountId)
			if result.Status == sysin.MessagePushStatusSent {
				seedSentByAccount[targetAccountId] = true
			}
		} else {
			priority := target.Priority
			if priority <= 0 {
				priority = tgJobPriorityBulk
			}
			queueName := strings.TrimSpace(target.QueueName)
			if queueName == "" {
				queueName = tgQueueNameBulk
			}
			result = s.queueMessageTemplateToChannelWithPriority(ctx, template, target.Channel, tenantId, targetAccountId, target.OperationNo, target.Delay, priority, queueName)
		}
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
	return s.pushMessageTemplateTargetsWithSeed(ctx, template, targets, account.TenantId, in.AccountId), nil
}

func (s *sSysPublish) sendMessageTemplateToChannel(ctx context.Context, template *sysin.MessageTemplateModel, channel *messagePushChannel, tenantId int64, accountId int64) *sysin.MessageTemplatePushResultModel {
	result := &sysin.MessageTemplatePushResultModel{
		ChannelId:    channel.Id,
		TargetChatId: channel.TargetChatId,
		Status:       sysin.MessagePushStatusFailed,
	}
	if channel == nil || strings.TrimSpace(channel.TargetChatId) == "" {
		result.Message = "目标群聊或频道未配置"
		return result
	}
	botId := firstMessagePushBotId(channel)
	job, err := s.createMessagePushJob(ctx, template, channel, tenantId, accountId, botId)
	if err != nil {
		result.Message = err.Error()
		return result
	}
	result.JobId = job.Id
	lease, ok, err := s.tryTelegramChannelLease(ctx, channel.TargetChatId)
	if err != nil {
		s.failMessagePushJob(ctx, job, err.Error())
		result.Message = err.Error()
		return result
	}
	if !ok {
		message := "频道正在发送其他任务，请稍后重试"
		s.failMessagePushJob(ctx, job, message)
		result.Message = message
		return result
	}
	defer s.releaseTelegramChannelLease(ctx, lease)
	if err = s.sendMessagePushJob(ctx, job, template, channel, accountId); err != nil {
		s.failMessagePushJob(ctx, job, err.Error())
		result.Message = err.Error()
		return result
	}
	result.Status = sysin.MessagePushStatusSent
	result.Message = "推送成功"
	return result
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
		delay := s.telegramChannelBusyDelay(ctx, jobId)
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", jobId).Data(g.Map{
			"dispatch_status":     tgDispatchStatusIdle,
			"next_retry_at":       gtime.Now().Add(delay),
			"last_dispatch_error": "频道正在发送其他任务，已等待重新调度",
			"updated_at":          gtime.Now(),
		}).Update()
		return nil
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
	if err = s.sendMessagePushJob(ctx, job, template, channel, job.AccountId); err != nil {
		return s.handleMessagePushQueuedJobError(ctx, job, err)
	}
	return nil
}

func (s *sSysPublish) sendMessagePushJob(ctx context.Context, job telegramJobRecord, template *sysin.MessageTemplateModel, channel *messagePushChannel, tgAccountId int64) error {
	media := messageTemplateTelegramMedia(template)
	// Inline results are single-message results. Any media must use the
	// account sender so mixed media can be delivered as one Telegram group.
	if messageTemplateUsesInline(template) && channel != nil && channel.TgAccountId > 0 && job.BotId > 0 {
		s.appendTelegramJobLog(ctx, job, "inline_send", sysin.MessagePushStatusSending, fmt.Sprintf("尝试Inline推送 serial:%s botId:%d tgAccountId:%d", template.SerialNo, job.BotId, channel.TgAccountId))
		messages, inlineErr := s.sendInlineTemplateByAccount(ctx, channel.TgAccountId, job.BotId, channel, template.SerialNo)
		if inlineErr == nil {
			if err := s.saveTelegramSentMessages(ctx, job, messages); err != nil {
				return err
			}
			_, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{"status": sysin.MessagePushStatusSent, "dispatch_status": tgDispatchStatusDone, "error_message": "", "sent_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
			if err != nil {
				return gerror.Wrap(err, "更新Inline消息推送任务状态失败")
			}
			s.appendTelegramJobLog(ctx, job, "inline_send", sysin.MessagePushStatusSent, "Inline机器人消息模板推送成功")
			return nil
		}
		s.appendTelegramJobLog(ctx, job, "inline_send", "fallback", "Inline推送失败，回退账号直发："+inlineErr.Error())
	} else {
		reason := "模板包含媒体"
		if template == nil || strings.TrimSpace(template.SerialNo) == "" {
			reason = "模板缺少Inline编号"
		} else if channel == nil || channel.TgAccountId <= 0 {
			reason = "目标缺少TG账号"
		} else if job.BotId <= 0 {
			reason = "目标缺少可用机器人"
		}
		s.appendTelegramJobLog(ctx, job, "inline_send", "skipped", "未执行Inline推送："+reason)
	}
	s.appendTelegramJobLog(ctx, job, "message_push", sysin.MessagePushStatusSending, "开始使用账号直发消息模板")
	messages, err := s.sendMessageTemplateByTgAccount(ctx, tgAccountId, channel, telegramRichTextHTML(template.Text), media, messageTemplateHash(template))
	if err != nil {
		return err
	}
	if err = s.saveTelegramSentMessages(ctx, job, messages); err != nil {
		return err
	}
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
		"status":          sysin.MessagePushStatusSent,
		"dispatch_status": tgDispatchStatusDone,
		"error_message":   "",
		"sent_at":         gtime.Now(),
		"updated_at":      gtime.Now(),
	}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新消息推送任务状态失败")
	}
	s.appendTelegramJobLog(ctx, job, "message_push", sysin.MessagePushStatusSent, "账号消息模板推送成功")
	return nil
}

func messageTemplateUsesInline(template *sysin.MessageTemplateModel) bool {
	return template != nil && len(messageTemplateTelegramMedia(template)) == 0 && strings.TrimSpace(template.SerialNo) != ""
}

func (s *sSysPublish) sendMessageTemplateByTgAccount(ctx context.Context, tgAccountId int64, channel *messagePushChannel, caption string, media []*telegramMediaItem, templateHash string) ([]*telegramSentMessage, error) {
	if channel == nil {
		return nil, gerror.New("推送目标不能为空")
	}
	peer, err := messagePushInputPeer(channel)
	if err != nil {
		return nil, err
	}
	source, err := s.messageTemplateForwardSource(ctx, tgAccountId, templateHash)
	if err != nil {
		g.Log().Warningf(ctx, "读取消息模板复用源失败 tgAccountId:%d templateHash:%s err:%+v", tgAccountId, templateHash, err)
	}
	var sent []*telegramSentMessage
	send := func(runCtx context.Context, client *telegram.Client) error {
		if source != nil && len(source.MessageIds) > 0 {
			messages, forwardErr := forwardMessageTemplateWithGotd(runCtx, client, source.Peer, peer, source.MessageIds, media)
			if forwardErr == nil && len(messages) > 0 {
				sent = messages
				return nil
			}
			if forwardErr != nil {
				if delay, ok := collectMediaFloodWaitDelay(forwardErr); ok {
					g.Log().Warningf(runCtx, "复用TG历史消息触发Telegram限流，交给队列等待重试 tgAccountId:%d templateHash:%s wait:%s err:%v", tgAccountId, templateHash, delay, forwardErr)
					return forwardErr
				}
				g.Log().Warningf(runCtx, "复用TG历史消息转发失败，回退上传 tgAccountId:%d templateHash:%s err:%v", tgAccountId, templateHash, forwardErr)
			}
		}
		sender := gotdmessage.NewSender(client.API())
		messages, sendErr := sendMessageTemplateWithGotd(runCtx, sender.To(peer), caption, media)
		if sendErr != nil {
			return sendErr
		}
		sent = messages
		return nil
	}
	usedRuntime, err := s.executeAccountCollectOperation(ctx, tgAccountId, 2*time.Minute, send)
	if err != nil {
		return nil, err
	}
	if usedRuntime {
		if len(sent) == 0 {
			return nil, gerror.New("账号推送已调用，但未读取到TG消息结果")
		}
		return sent, nil
	}
	account, err := s.accountCollectTgAccount(ctx, tgAccountId)
	if err != nil {
		return nil, err
	}
	conf, err := NewSysConfig().GetTelegram(ctx)
	if err != nil {
		return nil, err
	}
	client, err := s.newAccountCollectClient(ctx, conf, account, tg.NewUpdateDispatcher())
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	err = client.Run(runCtx, func(runCtx context.Context) error {
		return send(runCtx, client)
	})
	if err != nil {
		return nil, err
	}
	if len(sent) == 0 {
		return nil, gerror.New("账号推送已调用，但未读取到TG消息结果")
	}
	return sent, nil
}

func sendMessageTemplateWithGotd(ctx context.Context, builder *gotdmessage.RequestBuilder, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
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
		updates, err := sendSingleMessageTemplateMediaWithGotd(ctx, builder, caption, media[0])
		if err != nil {
			return nil, err
		}
		return gotdSentMessagesFromUpdates(updates, media), nil
	}
	album := make([]gotdmessage.MultiMediaOption, 0, len(media))
	for index, item := range media {
		itemCaption := ""
		if index == 0 {
			itemCaption = caption
		}
		option, err := gotdMessageMediaAlbumOption(ctx, builder, itemCaption, item)
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

func sendSingleMessageTemplateMediaWithGotd(ctx context.Context, builder *gotdmessage.RequestBuilder, caption string, media *telegramMediaItem) (tg.UpdatesClass, error) {
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
		return builder.Upload(upload).Video(ctx, captionOptions...)
	}
	return builder.Upload(upload).Photo(ctx, captionOptions...)
}

func gotdMessageMediaAlbumOption(ctx context.Context, builder *gotdmessage.RequestBuilder, caption string, media *telegramMediaItem) (gotdmessage.MultiMediaOption, error) {
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
	if err := ensureTelegramOperationColumns(ctx); err != nil {
		return telegramJobRecord{}, err
	}
	if strings.TrimSpace(operationNo) == "" {
		return telegramJobRecord{}, gerror.New("消息推送操作号不能为空")
	}
	if existing, err := s.messagePushJobByOperation(ctx, operationNo, channel.Id); err != nil {
		return telegramJobRecord{}, err
	} else if existing.Id > 0 {
		return existing, nil
	}
	data := g.Map{
		"task_id":         0,
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
	return telegramJobRecord{
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
	}, nil
}

func (s *sSysPublish) messagePushJobByOperation(ctx context.Context, operationNo string, channelId int64) (telegramJobRecord, error) {
	var job telegramJobRecord
	err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("task_id", 0).
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
	retryCount := job.RetryCount + 1
	policy := telegramJobErrorRetryPolicy(err, retryCount)
	status := sysin.MessagePushStatusFailed
	dispatchStatus := tgDispatchStatusDone
	var nextRetryAt any
	if !policy.Permanent {
		status = "failed_retry"
		dispatchStatus = tgDispatchStatusIdle
		nextRetryAt = gtime.Now().Add(policy.RetryDelay)
	}
	_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
		"status":              status,
		"dispatch_status":     dispatchStatus,
		"retry_count":         retryCount,
		"next_retry_at":       nextRetryAt,
		"error_message":       policy.Message,
		"last_dispatch_error": policy.Message,
		"updated_at":          gtime.Now(),
	}).Update()
	s.appendTelegramJobLog(ctx, job, "message_push", status, policy.Message)
	if isTelegramPermanentAccountAuthError(err) {
		s.handleMessagePushPermanentAuthError(ctx, job, policy.Message, err)
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
	if job.Id > 0 {
		_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).Where("id", job.Id).Data(g.Map{
			"status":          sysin.MessagePushStatusFailed,
			"dispatch_status": tgDispatchStatusDone,
			"error_message":   message,
			"updated_at":      gtime.Now(),
		}).Update()
	}
	s.appendTelegramJobLog(ctx, job, "message_push", sysin.MessagePushStatusFailed, message)
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
