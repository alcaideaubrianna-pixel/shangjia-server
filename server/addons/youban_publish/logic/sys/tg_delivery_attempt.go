package sys

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const (
	publishTgAttemptTable          = "hg_youban_publish_tg_attempt"
	publishTgAttemptMessageTable   = "hg_youban_publish_tg_attempt_message"
	telegramAttemptStatusSending   = "sending"
	telegramAttemptStatusWaiting   = "awaiting_webhook"
	telegramAttemptStatusConfirmed = "confirmed"
	telegramAttemptStatusRetryWait = "retry_wait"
	telegramAttemptWebhookWait     = 15 * time.Second
	telegramAttemptLateGrace       = 15 * time.Second
	telegramAttemptMaxRetries      = 3
)

var errTelegramAwaitingWebhook = gerror.New("等待Telegram Webhook确认")

type telegramDeliveryAttempt struct {
	Id              int64       `json:"id"`
	JobId           int64       `json:"jobId"`
	TenantId        int64       `json:"tenantId"`
	BotId           int64       `json:"botId"`
	ChannelId       int64       `json:"channelId"`
	TargetChatId    string      `json:"targetChatId"`
	Phase           string      `json:"phase"`
	ChunkIndex      int         `json:"chunkIndex"`
	AttemptNo       int         `json:"attemptNo"`
	AttemptToken    string      `json:"attemptToken"`
	ExpectedCount   int         `json:"expectedCount"`
	ConfirmedCount  int         `json:"confirmedCount"`
	Status          string      `json:"status"`
	WebhookDeadline *gtime.Time `json:"webhookDeadline"`
}

func (s *sSysPublish) beginTelegramDeliveryAttempt(ctx context.Context, job telegramJobRecord, phase string, expectedCount int) (telegramDeliveryAttempt, error) {
	var attempt telegramDeliveryAttempt
	value, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Fields("COALESCE(MAX(attempt_no),0)").
		Where("job_id", job.Id).Where("phase", phase).Where("chunk_index", 0).Value()
	if err != nil {
		return attempt, gerror.Wrap(err, "读取TG发送尝试次数失败")
	}
	attempt = telegramDeliveryAttempt{
		JobId: job.Id, TenantId: job.TenantId, BotId: job.BotId, ChannelId: job.ChannelId,
		TargetChatId: normalizeTelegramChannelChatID(job.TargetChatId), Phase: phase,
		AttemptNo: value.Int() + 1, AttemptToken: guid.S(), ExpectedCount: expectedCount,
		Status: telegramAttemptStatusSending,
	}
	attempt.Id, err = g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).Data(g.Map{
		"job_id": attempt.JobId, "tenant_id": attempt.TenantId, "bot_id": attempt.BotId,
		"channel_id": attempt.ChannelId, "target_chat_id": attempt.TargetChatId, "phase": attempt.Phase,
		"chunk_index": attempt.ChunkIndex, "attempt_no": attempt.AttemptNo, "attempt_token": attempt.AttemptToken,
		"expected_count": attempt.ExpectedCount, "confirmed_count": 0, "status": attempt.Status,
		"created_at": gtime.Now(), "updated_at": gtime.Now(),
	}).InsertAndGetId()
	if err != nil {
		return attempt, gerror.Wrap(err, "创建TG发送Attempt失败")
	}
	return attempt, nil
}

