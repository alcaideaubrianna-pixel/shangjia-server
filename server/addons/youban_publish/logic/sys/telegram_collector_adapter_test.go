package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/os/gcache"
	"github.com/gogf/gf/v2/os/gtime"

	collectorin "hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

func TestPublishAccountTaskHandlersRegistered(t *testing.T) {
	for _, taskType := range []string{
		collectorin.AccountTaskTypeHistoryPage,
		collectorin.AccountTaskTypeMaterialImportHistoryPage,
		collectorin.AccountTaskTypeDialogCacheRefresh,
		collectorin.AccountTaskTypeMessagePushInline,
		collectorin.AccountTaskTypeMessageDeleteFallback,
		collectorin.AccountTaskTypeManagedBotUsernameCheck,
	} {
		if collectorservice.AccountTaskHandlerFor(taskType) == nil {
			t.Fatalf("Telegram account task handler is not registered: %s", taskType)
		}
	}
	for _, taskType := range []string{collectorin.AccountTaskTypeMessageMediaFallback} {
		if collectorservice.AccountTaskFailureHandlerFor(taskType) == nil {
			t.Fatalf("Telegram account task failure handler is not registered: %s", taskType)
		}
	}
	policies := collectorservice.AccountTaskPolicies()
	if got := policies[collectorin.AccountTaskTypeMessageMediaFallback].PendingTTL; got != 45*time.Minute {
		t.Fatalf("message media fallback pending TTL = %s, want 45m", got)
	}
}

func TestMessageReconcileTaskIsDisabled(t *testing.T) {
	if collectorservice.AccountTaskHandlerFor(collectorin.AccountTaskTypeMessageReconcile) != nil {
		t.Fatal("message reconcile account task must not be registered")
	}
	if collectorservice.AccountTaskFailureHandlerFor(collectorin.AccountTaskTypeMessageReconcile) != nil {
		t.Fatal("message reconcile failure handler must not be registered")
	}
	if _, ok := collectorservice.AccountTaskPolicies()[collectorin.AccountTaskTypeMessageReconcile]; ok {
		t.Fatal("message reconcile policy must not be registered")
	}
}

