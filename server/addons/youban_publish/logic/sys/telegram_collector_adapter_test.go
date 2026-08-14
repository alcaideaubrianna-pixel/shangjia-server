package sys

import (
	"testing"
	"time"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_publish/model/input/sysin"
)

func TestPublishAccountTaskHandlersRegistered(t *testing.T) {
	for _, taskType := range []string{
		collectorin.AccountTaskTypeHistoryPage,
		collectorin.AccountTaskTypeMaterialImportHistoryPage,
		collectorin.AccountTaskTypeDialogCacheRefresh,
	} {
		if collectorservice.AccountTaskHandlerFor(taskType) == nil {
			t.Fatalf("Telegram account task handler is not registered: %s", taskType)
		}
	}
}

func TestCollectorDeliveryMessageCases(t *testing.T) {
	receivedAt := time.Date(2026, 8, 14, 2, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		sourceType string
		delivery   *collectorin.CollectorDelivery
	}{
		{
			name: "bot private group", sourceType: sysin.CollectSourceTypeBot,
			delivery: &collectorin.CollectorDelivery{
				SourceChatID: "88", SourceMessageID: 10, SourceGroupedID: "11",
				SourceUniqueKey: "bot:3:88:group:11", RawText: "私聊资料", ReceivedAt: receivedAt,
				Media: []collectorin.CollectorMediaItem{{Type: "photo", FileID: "photo"}},
			},
		},
		{
			name: "account history video", sourceType: sysin.CollectSourceTypeAccount,
			delivery: &collectorin.CollectorDelivery{
				SourceChatID: "-10088", SourceMessageID: 12, SourceGroupedID: "13",
				SourceUniqueKey: "account:4:source:3:-10088:message:12", RawText: "历史资料", ReceivedAt: receivedAt,
				Media: []collectorin.CollectorMediaItem{{
					Type: "video", FileID: "gotd:-10088:12", SourceKind: "document", SourceMediaID: 14,
					SourceAccessHash: 15, SourceFileReference: []byte{1, 2}, SourceMimeType: "video/mp4", SourceSize: 1024,
				}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := collectorDeliveryMessage(test.delivery, 1, 2, 3, test.sourceType)
			if message.TenantId != 1 || message.AccountId != 2 || message.SourceId != 3 || message.SourceType != test.sourceType {
				t.Fatalf("owner mismatch: %+v", message)
			}
			if message.SourceChatId != test.delivery.SourceChatID || message.SourceMessageId != test.delivery.SourceMessageID || message.SourceGroupedId != test.delivery.SourceGroupedID || message.RawText != test.delivery.RawText {
				t.Fatalf("identity mismatch: %+v", message)
			}
			if message.ReceivedAt == nil || message.ReceivedAt.Time.Equal(receivedAt) == false || len(message.Media) != 1 {
				t.Fatalf("delivery metadata mismatch: %+v", message)
			}
			if test.sourceType == sysin.CollectSourceTypeAccount {
				media := message.Media[0]
				if media.SourceMediaId != 14 || media.SourceAccessHash != 15 || media.SourceMimeType != "video/mp4" || media.SourceSize != 1024 {
					t.Fatalf("account media metadata lost: %+v", media)
				}
			}
		})
	}
}
