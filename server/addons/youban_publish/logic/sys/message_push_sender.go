package sys

import (
	"context"
	"fmt"
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

	"hotgo/addons/youban_publish/model/input/sysin"
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
	res = &sysin.MessageTemplatePushModel{
		Total:   len(channels),
		Results: make([]*sysin.MessageTemplatePushResultModel, 0, len(channels)),
	}
	for _, channel := range channels {
		result := s.sendMessageTemplateToChannel(ctx, template, channel, account.TenantId, in.AccountId)
		res.Results = append(res.Results, result)
		if result.Status == sysin.MessagePushStatusSent {
			res.Success++
		} else {
			res.Failed++
		}
	}
	return res, nil
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

func (s *sSysPublish) queueMessageTemplateToChannel(ctx context.Context, template *sysin.MessageTemplateModel, channel *messagePushChannel, tenantId int64, accountId int64, operationNo string, delay time.Duration) *sysin.MessageTemplatePushResultModel {
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
	job, err := s.createQueuedMessagePushJob(ctx, template, channel, tenantId, accountId, botId, operationNo)
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
	s.appendTelegramJobLog(ctx, job, "message_push", sysin.MessagePushStatusSending, "开始使用账号推送消息模板")
	media := messageTemplateTelegramMedia(template)
	messages, err := s.sendMessageTemplateByTgAccount(ctx, tgAccountId, channel, telegramRichTextHTML(template.Text), media)
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

func (s *sSysPublish) sendMessageTemplateByTgAccount(ctx context.Context, tgAccountId int64, channel *messagePushChannel, caption string, media []*telegramMediaItem) ([]*telegramSentMessage, error) {
	if channel == nil {
		return nil, gerror.New("推送目标不能为空")
	}
	peer, err := messagePushInputPeer(channel)
	if err != nil {
		return nil, err
	}
	var sent []*telegramSentMessage
	send := func(runCtx context.Context, client *telegram.Client) error {
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
		var sendErr error
		sent, sendErr = sendMessageTemplateWithGotd(runCtx, gotdmessage.NewSender(client.API()).To(peer), caption, media)
		return sendErr
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
	upload, err := gotdMessageUploadOption(media)
	if err != nil {
		return nil, err
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
	upload, err := gotdMessageUploadOption(media)
	if err != nil {
		return nil, err
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

func gotdMessageUploadOption(media *telegramMediaItem) (gotdmessage.UploadOption, error) {
	if media == nil {
		return nil, gerror.New("媒体文件为空")
	}
	if path := strings.TrimSpace(media.StoragePath); path != "" {
		localPath := resolveTelegramLocalPath(path)
		if fileExists(localPath) {
			return gotdmessage.FromPath(localPath), nil
		}
	}
	return nil, gerror.New("账号推送暂不支持远程媒体地址，请重新上传媒体文件")
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
	if channelId < 0 {
		value := strings.TrimPrefix(strconv.FormatInt(channelId, 10), "-100")
		channelId, err = strconv.ParseInt(value, 10, 64)
		if err != nil || channelId <= 0 {
			return nil, gerror.New("推送目标频道ID无效")
		}
	}
	accessHash, err := strconv.ParseInt(strings.TrimSpace(channel.AccessHash), 10, 64)
	if err != nil {
		return nil, gerror.New("推送目标AccessHash无效，请刷新群聊 / 频道缓存")
	}
	return &tg.InputPeerChannel{ChannelID: channelId, AccessHash: accessHash}, nil
}

func gotdSentMessagesFromUpdates(updates tg.UpdatesClass, media []*telegramMediaItem) []*telegramSentMessage {
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

func (s *sSysPublish) createMessagePushJob(ctx context.Context, template *sysin.MessageTemplateModel, channel *messagePushChannel, tenantId int64, accountId int64, botId int64) (telegramJobRecord, error) {
	now := gtime.Now()
	targetKey := strings.NewReplacer("-", "", ":", "", "@", "").Replace(channel.TargetChatId)
	operationNo := fmt.Sprintf("message_push:%d:%d:%d:%s", template.Id, now.TimestampNano(), channel.Id, targetKey)
	return s.createMessagePushJobWithOperation(ctx, template, channel, tenantId, accountId, botId, operationNo, tgJobPriorityUrgent, tgQueueNameUrgent, tgDispatchStatusProcessing, now)
}

func (s *sSysPublish) createQueuedMessagePushJob(ctx context.Context, template *sysin.MessageTemplateModel, channel *messagePushChannel, tenantId int64, accountId int64, botId int64, operationNo string) (telegramJobRecord, error) {
	return s.createMessagePushJobWithOperation(ctx, template, channel, tenantId, accountId, botId, operationNo, tgJobPriorityBulk, tgQueueNameBulk, tgDispatchStatusIdle, gtime.Now())
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
	return nil
}

func (s *sSysPublish) messagePushChannelFromJob(ctx context.Context, job telegramJobRecord) (*messagePushChannel, error) {
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
		ChannelId    string `json:"channelId"`
		AccessHash   string `json:"accessHash"`
		ChannelTitle string `json:"channelTitle"`
		IsBroadcast  int    `json:"isBroadcast"`
		IsMegagroup  int    `json:"isMegagroup"`
	}
	err := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
		Fields("channel_id,access_hash,channel_title,is_broadcast,is_megagroup").
		Where("tenant_id", tenantId).
		Where("tg_account_id", tgAccountId).
		WhereIn("channel_id", targetChatIds).
		OrderAsc("id").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取缓存推送目标失败")
	}
	if len(rows) != len(targetChatIds) {
		return nil, gerror.New("存在不属于当前TG账号的推送目标，请刷新缓存后重新选择")
	}
	channels := make([]*messagePushChannel, 0, len(rows))
	for _, row := range rows {
		channels = append(channels, &messagePushChannel{
			TgAccountId:  tgAccountId,
			AccessHash:   row.AccessHash,
			ChannelTitle: row.ChannelTitle,
			IsBroadcast:  row.IsBroadcast,
			IsMegagroup:  row.IsMegagroup,
			TargetChatId: row.ChannelId,
		})
	}
	return channels, nil
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
	count, err := g.DB().Model(publishTgChannelTable).Safe().Ctx(ctx).
		Where("tenant_id", tenantId).
		Where("tg_account_id", tgAccountId).
		WhereIn("channel_id", targetChatIds).
		Count()
	if err != nil {
		return gerror.Wrap(err, "检查推送目标失败")
	}
	if count != len(targetChatIds) {
		return gerror.New("存在不属于当前TG账号的推送目标")
	}
	return nil
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
