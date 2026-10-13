package sys

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/guid"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/hgrds/lock"
)

const (
	publishTgAttemptTable           = "hg_youban_publish_tg_attempt"
	publishTgAttemptMessageTable    = "hg_youban_publish_tg_attempt_message"
	telegramAttemptStatusSending    = "sending"
	telegramAttemptStatusWaiting    = "awaiting_webhook"
	telegramAttemptStatusConfirmed  = "confirmed"
	telegramAttemptStatusRetryWait  = "retry_wait"
	telegramAttemptStatusSuperseded = "superseded"
	telegramAttemptWebhookWait      = 15 * time.Second
	telegramAttemptLateGrace        = 15 * time.Second
	telegramAttemptMaxRetries       = 3
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
	mutex := lock.NewConfig(30*time.Second, 100*time.Millisecond).Mutex(fmt.Sprintf("youban_publish:tg_attempt:%d:%s:%d", job.Id, phase, 0))
	if err := mutex.Lock(ctx); err != nil {
		return telegramDeliveryAttempt{}, gerror.Wrap(err, "获取TG发送Attempt锁失败")
	}
	defer func() { _ = mutex.Unlock(context.Background()) }()
	var attempt telegramDeliveryAttempt
	value, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Fields("COALESCE(MAX(attempt_no),0)").
		Where("job_id", job.Id).Where("phase", phase).Where("chunk_index", 0).Value()
	if err != nil {
		return attempt, gerror.Wrap(err, "读取TG发送尝试次数失败")
	}
	if err = s.supersedeExpiredTelegramAttempts(ctx, job.Id, phase); err != nil {
		return attempt, err
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
	// Keep a durable job-level audit entry for every Bot send attempt, including
	// attempts whose HTTP response later times out and has no Telegram message ID.
	s.appendTelegramJobLog(ctx, job, "publish", "attempt_started", fmt.Sprintf(
		"TG发送Attempt已创建 attemptId:%d phase:%s attemptNo:%d expectedCount:%d chat:%s",
		attempt.Id, phase, attempt.AttemptNo, expectedCount, attempt.TargetChatId))
	g.Log().Infof(ctx, "TG发送Attempt已创建 attemptId:%d jobId:%d profileId:%d channelId:%d botId:%d chat:%s phase:%s attemptNo:%d expectedCount:%d", attempt.Id, job.Id, job.ProfileId, job.ChannelId, job.BotId, attempt.TargetChatId, phase, attempt.AttemptNo, expectedCount)
	return attempt, nil
}

func (s *sSysPublish) supersedeExpiredTelegramAttempts(ctx context.Context, jobId int64, phase string) error {
	_, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("job_id", jobId).Where("phase", phase).
		Where("status", telegramAttemptStatusRetryWait).
		Where("webhook_deadline IS NULL OR webhook_deadline <= ?", gtime.Now().Add(-telegramAttemptLateGrace)).
		Data(g.Map{"status": telegramAttemptStatusSuperseded, "webhook_deadline": nil, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "关闭过期TG发送Attempt失败")
	}
	return nil
}

func (s *sSysPublish) supersedeOpenTelegramAttempts(ctx context.Context, jobId int64) error {
	_, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("job_id", jobId).
		WhereIn("status", []string{telegramAttemptStatusSending, telegramAttemptStatusWaiting, telegramAttemptStatusRetryWait}).
		Data(g.Map{"status": telegramAttemptStatusSuperseded, "webhook_deadline": nil, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "关闭TG任务遗留Attempt失败")
	}
	return nil
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
		Where("id", attempt.Id).
		WhereIn("status", []string{telegramAttemptStatusSending, telegramAttemptStatusWaiting, telegramAttemptStatusRetryWait, telegramAttemptStatusConfirmed}).
		Data(data).Update()
	return err
}

func (s *sSysPublish) waitTelegramDeliveryWebhook(ctx context.Context, attempt telegramDeliveryAttempt, cause error) error {
	deadline := gtime.Now().Add(telegramAttemptWebhookWait)
	confirmed := false
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var current telegramDeliveryAttempt
		if err := tx.Model(publishTgAttemptTable).Where("id", attempt.Id).LockUpdate().Scan(&current); err != nil {
			return err
		}
		confirmed = current.Status == telegramAttemptStatusConfirmed
		if confirmed {
			phase := telegramSendPhaseDisplayConfirmed
			if attempt.Phase == "verify" {
				phase = telegramSendPhaseVerifyConfirmed
			}
			_, err := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
				JobId:      attempt.JobId,
				From:       []string{telegramJobStatusSending},
				To:         telegramJobStatusRetryWait,
				SendPhases: []string{phase},
				Additional: g.Map{"error_message": ""},
			})
			return err
		}
		if current.Status != telegramAttemptStatusSending && current.Status != telegramAttemptStatusWaiting {
			return gerror.New("TG发送Attempt状态已变化，不能等待Webhook")
		}
		if _, err := tx.Model(publishTgAttemptTable).Where("id", attempt.Id).
			Data(g.Map{"status": telegramAttemptStatusWaiting, "webhook_deadline": deadline, "error_message": cause.Error(), "updated_at": gtime.Now()}).Update(); err != nil {
			return err
		}
		dispatchStatus := tgDispatchStatusProcessing
		if attempt.Phase == "verify" {
			dispatchStatus = tgDispatchStatusIdle
		}
		_, err := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
			JobId:        attempt.JobId,
			From:         []string{telegramJobStatusSending},
			To:           telegramJobStatusConfirming,
			ErrorMessage: "等待Telegram Webhook确认",
			Additional: g.Map{
				"dispatch_status": dispatchStatus,
				"next_retry_at":   deadline,
			},
		})
		return err
	})
	if err != nil {
		return gerror.Wrap(err, "设置TG任务等待Webhook失败")
	}
	if confirmed {
		return s.enqueueTelegramJobDirectWithUnique(ctx, attempt.JobId, 0, false)
	}
	_, err = s.enqueueTelegramAttemptTimeout(ctx, attempt.Id, telegramAttemptWebhookWait)
	g.Log().Warningf(ctx, "TG发送响应不确定，等待Webhook确认 attemptId:%d jobId:%d phase:%s cause:%v", attempt.Id, attempt.JobId, attempt.Phase, cause)
	return err
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
	result, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("id", attempt.Id).Where("status", telegramAttemptStatusWaiting).
		Where("webhook_deadline IS NOT NULL AND webhook_deadline <= NOW()").
		Data(g.Map{"status": telegramAttemptStatusRetryWait, "updated_at": gtime.Now()}).Update()
	if err != nil {
		return gerror.Wrap(err, "更新TG Attempt超时状态失败")
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil
	}
	job, err := s.telegramJobById(ctx, attempt.JobId)
	if err != nil {
		return err
	}
	if attempt.Phase == "verify" {
		if err = s.enqueueTelegramAttemptReconcile(ctx, job, attempt); err == nil {
			s.appendTelegramJobLog(ctx, job, "reconcile", "queued", "验证视频未收到Webhook确认，已提交协议号对账")
			return nil
		}
		g.Log().Warningf(ctx, "提交验证视频协议号对账失败 attemptId:%d jobId:%d err:%+v", attempt.Id, job.Id, err)
	}
	// Preserve partial confirmations so a late Webhook can still complete the attempt.
	phase := telegramSendPhaseCleanupConfirmed
	if attempt.Phase == "verify" {
		phase = telegramSendPhaseDisplayConfirmed
	}
	message := fmt.Sprintf("Telegram发送结果未在%d秒内确认，已停止自动重试，请人工核对频道消息", int(telegramAttemptWebhookWait/time.Second))
	g.Log().Warningf(ctx, "TG发送结果不确定，禁止自动重发 attemptId:%d jobId:%d profileId:%d channelId:%d botId:%d chat:%s phase:%s attemptNo:%d", attempt.Id, job.Id, job.ProfileId, job.ChannelId, job.BotId, attempt.TargetChatId, attempt.Phase, attempt.AttemptNo)
	transitioned, transitionErr := s.transitionTelegramJob(ctx, telegramJobTransition{
		JobId:        attempt.JobId,
		From:         []string{telegramJobStatusConfirming},
		To:           telegramJobStatusFailed,
		ErrorMessage: message,
		Additional:   g.Map{"send_phase": phase},
	})
	err = transitionErr
	if err != nil {
		return gerror.Wrap(err, "恢复TG Attempt超时任务失败")
	}
	if !transitioned {
		return nil
	}
	job.Status = "failed"
	job.ErrorMessage = message
	if recordErr := s.upsertPublishJobRecord(ctx, job, "failed", message); recordErr != nil {
		g.Log().Warningf(ctx, "同步TG Attempt超时发布记录失败 jobId:%d err:%+v", job.Id, recordErr)
	}
	s.appendTelegramJobLog(ctx, job, "reconcile", "failed", message)
	if job.CollectEventId > 0 {
		_ = s.markCollectDispatchFailedByProfile(ctx, job.ProfileId, job.CollectEventId, message)
	}
	if err = s.updateProfilePublishOperationState(ctx, job, sysin.PublishTaskStatusFailed); err != nil {
		return err
	}
	return s.wakeNextTelegramChannelJob(ctx, job)
}