func TestAccountTaskFinalAttempt(t *testing.T) {
	tests := []struct {
		name string
		task *collectorin.AccountTask
		want bool
	}{
		{name: "nil task", want: false},
		{name: "no max attempts", task: &collectorin.AccountTask{AttemptCount: 1}, want: false},
		{name: "retry remains", task: &collectorin.AccountTask{AttemptCount: 2, MaxAttempts: 3}, want: false},
		{name: "final attempt", task: &collectorin.AccountTask{AttemptCount: 3, MaxAttempts: 3}, want: true},
		{name: "past final attempt", task: &collectorin.AccountTask{AttemptCount: 4, MaxAttempts: 3}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := accountTaskFinalAttempt(test.task); got != test.want {
				t.Fatalf("accountTaskFinalAttempt() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestParseMessagePushAccountTaskKey(t *testing.T) {
	tests := []struct {
		key     string
		jobID   int64
		mode    string
		wantErr bool
	}{
		{key: "message-push-inline:123", jobID: 123, mode: "inline"},
		{key: "message-push-inline:456:account", jobID: 456, mode: "account"},
		{key: "message-push-inline:0", wantErr: true},
		{key: "message-push-inline:1:unknown", wantErr: true},
		{key: "other:1", wantErr: true},
	}
	for _, test := range tests {
		jobID, mode, err := parseMessagePushAccountTaskKey(test.key)
		if test.wantErr {
			if err == nil {
				t.Fatalf("parseMessagePushAccountTaskKey(%q) expected error", test.key)
			}
			continue
		}
		if err != nil || jobID != test.jobID || mode != test.mode {
			t.Fatalf("parseMessagePushAccountTaskKey(%q) = (%d, %q, %v)", test.key, jobID, mode, err)
		}
	}
}

func TestParseMessageMediaFallbackTaskKey(t *testing.T) {
	for _, test := range []struct {
		key     string
		jobID   int64
		purpose string
		wantErr bool
	}{
		{key: "message-media-fallback:12:verify", jobID: 12, purpose: "verify"},
		{key: "message-media-fallback:13:display", jobID: 13, purpose: "display"},
		{key: "message-media-fallback:0:verify", wantErr: true},
		{key: "message-media-fallback:12:other", wantErr: true},
		{key: "message-media-fallback:12", wantErr: true},
	} {
		jobID, purpose, err := parseMessageMediaFallbackTaskKey(test.key)
		if test.wantErr {
			if err == nil {
				t.Fatalf("parseMessageMediaFallbackTaskKey(%q) expected error", test.key)
			}
			continue
		}
		if err != nil || jobID != test.jobID || purpose != test.purpose {
			t.Fatalf("parseMessageMediaFallbackTaskKey(%q) = (%d, %q, %v)", test.key, jobID, purpose, err)
		}
	}
}

func TestAccountFallbackPhaseConfirmedPreventsDuplicateSend(t *testing.T) {
	tests := []struct {
		phase   string
		purpose string
		want    bool
	}{
		{phase: telegramSendPhaseDisplayConfirmed, purpose: "display", want: true},
		{phase: telegramSendPhaseVerifySending, purpose: "display", want: true},
		{phase: telegramSendPhaseVerifyConfirmed, purpose: "display", want: true},
		{phase: telegramSendPhaseVerifyConfirmed, purpose: "verify", want: true},
		{phase: telegramSendPhaseDisplayConfirmed, purpose: "verify", want: false},
		{phase: telegramSendPhaseDisplaySending, purpose: "display", want: false},
	}
	for _, test := range tests {
		if got := accountFallbackPhaseConfirmed(test.phase, test.purpose); got != test.want {
			t.Fatalf("accountFallbackPhaseConfirmed(%q, %q) = %v, want %v", test.phase, test.purpose, got, test.want)
		}
	}
}

func TestAccountFallbackFailureIsObsoleteAfterDisplayConfirmation(t *testing.T) {
	tests := []struct {
		purpose string
		phase   string
		want    bool
	}{
		{purpose: "display", phase: telegramSendPhaseDisplayConfirmed, want: true},
		{purpose: "display", phase: telegramSendPhaseVerifySending, want: true},
		{purpose: "display", phase: telegramSendPhaseVerifyConfirmed, want: true},
		{purpose: "display", phase: telegramSendPhaseDisplaySending, want: false},
		{purpose: "verify", phase: telegramSendPhaseVerifySending, want: false},
	}
	for _, test := range tests {
		if got := accountFallbackFailureIsObsolete(test.purpose, test.phase); got != test.want {
			t.Fatalf("accountFallbackFailureIsObsolete(%q, %q) = %v, want %v", test.purpose, test.phase, got, test.want)
		}
	}
}

func TestTelegramMediaPurposeLabel(t *testing.T) {
	if got := telegramMediaPurposeLabel("display"); got != "展示资料" {
		t.Fatalf("display label = %q", got)
	}
	if got := telegramMediaPurposeLabel("verify"); got != "验证视频" {
		t.Fatalf("verify label = %q", got)
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

// TestPublishChannelMessagesAreAlwaysBlocked verifies that target channels are
// recognized by normalized Telegram chat IDs regardless of message audit state.
func TestPublishChannelMessagesAreAlwaysBlocked(t *testing.T) {
	if !collectChatMatchesPublishChannel("-1003763752459", []string{"3763752459"}) {
		t.Fatal("normalized publish target chat ID must be blocked")
	}
	if collectChatMatchesPublishChannel("-1003998084277", []string{"-1003763752459"}) {
		t.Fatal("unconfigured chat must remain collectable")
	}
}

// TestPublishChannelMessageAuditIsNotRequired documents the regression boundary:
// the caller must use the channel-level decision even when tg_message is absent.
func TestPublishChannelMessageAuditIsNotRequired(t *testing.T) {
	publishTargets := []string{"-1003763752459"}
	for _, incoming := range []string{"-1003763752459", "3763752459"} {
		if !collectChatMatchesPublishChannel(incoming, publishTargets) {
			t.Fatalf("publish target %s was not blocked without audit lookup", incoming)
		}
	}
}

func TestTelegramWebhookReconcileIndexUsesMessageAndGroupKeys(t *testing.T) {
	if !cache.Initialized() {
		cache.SetAdapter(context.Background())
	}
	previous := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	t.Cleanup(func() { cache.Instance().SetAdapter(previous) })

	message := telegramWebhookReconcileMessage{
		ChatID: "-100123", MessageID: 88, MediaGroupID: "album-7", ReceivedAt: time.Unix(100, 0),
	}
	cacheTelegramWebhookReconcileMessage(context.Background(), message)

	value, err := cache.Instance().Get(context.Background(), telegramWebhookReconcileMessageKey(message.ChatID, message.MessageID))
	if err != nil || value.IsNil() {
		t.Fatalf("message reconcile index missing: value=%v err=%v", value, err)
	}
	var got telegramWebhookMessageState
	if err := json.Unmarshal([]byte(value.String()), &got); err != nil {
		t.Fatalf("decode compact message reconcile state: %v", err)
	}
	if got.MediaGroupID != message.MediaGroupID || got.Status != telegramWebhookMessageStatusReceived {
		t.Fatalf("message reconcile state = %+v, want group=%q status=%q", got, message.MediaGroupID, telegramWebhookMessageStatusReceived)
	}
	if strings.Contains(value.String(), "chatId") || strings.Contains(value.String(), "messageId") || strings.Contains(value.String(), "text") {
		t.Fatalf("message reconcile state contains forbidden fields: %s", value.String())
	}
	group, err := cache.Instance().Get(context.Background(), telegramWebhookReconcileGroupKey(message.ChatID, message.MediaGroupID))
	if err != nil || group.IsNil() {
		t.Fatalf("group reconcile index missing: value=%v err=%v", group, err)
	}
}

func TestTelegramWebhookRecentIndexDefaultsTo500(t *testing.T) {
	if telegramWebhookRecentMessagesCapacity != 500 {
		t.Fatalf("recent message capacity=%d, want 500", telegramWebhookRecentMessagesCapacity)
	}
	if got := telegramWebhookRecentCapacity(context.Background()); got != 500 {
		t.Fatalf("configured default recent capacity=%d, want 500", got)
	}
}

func TestTelegramWebhookAttemptIndexRoundTripAndRedisFallback(t *testing.T) {
	if !cache.Initialized() {
		cache.SetAdapter(context.Background())
	}
	previous := cache.Instance().GetAdapter()
	cache.Instance().SetAdapter(gcache.NewAdapterMemory())
	t.Cleanup(func() { cache.Instance().SetAdapter(previous) })

	attempt := telegramDeliveryAttempt{Id: 901, JobId: 902, Phase: "display", TargetChatId: "-100123", CreatedAt: gtime.Now()}
	indexTelegramDeliveryAttempt(context.Background(), attempt)
	ids := telegramWebhookAttemptIDs(context.Background(), attempt.TargetChatId)
	if len(ids) != 1 || ids[0] != attempt.Id {
		t.Fatalf("attempt index ids=%v, want [%d]", ids, attempt.Id)
	}

	_, err := cache.Instance().Remove(context.Background(), telegramWebhookAttemptIndexKey(attempt.TargetChatId))
	if err != nil {
		t.Fatalf("remove attempt index: %v", err)
	}
	if got := telegramWebhookAttemptIDs(context.Background(), attempt.TargetChatId); len(got) != 0 {
		t.Fatalf("redis fallback should return no candidates, got %v", got)
	}
}

func TestMergeTelegramWebhookAttemptsKeepsPostgresAttemptWhenRedisCandidateIsDirty(t *testing.T) {
	redisAttempt := telegramDeliveryAttempt{Id: 10, JobId: 100, TargetChatId: "-1001"}
	postgresAttempt := telegramDeliveryAttempt{Id: 20, JobId: 200, TargetChatId: "-1001"}
	merged := mergeTelegramWebhookAttempts([]telegramDeliveryAttempt{redisAttempt, redisAttempt}, []telegramDeliveryAttempt{postgresAttempt, redisAttempt})
	if len(merged) != 2 || merged[0].Id != redisAttempt.Id || merged[1].Id != postgresAttempt.Id {
		t.Fatalf("merged attempts=%+v, want redis candidate plus postgres fallback without duplicates", merged)
	}
}

func TestTelegramWebhookMessageTypeAndStoredFacts(t *testing.T) {
	cases := []struct {
		name string
		msg  *models.Message
		want string
	}{
		{name: "album", msg: &models.Message{MediaGroupID: "g", Photo: []models.PhotoSize{{}}}, want: "album"},
		{name: "photo", msg: &models.Message{Photo: []models.PhotoSize{{}}}, want: "photo"},
		{name: "video", msg: &models.Message{Video: &models.Video{}}, want: "video"},
		{name: "document", msg: &models.Message{Document: &models.Document{}}, want: "document"},
		{name: "text", msg: &models.Message{Text: "hello"}, want: "text"},
		{name: "unknown", msg: &models.Message{ID: 1}, want: "unknown"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := telegramWebhookMessageType(test.msg); got != test.want {
				t.Fatalf("message type=%q, want %q", got, test.want)
			}
		})
	}
	message := telegramWebhookReconcileMessage{ChatID: "-1001", MessageID: 7, MessageType: "video", Status: telegramWebhookMessageStatusMatched, AttemptID: 9, JobID: 10, Phase: "verify", ReceivedAt: time.Now()}
	if strings.Contains(fmt.Sprintf("%+v", message), "text") {
		t.Fatal("webhook reconcile fact must not contain text")
	}
}
