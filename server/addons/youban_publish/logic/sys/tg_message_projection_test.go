package sys

import (
	"context"
	"testing"

	"github.com/gogf/gf/v2/os/gcache"
	"hotgo/internal/library/cache"
)

func TestProjectTelegramReturnedMessagesIsPerMessageAndIdempotent(t *testing.T) {
	if !cache.Initialized() {
		cache.SetAdapter(context.Background())
	}
	previous := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	t.Cleanup(func() { cache.Instance().SetAdapter(previous) })

	job := telegramJobRecord{Id: 42, TenantId: 7, TargetChatId: "-100123", PushMode: "bot"}
	projectTelegramReturnedMessages(context.Background(), job, []*telegramSentMessage{
		{MessageId: 101, MediaGroupId: "album"},
		{MessageId: 102, MediaGroupId: "album"},
	}, "telegram_api")
	projectTelegramReturnedMessages(context.Background(), job, []*telegramSentMessage{{MessageId: 101}}, "telegram_api")

	for _, messageID := range []int64{101, 102} {
		value, err := cache.Instance().Get(context.Background(), telegramReturnedMessageProjectionKey(job.TenantId, job.TargetChatId, messageID))
		if err != nil || value.IsNil() {
			t.Fatalf("projection message_id=%d missing: %v", messageID, err)
		}
		projection, ok := value.Val().(telegramReturnedMessageProjection)
		if !ok {
			t.Fatalf("projection message_id=%d type=%T", messageID, value.Val())
		}
		if projection.JobID != job.Id || projection.TenantID != job.TenantId || projection.MessageID != messageID {
			t.Fatalf("unexpected projection: %+v", projection)
		}
	}
}