func telegramAttemptMarker(token string) string {
	sum := sha256.Sum256([]byte("attempt:" + strings.TrimSpace(token)))
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

func telegramTextHasInternalDeliveryMarker(text string) bool {
	return strings.Contains(text, "\u2063") && strings.Contains(text, "\u2064")
}

func (s *sSysPublish) confirmTelegramDeliveryAttempt(ctx context.Context, attempt telegramDeliveryAttempt, messages []*telegramSentMessage, response bool) error {
	if err := s.recordTelegramAttemptMessages(ctx, attempt, messages, "response"); err != nil {
		return err
	}
	count := len(messages)
	if attempt.ExpectedCount == 0 {
		count = 0
	}
	data := g.Map{"status": telegramAttemptStatusConfirmed, "confirmed_count": count, "webhook_deadline": nil, "updated_at": gtime.Now()}
	if response {
		data["response_received"] = 1
	}
	_, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("id", attempt.Id).WhereIn("status", []string{telegramAttemptStatusSending, telegramAttemptStatusWaiting}).Data(data).Update()
	return err
}

func (s *sSysPublish) waitTelegramDeliveryWebhook(ctx context.Context, attempt telegramDeliveryAttempt, cause error) error {
	deadline := gtime.Now().Add(telegramAttemptWebhookWait)
	result, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("id", attempt.Id).Where("status", telegramAttemptStatusSending).
		Data(g.Map{"status": telegramAttemptStatusWaiting, "webhook_deadline": deadline, "error_message": cause.Error(), "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "设置TG Attempt等待Webhook失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		current, readErr := s.telegramAttemptById(ctx, attempt.Id)
		if readErr == nil && current.Status == telegramAttemptStatusConfirmed {
			phase := telegramSendPhaseDisplayConfirmed
			if attempt.Phase == "verify" {
				phase = telegramSendPhaseVerifyConfirmed
			}
			_, _ = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
				Where("id", attempt.JobId).WhereIn("status", []string{"sending", "unknown"}).
				Data(g.Map{"status": "failed_retry", "send_phase": phase, "dispatch_status": tgDispatchStatusIdle,
					"next_retry_at": nil, "error_message": "", "updated_at": gtime.Now()}).Update()
			_ = s.enqueueTelegramJobDirectWithUnique(ctx, attempt.JobId, 0, false)
		}
		return nil
	}
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", attempt.JobId).Where("status", "sending").
		Data(g.Map{"status": "unknown", "dispatch_status": tgDispatchStatusProcessing, "next_retry_at": deadline, "error_message": "等待Telegram Webhook确认", "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "设置TG任务等待Webhook失败")
	}
	return s.enqueueTelegramAttemptTimeout(ctx, attempt.Id, telegramAttemptWebhookWait)
}

func (s *sSysPublish) telegramAttemptById(ctx context.Context, attemptId int64) (telegramDeliveryAttempt, error) {
	var attempt telegramDeliveryAttempt
	err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).Where("id", attemptId).Scan(&attempt)
	return attempt, err
}

func (s *sSysPublish) telegramJobHasWaitingAttempt(ctx context.Context, jobId int64) (bool, error) {
	count, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("job_id", jobId).Where("status", telegramAttemptStatusWaiting).Count()
	return count > 0, err
}

func (s *sSysPublish) handleTelegramAttemptTimeout(ctx context.Context, attemptId int64) error {
	attempt, err := s.telegramAttemptById(ctx, attemptId)
	if err != nil || attempt.Id <= 0 || attempt.Status != telegramAttemptStatusWaiting {
		return err
	}
	if attempt.WebhookDeadline != nil && attempt.WebhookDeadline.After(gtime.Now()) {
		return s.enqueueTelegramAttemptTimeout(ctx, attempt.Id, time.Until(attempt.WebhookDeadline.Time))
	}
	result, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("id", attempt.Id).Where("status", telegramAttemptStatusWaiting).
		Data(g.Map{"status": telegramAttemptStatusRetryWait, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新TG Attempt超时状态失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	if err = s.cleanupTelegramAttemptMessages(ctx, attempt); err != nil {
		g.Log().Warningf(ctx, "清理TG Attempt已知消息失败 attemptId:%d err:%+v", attempt.Id, err)
	}
	phase := telegramSendPhaseCleanupConfirmed
	if attempt.Phase == "verify" {
		phase = telegramSendPhaseDisplayConfirmed
	}
	job, err := s.telegramJobById(ctx, attempt.JobId)
	if err != nil {
		return err
	}
	withinRound := (attempt.AttemptNo-1)%telegramAttemptMaxRetries + 1
	status := "failed_retry"
	dispatchStatus := tgDispatchStatusIdle
	retryCount := job.RetryCount
	delay := time.Duration(0)
	message := fmt.Sprintf("Telegram发送结果未在%d秒内确认，准备本轮第%d次重试", int(telegramAttemptWebhookWait/time.Second), withinRound+1)
	if withinRound >= telegramAttemptMaxRetries {
		conf, confErr := s.telegramPublishConfig(ctx, job.TenantId, job.AccountId)
		if confErr != nil {
			conf = defaultPublishConfig()
		}
		decision := telegramJobFailureNextStateWithConfig(gerror.New("Telegram Webhook确认超时"), job.RetryCount, conf)
		status, dispatchStatus, retryCount, delay = decision.Status, decision.DispatchStatus, decision.RetryCount, decision.RetryDelay
		message = "Telegram发送结果连续无法确认，任务已移到队列尾部"
		if status == "failed" {
			message = "Telegram发送结果连续无法确认，已达到最大重试次数"
		}
	}
	nextRetryAt := interface{}(nil)
	if delay > 0 {
		nextRetryAt = gtime.Now().Add(delay)
	}
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", attempt.JobId).Where("status", "unknown").
		Data(g.Map{"status": status, "send_phase": phase, "dispatch_status": dispatchStatus,
			"retry_count": retryCount, "next_retry_at": nextRetryAt,
			"error_message": message, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "恢复TG Attempt超时任务失败")
	}
	if status == "failed" {
		observeTelegramPublishFailure(ctx, "webhook_timeout")
		return s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed)
	}
	if delay < telegramAttemptLateGrace {
		delay = telegramAttemptLateGrace
	}
	return s.enqueueTelegramJobDirectWithUnique(ctx, attempt.JobId, delay, false)
}

func (s *sSysPublish) cleanupTelegramAttemptMessages(ctx context.Context, attempt telegramDeliveryAttempt) error {
	var rows []*struct {
		MessageId int64 `json:"messageId"`
	}
	if err := g.DB().Model(publishTgAttemptMessageTable).Safe().Ctx(ctx).
		Fields("message_id").Where("attempt_id", attempt.Id).Scan(&rows); err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	token, err := s.telegramJobBotToken(ctx, attempt.BotId, attempt.TenantId)
	if err != nil {
		return err
	}
	bot, err := s.telegramBot(ctx, token)
	if err != nil {
		return err
	}
	messages := make([]*telegramSentMessage, 0, len(rows))
	for _, row := range rows {
		if row != nil && row.MessageId > 0 {
			messages = append(messages, &telegramSentMessage{MessageId: row.MessageId, Purpose: attempt.Phase})
		}
	}
	s.cleanupTelegramSentMessages(ctx, bot, attempt.TargetChatId, messages, "Attempt确认超时")
	return nil
}

func (s *sSysPublish) abandonTelegramDeliveryAttempt(ctx context.Context, attempt telegramDeliveryAttempt, messages []*telegramSentMessage, cause error) {
	_ = s.recordTelegramAttemptMessages(ctx, attempt, messages, "response")
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	_, _ = g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("id", attempt.Id).Where("status", telegramAttemptStatusSending).
		Data(g.Map{"status": telegramAttemptStatusRetryWait, "error_message": message, "updated_at": gtime.Now()}).Update()
}

func (s *sSysPublish) recordTelegramAttemptMessages(ctx context.Context, attempt telegramDeliveryAttempt, messages []*telegramSentMessage, source string) error {
	for _, message := range messages {
		if message == nil || message.MessageId <= 0 {
			continue
		}
		_, err := g.DB().Model(publishTgAttemptMessageTable).Safe().Ctx(ctx).Data(g.Map{
			"attempt_id": attempt.Id, "job_id": attempt.JobId, "target_chat_id": attempt.TargetChatId,
			"message_id": message.MessageId, "source": source, "created_at": gtime.Now(),
		}).OnConflict("attempt_id,message_id").OnDuplicate("source").Save()
		if err != nil {
			return gerror.Wrap(err, "记录TG Attempt消息失败")
		}
	}
	return nil
}

func telegramAttemptLogKey(attempt telegramDeliveryAttempt) string {
	return fmt.Sprintf("attempt:%d:%s:%d", attempt.JobId, attempt.Phase, attempt.AttemptNo)
}

func telegramAttemptMessageKey(attemptId int64, messageId int) string {
	return strconv.FormatInt(attemptId, 10) + ":" + strconv.Itoa(messageId)
}

func (s *sSysPublish) matchTelegramDeliveryAttempt(ctx context.Context, botId int64, msg *models.Message, text string) {
	if msg == nil || msg.ID <= 0 || msg.Chat.ID == 0 {
		return
	}
	chatId := normalizeTelegramChannelChatID(strconv.FormatInt(msg.Chat.ID, 10))
	var attempts []telegramDeliveryAttempt
	if err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("bot_id", botId).Where("target_chat_id", chatId).
		WhereIn("status", []string{telegramAttemptStatusSending, telegramAttemptStatusWaiting, telegramAttemptStatusRetryWait}).
		Where("status <> ? OR webhook_deadline IS NOT NULL", telegramAttemptStatusRetryWait).
		WhereGTE("created_at", gtime.Now().Add(-time.Minute)).
		OrderDesc("id").Limit(4).Scan(&attempts); err != nil {
		g.Log().Warningf(ctx, "匹配TG Webhook Attempt失败 bot:%d chat:%s message:%d err:%+v", botId, chatId, msg.ID, err)
		return
	}
	var matched *telegramDeliveryAttempt
	if strings.TrimSpace(msg.MediaGroupID) != "" {
		var grouped telegramDeliveryAttempt
		err := g.DB().Model(telegramBotMessageSourceTable+" source").Safe().Ctx(ctx).
			LeftJoin(publishTgAttemptTable+" attempt", "attempt.id=source.matched_attempt_id").
			Fields("attempt.*").
			Where("source.chat_id", chatId).Where("source.media_group_id", msg.MediaGroupID).
			WhereGT("source.matched_attempt_id", 0).
			WhereIn("attempt.status", []string{telegramAttemptStatusSending, telegramAttemptStatusWaiting, telegramAttemptStatusRetryWait}).
			Where("attempt.status <> ? OR attempt.webhook_deadline IS NOT NULL", telegramAttemptStatusRetryWait).
			OrderDesc("source.id").Limit(1).Scan(&grouped)
		if err == nil && grouped.Id > 0 {
			matched = &grouped
		}
	}
	for i := range attempts {
		if matched != nil {
			break
		}
		candidate := &attempts[i]
		if candidate.Phase == "display" && strings.Contains(text, telegramAttemptMarker(candidate.AttemptToken)) {
			matched = candidate
			break
		}
	}
	if matched == nil {
		matched = s.matchTelegramVerifyAttempt(ctx, attempts, msg)
	}
	if matched == nil {
		return
	}
	if matched.Status == telegramAttemptStatusRetryWait {
		if !s.confirmLateTelegramAttempt(ctx, *matched) {
			return
		}
	}
	result, err := g.DB().Model(telegramBotMessageSourceTable).Safe().Ctx(ctx).
		Where("chat_id", chatId).Where("message_id", msg.ID).
		Where("matched_attempt_id", 0).
		Data(g.Map{"matched_attempt_id": matched.Id, "process_status": "matched"}).Update()
	if err != nil {
		g.Log().Warningf(ctx, "绑定TG Webhook Attempt失败 attemptId:%d message:%d err:%+v", matched.Id, msg.ID, err)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return
	}
	job, err := s.telegramJobById(ctx, matched.JobId)
	if err != nil {
		return
	}
	sent := &telegramSentMessage{MessageId: int64(msg.ID), MediaGroupId: msg.MediaGroupID, Purpose: matched.Phase,
		TgFileId: telegramMessageFileId(msg), TgFileUniqueId: telegramMessageFileUniqueId(msg)}
	if err = s.recordTelegramAttemptMessages(ctx, *matched, []*telegramSentMessage{sent}, "webhook"); err != nil {
		g.Log().Warningf(ctx, "记录TG Webhook Attempt消息失败 attemptId:%d message:%d err:%+v", matched.Id, msg.ID, err)
		return
	}
	if err = s.saveTelegramSentMessages(ctx, job, []*telegramSentMessage{sent}); err != nil {
		g.Log().Warningf(ctx, "保存TG Webhook确认消息失败 attemptId:%d message:%d err:%+v", matched.Id, msg.ID, err)
		return
	}
	count, err := g.DB().Model(telegramBotMessageSourceTable).Safe().Ctx(ctx).
		Where("matched_attempt_id", matched.Id).Count()
	if err != nil {
		return
	}
	_, _ = g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).Where("id", matched.Id).
		WhereIn("status", []string{telegramAttemptStatusSending, telegramAttemptStatusWaiting}).
		Data(g.Map{"confirmed_count": count, "updated_at": gtime.Now()}).Update()
	if count < matched.ExpectedCount {
		return
	}
	s.confirmTelegramAttemptFromWebhook(ctx, *matched, job, count)
}

func (s *sSysPublish) matchTelegramVerifyAttempt(ctx context.Context, attempts []telegramDeliveryAttempt, msg *models.Message) *telegramDeliveryAttempt {
	fileUniqueId := telegramMessageFileUniqueId(msg)
	mediaType := telegramMessageMediaType(msg)
	if mediaType == "" {
		return nil
	}
	matched := make([]*telegramDeliveryAttempt, 0, 1)
	for index := range attempts {
		attempt := &attempts[index]
		if attempt.Phase != "verify" || attempt.Status == telegramAttemptStatusRetryWait && (attempt.WebhookDeadline == nil || gtime.Now().Sub(attempt.WebhookDeadline) > telegramAttemptLateGrace) {
			continue
		}
		job, err := s.telegramJobById(ctx, attempt.JobId)
		if err != nil {
			continue
		}
		media, err := s.telegramJobMedia(ctx, job, "verify")
		if err != nil {
			continue
		}
		s.loadTelegramMediaUniqueIds(ctx, media)
		if !telegramVerifyMediaMatches(media, mediaType, fileUniqueId) {
			continue
		}
		matched = append(matched, attempt)
	}
	if len(matched) != 1 {
		return nil
	}
	return matched[0]
}

func (s *sSysPublish) loadTelegramMediaUniqueIds(ctx context.Context, media []*telegramMediaItem) {
	ids := make([]int64, 0, len(media))
	byId := make(map[int64]*telegramMediaItem, len(media))
	for _, item := range media {
		if item != nil && item.Id > 0 {
			ids = append(ids, item.Id)
			byId[item.Id] = item
		}
	}
	if len(ids) == 0 {
		return
	}
	var rows []*struct {
		MediaId        int64  `json:"mediaId"`
		TgFileUniqueId string `json:"tgFileUniqueId"`
	}
	if err := g.DB().Model(publishTgMessageTable).Safe().Ctx(ctx).
		Fields("media_id,tg_file_unique_id").WhereIn("media_id", ids).
		Where("tg_file_unique_id <> ''").WhereNull("deleted_at").
		OrderDesc("id").Scan(&rows); err != nil {
		return
	}
	for _, row := range rows {
		if item := byId[row.MediaId]; item != nil && item.TgFileUniqueId == "" {
			item.TgFileUniqueId = strings.TrimSpace(row.TgFileUniqueId)
		}
	}
}

func telegramVerifyMediaMatches(media []*telegramMediaItem, messageType, fileUniqueId string) bool {
	for _, item := range media {
		if item == nil || normalizeTelegramMediaType(item.MediaType) != normalizeTelegramMediaType(messageType) {
			continue
		}
		// A known unique ID is authoritative. Fresh uploads do not have one yet,
		// so they additionally rely on the sole active Attempt and short window.
		if item.TgFileUniqueId == "" || fileUniqueId != "" && item.TgFileUniqueId == fileUniqueId {
			return true
		}
	}
	return false
}

func normalizeTelegramMediaType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "photo" {
		return "image"
	}
	return value
}

func (s *sSysPublish) confirmLateTelegramAttempt(ctx context.Context, attempt telegramDeliveryAttempt) bool {
	result, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("id", attempt.Id).Where("status", telegramAttemptStatusRetryWait).
		Data(g.Map{"status": telegramAttemptStatusWaiting, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return false
	}
	affected, _ := result.RowsAffected()
	return affected > 0
}

func (s *sSysPublish) confirmTelegramAttemptFromWebhook(ctx context.Context, attempt telegramDeliveryAttempt, job telegramJobRecord, count int) {
	result, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("id", attempt.Id).WhereIn("status", []string{telegramAttemptStatusSending, telegramAttemptStatusWaiting}).
		Data(g.Map{"status": telegramAttemptStatusConfirmed, "confirmed_count": count, "webhook_deadline": nil, "updated_at": gtime.Now()}).Update()
	if err != nil {
		g.Log().Warningf(ctx, "确认TG Webhook Attempt失败 attemptId:%d err:%+v", attempt.Id, err)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return
	}
	phase := telegramSendPhaseDisplayConfirmed
	if attempt.Phase == "verify" {
		phase = telegramSendPhaseVerifyConfirmed
	}
	_, err = g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", job.Id).WhereIn("status", []string{"unknown", "pending", "failed_retry"}).
		Data(g.Map{"status": "failed_retry", "send_phase": phase, "dispatch_status": tgDispatchStatusIdle,
			"next_retry_at": nil, "error_message": "", "updated_at": gtime.Now()}).Update()
	if err == nil {
		_ = s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false)
	}
}
