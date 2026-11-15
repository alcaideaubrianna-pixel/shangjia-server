package sys

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hotgo/internal/library/cache"
)

const telegramReturnedMessageProjectionTTL = 7 * 24 * time.Hour

type telegramReturnedMessageProjection struct {
	TenantID   int64  `json:"tenant_id"`
	TargetChat string `json:"target_chat"`
	MessageID  int64  `json:"message_id"`
	JobID      int64  `json:"job_id"`
	Source     string `json:"source"`
	SendMode   string `json:"send_mode"`
	CreatedAt  string `json:"created_at"`
}

func telegramReturnedMessageProjectionKey(tenantID int64, targetChatID string, messageID int64) string {
	return fmt.Sprintf("youban_publish:tg_message_projection:%d:%s:%d", tenantID, normalizeTelegramChannelChatID(targetChatID), messageID)
}

// projectTelegramReturnedMessages writes one idempotent cache record per
// Telegram message. It is intentionally independent from the PostgreSQL
// confirmation transaction so a partial album response remains available for
// reconciliation even when the local transaction later fails.
func projectTelegramReturnedMessages(ctx context.Context, job telegramJobRecord, messages []*telegramSentMessage, source string) {
	if !cache.Initialized() || job.TenantId <= 0 || strings.TrimSpace(job.TargetChatId) == "" {
		return
	}
	if strings.TrimSpace(source) == "" {
		source = "telegram_api"
	}
	for _, message := range messages {
		if message == nil || message.MessageId <= 0 {
			continue
		}
		projection := telegramReturnedMessageProjection{
			TenantID: job.TenantId, TargetChat: job.TargetChatId, MessageID: message.MessageId,
			JobID: job.Id, Source: source, SendMode: job.PushMode,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		if err := cache.Instance().Set(ctx, telegramReturnedMessageProjectionKey(job.TenantId, job.TargetChatId, message.MessageId), projection, telegramReturnedMessageProjectionTTL); err != nil {
			// The durable SQL evidence remains authoritative; cache failure must
			// never turn a confirmed Telegram response into a retry.
			continue
		}
	}
}