func (s *sSysPublish) enqueueTelegramAttemptReconcile(ctx context.Context, job telegramJobRecord, attempt telegramDeliveryAttempt) error {
	channel, err := s.messagePushChannelFromJob(ctx, job)
	if err != nil {
		return err
	}
	if channel.TgAccountId <= 0 {
		return gerror.New("目标频道未绑定协议号，无法对账验证视频")
	}
	_, err = collectorservice.AccountTasks().Submit(ctx, &collectorin.AccountTaskSubmit{
		TenantID: job.TenantId, AccountID: channel.TgAccountId,
		TaskType: collectorin.AccountTaskTypeMessageReconcile,
		TaskKey:  fmt.Sprintf("message-reconcile:%d:%d", job.Id, attempt.Id),
		Priority: collectorin.EventPriorityUrgent, MaxAttempts: 3,
	})
	return gerror.Wrap(err, "提交验证视频协议号对账失败")
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
			g.Log().Errorf(ctx, "TG消息已返回但记录发送审计失败 attempt_id:%d job_id:%d phase:%s chat_id:%s message_id:%d source:%s err:%v",
				attempt.Id, attempt.JobId, attempt.Phase, attempt.TargetChatId, message.MessageId, source, err)
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
	window := time.Minute
	limit := 4
	if telegramTextHasInternalDeliveryMarker(text) {
		window = 24 * time.Hour
		limit = 1000
	}
	if err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).
		Where("bot_id", botId).Where("target_chat_id", chatId).
		WhereIn("status", []string{telegramAttemptStatusSending, telegramAttemptStatusWaiting, telegramAttemptStatusRetryWait}).
		Where("status <> ? OR webhook_deadline IS NOT NULL", telegramAttemptStatusRetryWait).
		WhereGTE("created_at", gtime.Now().Add(-window)).
		OrderDesc("id").Limit(limit).Scan(&attempts); err != nil {
		g.Log().Warningf(ctx, "匹配TG Webhook Attempt失败 bot:%d chat:%s message:%d err:%+v", botId, chatId, msg.ID, err)
		return
	}
	var matched *telegramDeliveryAttempt
	if strings.TrimSpace(msg.MediaGroupID) != "" {
		var grouped telegramDeliveryAttempt
		err := g.DB().Model(telegramBotMessageSourceTable+" source").Safe().Ctx(ctx).
			LeftJoin(publishTgAttemptTable+" attempt", "attempt.id=source.matched_attempt_id").
			Fields("attempt.*").
			Where("attempt.bot_id", botId).Where("source.chat_id", chatId).Where("source.media_group_id", msg.MediaGroupID).
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
		if strings.Contains(text, telegramAttemptMarker(candidate.AttemptToken)) {
			matched = candidate
			break
		}
	}
	if matched == nil && !telegramTextHasInternalDeliveryMarker(text) {
		matched = s.matchTelegramVerifyAttempt(ctx, attempts, msg)
	}
	if matched == nil {
		return
	}
	if matched.Phase == "verify" && telegramMessageFileId(msg) == "" {
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
		// A retry_wait attempt represents an ambiguous send. Keep it eligible
		// for late webhook reconciliation; it must never be retried merely
		// because the original HTTP request timed out.
		if attempt.Phase != "verify" || attempt.Status == telegramAttemptStatusRetryWait && attempt.WebhookDeadline == nil {
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
		if fileUniqueId != "" && item.TgFileUniqueId != "" && item.TgFileUniqueId == fileUniqueId {
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
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var current telegramDeliveryAttempt
		if err := tx.Model(publishTgAttemptTable).Where("id", attempt.Id).LockUpdate().Scan(&current); err != nil {
			return err
		}
		if current.Status != telegramAttemptStatusSending && current.Status != telegramAttemptStatusWaiting && current.Status != telegramAttemptStatusConfirmed {
			return nil
		}
		phase := telegramSendPhaseDisplayConfirmed
		sendingPhase := telegramSendPhaseDisplaySending
		if attempt.Phase == "verify" {
			phase = telegramSendPhaseVerifyConfirmed
			sendingPhase = telegramSendPhaseVerifySending
		}
		// Preserve the active sender: its successful response must not delete webhook-confirmed media.
		if _, err := tx.Model(publishTgJobTable).Where("id", job.Id).Where("status", "sending").Where("send_phase", sendingPhase).
			Data(g.Map{"send_phase": phase}).Update(); err != nil {
			return err
		}
		if _, err := s.transitionTelegramJobTx(ctx, tx, telegramJobTransition{
			JobId: job.Id,
			From: []string{
				telegramJobStatusConfirming, telegramJobStatusPending,
				telegramJobStatusRetryWait, telegramJobStatusFailed,
			},
			To:         telegramJobStatusRetryWait,
			SendPhases: []string{sendingPhase, phase, telegramSendPhaseDisplayConfirmed},
			Additional: g.Map{"send_phase": phase, "error_message": ""},
		}); err != nil {
			return err
		}
		_, err := tx.Model(publishTgAttemptTable).Where("id", attempt.Id).
			Data(g.Map{"status": telegramAttemptStatusConfirmed, "confirmed_count": count, "webhook_deadline": nil, "updated_at": gtime.Now()}).Update()
		return err
	})
	if err != nil {
		g.Log().Warningf(ctx, "确认TG Webhook失败 attemptId:%d jobId:%d err:%+v", attempt.Id, job.Id, err)
		return
	}
	if current, err := s.telegramJobById(ctx, job.Id); err == nil && current.Status == "failed_retry" {
		_ = s.enqueueTelegramJobDirectWithUnique(ctx, job.Id, 0, false)
	}
}

// Strip only complete internal attempt markers, preserving user text and malformed input.
func stripTelegramAttemptMarkers(text string) string {
	for {
		start := strings.Index(text, "\u2063")
		if start < 0 {
			return text
		}
		end := strings.Index(text[start:], "\u2064")
		if end < 0 {
			return text
		}
		candidate := text[start : start+end+len("\u2064")]
		bits := []rune(strings.TrimSuffix(strings.TrimPrefix(candidate, "\u2063"), "\u2064"))
		valid := len(bits) == 64
		for _, bit := range bits {
			valid = valid && (bit == '\u200b' || bit == '\u200c')
		}
		if !valid {
			return text
		}
		text = text[:start] + text[start+end+len("\u2064"):]
	}
}
