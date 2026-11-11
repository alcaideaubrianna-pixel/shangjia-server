package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

// deferStreamWebhookAfterUnavailableReconcile 对账服务不可用不代表TG发送失败；保留已有迟到宽限，禁止直接重发。
func (s *sSysPublish) deferStreamWebhookAfterUnavailableReconcile(ctx context.Context, job telegramJobRecord, attempt telegramDeliveryAttempt, cause error) (bool, error) {
	if s.accountPushEngine(ctx, job.TenantId, job.AccountId) != pushEngineUnifiedStream || attempt.CreatedAt == nil {
		return false, nil
	}
	// 使用数据库本地时间解析，避免timestamp被附加UTC后偏移八小时。
	created := gtime.NewFromStr(attempt.CreatedAt.Format("Y-m-d H:i:s"))
	if created == nil {
		return false, nil
	}
	deadline := created.Add(telegramAttemptWebhookWait + telegramAttemptLateGrace)
	if !deadline.After(gtime.Now()) {
		return false, nil
	}
	result, err := g.DB().Model(publishTgAttemptTable).Safe().Ctx(ctx).Where("id", attempt.Id).Where("status", telegramAttemptStatusRetryWait).
		Data(g.Map{"status": telegramAttemptStatusWaiting, "webhook_deadline": deadline, "error_message": "协议号对账暂不可用，继续等待迟到Webhook：" + cause.Error(), "updated_at": gtime.Now()}).Update()
	if err != nil {
		return true, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 0 {
		return true, err
	}
	_, err = s.enqueueTelegramAttemptTimeout(ctx, attempt.Id, time.Until(deadline.Time))
	return true, err
}

// matchStreamWebhookGroup 原子绑定相册已有成员并保存完整证据；无说明文字的成员先到也不会丢失。
func (s *sSysPublish) matchStreamWebhookGroup(ctx context.Context, attempt telegramDeliveryAttempt, job telegramJobRecord, msg *models.Message) error {
	var currentJob telegramJobRecord
	var currentAttempt telegramDeliveryAttempt
	count := 0
	cancelled := false
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// 与API成功提交、取消操作统一锁顺序，避免Job/Attempt互相等待。
		if err := tx.Model(publishTgJobTable).Where("id", job.Id).LockUpdate().Scan(&currentJob); err != nil {
			return err
		}
		if err := tx.Model(publishTgAttemptTable).Where("id", attempt.Id).LockUpdate().Scan(&currentAttempt); err != nil {
			return err
		}
		if currentJob.Id <= 0 || currentAttempt.Id <= 0 {
			return nil
		}
		chat := normalizeTelegramChannelChatID(fmt.Sprint(msg.Chat.ID))
		if currentJob.Status == telegramJobStatusSuperseded || currentJob.SendPhase == telegramSendPhaseTimeoutCancelled || currentAttempt.Status == telegramAttemptStatusSuperseded {
			cancelled = true
			return nil
		}
		if currentJob.Status == telegramJobStatusSent || currentAttempt.Status == telegramAttemptStatusConfirmed {
			return nil
		}
		if _, err := tx.Model(telegramBotMessageSourceTable).Where("chat_id", chat).Where("message_id", msg.ID).
			Where("matched_attempt_id=0 OR matched_attempt_id=?", attempt.Id).
			Data(g.Map{"matched_attempt_id": attempt.Id, "process_status": "matched"}).Update(); err != nil {
			return err
		}
		group := strings.TrimSpace(msg.MediaGroupID)
		if group != "" {
			if _, err := tx.Model(telegramBotMessageSourceTable).Where("chat_id", chat).Where("media_group_id", group).
				Where("matched_attempt_id", 0).WhereNull("deleted_at").
				Data(g.Map{"matched_attempt_id": attempt.Id, "process_status": "matched"}).Update(); err != nil {
				return err
			}
		}
		query := tx.Model(telegramBotMessageSourceTable).Where("chat_id", chat).Where("matched_attempt_id", attempt.Id).WhereNull("deleted_at")
		if group != "" {
			query = query.Where("media_group_id", group)
		} else {
			query = query.Where("message_id", msg.ID)
		}
		rows, err := query.Fields("message_id,media_group_id,tg_file_id,tg_file_unique_id").OrderAsc("message_id").All()
		if err != nil {
			return err
		}
		messages := make([]*telegramSentMessage, 0, len(rows))
		for _, row := range rows {
			if attempt.Phase == "verify" && row["tg_file_id"].String() == "" {
				continue
			}
			messages = append(messages, &telegramSentMessage{MessageId: row["message_id"].Int64(), MediaGroupId: row["media_group_id"].String(), Purpose: attempt.Phase, TgFileId: row["tg_file_id"].String(), TgFileUniqueId: row["tg_file_unique_id"].String()})
		}
		if len(messages) == 0 {
			return nil
		}
		if err := recordTelegramAttemptMessagesTx(ctx, tx, currentAttempt, messages, "webhook"); err != nil {
			return err
		}
		if err := saveTelegramSentMessagesTx(ctx, tx, currentJob, messages); err != nil {
			return err
		}
		value, err := tx.Model(publishTgAttemptMessageTable).Where("attempt_id", attempt.Id).Fields("COUNT(DISTINCT message_id)").Value()
		if err != nil {
			return err
		}
		count = value.Int()
		_, err = tx.Model(publishTgAttemptTable).Where("id", attempt.Id).Data(g.Map{"status": telegramAttemptStatusWaiting, "confirmed_count": count, "updated_at": gtime.Now()}).Update()
		return err
	})
	if err != nil {
		return err
	}
	if cancelled {
		return s.enqueueTelegramDuplicateMessages(ctx, job, []*telegramSentMessage{{MessageId: int64(msg.ID), Purpose: attempt.Phase}})
	}
	if count >= currentAttempt.ExpectedCount && currentAttempt.Id > 0 {
		s.confirmTelegramAttemptFromWebhook(ctx, currentAttempt, currentJob, count)
	}
	return nil
}
