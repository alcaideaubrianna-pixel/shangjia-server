package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	_ "github.com/gogf/gf/contrib/drivers/pgsql/v2"
	"github.com/gogf/gf/v2/frame/g"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"

	"hotgo/addons/telegram_collector/internal/dao"
	"hotgo/addons/telegram_collector/internal/model/entity"
	"hotgo/addons/telegram_collector/model/input/sysin"
	collectorservice "hotgo/addons/telegram_collector/service"
)

type integrationDeliveryHandler struct{ calls int }

func (h *integrationDeliveryHandler) HandleCollectorDelivery(_ context.Context, delivery *sysin.CollectorDelivery) error {
	if delivery == nil || delivery.ID <= 0 {
		return fmt.Errorf("invalid delivery")
	}
	h.calls++
	return nil
}

func TestBuildMediaFingerprintStableAndTypeAware(t *testing.T) {
	image := BuildMediaFingerprint("ABC", 128, "photo", "image/jpeg")
	imageAgain := BuildMediaFingerprint("abc", 128, "PHOTO", "IMAGE/JPEG")
	video := BuildMediaFingerprint("abc", 128, "video", "video/mp4")

	if image != imageAgain {
		t.Fatalf("expected normalized fingerprints to match: %s != %s", image, imageAgain)
	}
	if image == video {
		t.Fatal("expected media kind to be part of fingerprint")
	}
}

func TestUpdateEventIDUsesTelegramUpdateID(t *testing.T) {
	first := updateEventID("bot-key", 7, 11, 42, []byte(`{"ok":true}`))
	second := updateEventID("bot-key", 7, 11, 42, []byte(`{"ok":false}`))
	if first != second {
		t.Fatalf("expected update id to be the stable identity: %s != %s", first, second)
	}
}

func TestUpdateEventIDUsesPayloadHashWithoutUpdateID(t *testing.T) {
	first := updateEventID("bot-key", 7, 11, 0, []byte(`{"ok":true}`))
	second := updateEventID("bot-key", 7, 11, 0, []byte(`{"ok":false}`))
	if first == second {
		t.Fatal("expected payload hash to distinguish updates without an id")
	}
}

func TestUpdateEventIDSeparatesCollectorSources(t *testing.T) {
	first := updateEventID("bot-key", 7, 11, 42, []byte(`{"ok":true}`))
	second := updateEventID("bot-key", 7, 12, 42, []byte(`{"ok":true}`))
	if first == second {
		t.Fatal("expected different collector sources to have different event ids")
	}
}

func TestCollectorDeliveryFromAccountEvent(t *testing.T) {
	receivedAt := time.Date(2026, 8, 11, 10, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	message := sysin.AccountMessageEvent{
		TenantID:        7,
		AccountID:       8,
		SourceID:        9,
		TgAccountID:     10,
		SourceChatID:    "-100123456",
		SourceMessageID: 11,
		SourceGroupedID: "12",
		SourceUniqueKey: "account:10:source:9:-100123456:group:12",
		RawText:         " 测试账号采集 ",
		ReceivedAt:      receivedAt,
		Media: []sysin.CollectorMediaItem{{
			Type:                "video",
			FileID:              "gotd:-100123456:11",
			SourceKind:          "document",
			SourceMediaID:       13,
			SourceAccessHash:    14,
			SourceFileReference: []byte{1, 2, 3},
			SourceMimeType:      "video/mp4",
			SourceSize:          1024,
		}},
	}
	raw, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("marshal account message: %v", err)
	}
	delivery, err := collectorDeliveryFromEvent(&entity.TgCollectorEvent{
		Id: 15, TenantId: message.TenantID, SourceId: message.SourceID,
		SourceType: sysin.SourceTypeAccount, AccountId: message.TgAccountID,
		ChatId: message.SourceChatID, MessageId: message.SourceMessageID,
	}, raw)
	if err != nil {
		t.Fatalf("build account delivery: %v", err)
	}
	if delivery.AccountID != message.AccountID || delivery.TgAccountID != message.TgAccountID {
		t.Fatalf("unexpected account identity: %+v", delivery)
	}
	if delivery.RawText != "测试账号采集" || delivery.SourceUniqueKey != message.SourceUniqueKey {
		t.Fatalf("unexpected delivery content: %+v", delivery)
	}
	if len(delivery.Media) != 1 || delivery.Media[0].SourceMediaID != 13 {
		t.Fatalf("unexpected delivery media: %+v", delivery.Media)
	}
	if !delivery.ReceivedAt.Equal(receivedAt) {
		t.Fatalf("receivedAt=%v want=%v", delivery.ReceivedAt, receivedAt)
	}
}

func TestCollectorDeliveryFromBotMediaGroup(t *testing.T) {
	receivedAt := time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	message := &models.Message{
		ID: 101, Chat: models.Chat{ID: 7074948877}, MediaGroupID: "9001",
		Caption: " 测试资料 ", Date: int(receivedAt.Unix()),
		Photo: []models.PhotoSize{{FileID: "small"}, {FileID: "large"}},
	}
	raw, err := json.Marshal(&models.Update{Message: message})
	if err != nil {
		t.Fatalf("marshal bot update: %v", err)
	}
	delivery, err := collectorDeliveryFromEvent(&entity.TgCollectorEvent{
		Id: 15, TenantId: 7, SourceId: 9, SourceType: sysin.SourceTypeBot,
		ChatId: "7074948877", MessageId: 101,
	}, raw)
	if err != nil {
		t.Fatalf("build bot delivery: %v", err)
	}
	if delivery.SourceGroupedID != "9001" || delivery.SourceUniqueKey != "bot:9:7074948877:group:9001" {
		t.Fatalf("unexpected media group identity: %+v", delivery)
	}
	if delivery.RawText != "测试资料" || len(delivery.Media) != 1 || delivery.Media[0].FileID != "large" {
		t.Fatalf("unexpected bot delivery content: %+v", delivery)
	}
	if !delivery.ReceivedAt.Equal(receivedAt) {
		t.Fatalf("receivedAt=%v want=%v", delivery.ReceivedAt, receivedAt)
	}
	secondKey := collectorBotMessageKey(9, "7074948877", 102, "9001")
	if secondKey != delivery.SourceUniqueKey {
		t.Fatalf("same media group must share event key: first=%q second=%q", delivery.SourceUniqueKey, secondKey)
	}
	if collectorBotMessageKey(9, "7074948877", 103, "9002") == delivery.SourceUniqueKey ||
		collectorBotMessageKey(9, "7074948877", 104, "") == delivery.SourceUniqueKey {
		t.Fatal("different groups and standalone messages must use independent event keys")
	}
}

func TestCollectorDeliveryFromBotCases(t *testing.T) {
	tests := []struct {
		name       string
		message    *models.Message
		wantChat   string
		wantKey    string
		wantText   string
		wantMedia  string
		wantFileID string
	}{
		{
			name: "private text", message: &models.Message{ID: 1, Chat: models.Chat{ID: 88, Type: models.ChatTypePrivate}, Text: " 私聊资料 "},
			wantChat: "88", wantKey: "bot:9:88:message:1", wantText: "私聊资料",
		},
		{
			name: "channel photo group", message: &models.Message{ID: 2, Chat: models.Chat{ID: -10088, Type: models.ChatTypeChannel}, MediaGroupID: "700", Caption: "频道资料", Photo: []models.PhotoSize{{FileID: "small"}, {FileID: "large"}}},
			wantChat: "-10088", wantKey: "bot:9:-10088:group:700", wantText: "频道资料", wantMedia: "photo", wantFileID: "large",
		},
		{
			name: "private video", message: &models.Message{ID: 3, Chat: models.Chat{ID: 99, Type: models.ChatTypePrivate}, Video: &models.Video{FileID: "video"}},
			wantChat: "99", wantKey: "bot:9:99:message:3", wantMedia: "video", wantFileID: "video",
		},
		{
			name: "private document", message: &models.Message{ID: 4, Chat: models.Chat{ID: 99, Type: models.ChatTypePrivate}, Document: &models.Document{FileID: "document"}},
			wantChat: "99", wantKey: "bot:9:99:message:4", wantMedia: "document", wantFileID: "document",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw, err := json.Marshal(&models.Update{Message: test.message})
			if err != nil {
				t.Fatal(err)
			}
			delivery, err := collectorDeliveryFromEvent(&entity.TgCollectorEvent{Id: 1, SourceId: 9, SourceType: sysin.SourceTypeBot}, raw)
			if err != nil {
				t.Fatal(err)
			}
			if delivery.SourceChatID != test.wantChat || delivery.SourceUniqueKey != test.wantKey || delivery.RawText != test.wantText {
				t.Fatalf("delivery identity/content mismatch: %+v", delivery)
			}
			if test.wantMedia == "" {
				if len(delivery.Media) != 0 {
					t.Fatalf("media=%+v, want none", delivery.Media)
				}
				return
			}
			if len(delivery.Media) != 1 || delivery.Media[0].Type != test.wantMedia || delivery.Media[0].FileID != test.wantFileID {
				t.Fatalf("media=%+v", delivery.Media)
			}
		})
	}
}

func TestMatchBotCollectionSourcesProductionCase(t *testing.T) {
	sources := []botCollectionSource{
		{TenantID: 10, SourceID: 130},
		{TenantID: 10, SourceID: 131},
		{TenantID: 11, SourceID: 140},
	}
	for _, messageType := range []string{"private", "group", "channel"} {
		matched := matchBotCollectionSources(sources, 10, 9)
		if len(matched) != 2 || matched[0].SourceID != 130 || matched[1].SourceID != 131 {
			t.Fatalf("%s update matched sources = %+v", messageType, matched)
		}
	}
}

func TestCollectorAccountDeliveryKeepsRealtimeAndHistoryIdentity(t *testing.T) {
	base := sysin.AccountMessageEvent{
		TenantID: 1, AccountID: 2, SourceID: 3, TgAccountID: 4,
		SourceChatID: "-1005", SourceMessageID: 6, SourceGroupedID: "7",
		SourceUniqueKey: "account:4:source:3:-1005:message:6", RawText: "资料",
		Media: []sysin.CollectorMediaItem{{Type: "photo", FileID: "gotd:-1005:6", SourceMediaID: 8}},
	}
	for _, name := range []string{"realtime", "history"} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(base)
			if err != nil {
				t.Fatal(err)
			}
			delivery, err := collectorDeliveryFromEvent(&entity.TgCollectorEvent{Id: 9, SourceType: sysin.SourceTypeAccount}, raw)
			if err != nil {
				t.Fatal(err)
			}
			if delivery.SourceUniqueKey != base.SourceUniqueKey || delivery.SourceGroupedID != base.SourceGroupedID || len(delivery.Media) != 1 || delivery.Media[0].SourceMediaID != 8 {
				t.Fatalf("delivery=%+v", delivery)
			}
		})
	}
}

func TestBotCollectorFeatureDoesNotConsumeUpdate(t *testing.T) {
	feature := &botCollectorFeature{}
	// The provider still handles non-collection features such as message cache
	// and automatic deletion after the collector records the update.
	handled, err := feature.HandleUpdate(t.Context(), gatewayservice.BotContext{}, &models.Update{ID: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if handled {
		t.Fatal("collector feature must not stop existing Gateway providers")
	}
}

func TestCollectorPersistenceIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_TELEGRAM_COLLECTOR_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_TELEGRAM_COLLECTOR_INTEGRATION=1 to run database integration test")
	}
	ctx := context.Background()
	collector := NewCollector()
	seed := time.Now().UnixNano()
	update := &models.Update{
		ID: seed,
		Message: &models.Message{
			ID:   int(seed % 1_000_000_000),
			Chat: models.Chat{ID: -seed},
			Text: "telegram collector integration",
		},
	}
	raw, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("marshal update: %v", err)
	}
	event := &sysin.RawUpdateEvent{
		EventID:    fmt.Sprintf("integration:%d", seed),
		TenantID:   seed,
		SourceID:   seed,
		SourceType: sysin.SourceTypeBot,
		ChatID:     update.Message.Chat.ID,
		MessageID:  int64(update.Message.ID),
		UpdateID:   update.ID,
		Priority:   sysin.EventPriorityUrgent,
		RawUpdate:  raw,
		ReceivedAt: time.Now(),
	}
	eventID, err := collector.persistEvent(ctx, event)
	if err != nil {
		t.Fatalf("persist event: %v", err)
	}
	eventColumns := dao.TgCollectorEvent.Columns()
	if _, err = dao.TgCollectorEvent.Ctx(ctx).WherePri(eventID).Data(g.Map{eventColumns.Status: sysin.EventStatusReady}).Update(); err != nil {
		t.Fatalf("mark event ready: %v", err)
	}
	duplicateID, err := collector.persistEvent(ctx, event)
	if err != nil {
		t.Fatalf("persist duplicate event: %v", err)
	}
	if duplicateID != eventID {
		t.Fatalf("duplicate event id=%d want=%d", duplicateID, eventID)
	}
	eventRow, err := dao.TgCollectorEvent.Ctx(ctx).Fields(eventColumns.Status).WherePri(eventID).One()
	if err != nil || eventRow[eventColumns.Status].String() != sysin.EventStatusReady {
		t.Fatalf("duplicate event reset status: status=%s err=%v", eventRow[eventColumns.Status].String(), err)
	}
	exists, err := collector.EventExists(ctx, event.TenantID, event.EventID)
	if err != nil || !exists {
		t.Fatalf("event exists=%v err=%v", exists, err)
	}
	delivery := &sysin.CollectorDelivery{
		DeliveryKey:     fmt.Sprintf("integration:%d", seed),
		TenantID:        seed,
		EventID:         eventID,
		SourceID:        seed,
		SourceType:      sysin.SourceTypeBot,
		SourceChatID:    fmt.Sprintf("%d", update.Message.Chat.ID),
		SourceMessageID: int64(update.Message.ID),
		RawText:         update.Message.Text,
		RawUpdate:       raw,
	}
	deliveryID, err := collector.saveDelivery(ctx, delivery)
	if err != nil {
		t.Fatalf("persist delivery: %v", err)
	}
	deliveryColumns := dao.TgCollectorDelivery.Columns()
	if _, err = dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).Data(g.Map{deliveryColumns.Status: sysin.DeliveryStatusCompleted}).Update(); err != nil {
		t.Fatalf("mark delivery completed: %v", err)
	}
	duplicateDeliveryID, err := collector.saveDelivery(ctx, delivery)
	if err != nil {
		t.Fatalf("persist duplicate delivery: %v", err)
	}
	if duplicateDeliveryID != deliveryID {
		t.Fatalf("duplicate delivery id=%d want=%d", duplicateDeliveryID, deliveryID)
	}
	deliveryRow, err := dao.TgCollectorDelivery.Ctx(ctx).Fields(deliveryColumns.Status).WherePri(deliveryID).One()
	if err != nil || deliveryRow[deliveryColumns.Status].String() != sysin.DeliveryStatusCompleted {
		t.Fatalf("duplicate delivery reset status: status=%s err=%v", deliveryRow[deliveryColumns.Status].String(), err)
	}
	if _, err = dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).Data(g.Map{
		deliveryColumns.Status:       sysin.DeliveryStatusPending,
		deliveryColumns.AttemptCount: 0,
	}).Update(); err != nil {
		t.Fatalf("reset delivery pending: %v", err)
	}
	previousHandler := collectorservice.CollectorDeliveryHandler()
	handler := &integrationDeliveryHandler{}
	collectorservice.RegisterDeliveryHandler(handler)
	t.Cleanup(func() { collectorservice.RegisterDeliveryHandler(previousHandler) })
	if err = collector.processDelivery(ctx, deliveryID); err != nil {
		t.Fatalf("process delivery: %v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("delivery handler calls=%d want=1", handler.calls)
	}
	t.Cleanup(func() {
		_, _ = dao.TgCollectorDelivery.Ctx(ctx).WherePri(deliveryID).Delete()
		_, _ = dao.TgCollectorEvent.Ctx(ctx).WherePri(eventID).Delete()
	})
}

func TestCollectorMediaCacheDatabaseFallbackIntegration(t *testing.T) {
	if os.Getenv("YOUBAN_TELEGRAM_COLLECTOR_INTEGRATION") != "1" {
		t.Skip("set YOUBAN_TELEGRAM_COLLECTOR_INTEGRATION=1 to run database integration test")
	}
	ctx := context.Background()
	collector := NewCollector()
	fingerprint := BuildMediaFingerprint(fmt.Sprintf("integration-%d", time.Now().UnixNano()), 128, "photo", "image/jpeg")
	missing, ready, err := collector.MediaCache(ctx, fingerprint)
	if err != nil || ready || missing != nil {
		t.Fatalf("missing media cache: cached=%+v ready=%v err=%v", missing, ready, err)
	}
	claimed, err := collector.ClaimMediaProcessing(ctx, fingerprint, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim media: claimed=%v err=%v", claimed, err)
	}
	claimedAgain, err := collector.ClaimMediaProcessing(ctx, fingerprint, time.Minute)
	if err != nil || claimedAgain {
		t.Fatalf("duplicate media claim: claimed=%v err=%v", claimedAgain, err)
	}
	entry := &sysin.MediaCacheEntry{
		Fingerprint:     fingerprint,
		StoragePath:     "attachment/integration/test.jpg",
		PHash:           "0123456789abcdef",
		DHash:           "fedcba9876543210",
		Kind:            sysin.MediaKindPhoto,
		MimeType:        "image/jpeg",
		Size:            128,
		PipelineVersion: mediaPipelineVersion,
	}
	if err = collector.SaveMediaReady(ctx, entry, time.Hour); err != nil {
		t.Fatalf("save media ready: %v", err)
	}
	claimedAfterReady, err := collector.ClaimMediaProcessing(ctx, fingerprint, time.Minute)
	if err != nil || claimedAfterReady {
		t.Fatalf("ready media reclaimed: claimed=%v err=%v", claimedAfterReady, err)
	}
	cached, ready, err := collector.MediaCache(ctx, fingerprint)
	if err != nil || !ready || cached == nil || cached.StoragePath != entry.StoragePath {
		t.Fatalf("read media fallback: cached=%+v ready=%v err=%v", cached, ready, err)
	}
	t.Cleanup(func() {
		columns := dao.TgCollectorMedia.Columns()
		_, _ = dao.TgCollectorMedia.Ctx(ctx).
			Where(columns.TenantId, 0).
			Where(columns.Fingerprint, fingerprint).
			Where(columns.PipelineVersion, mediaPipelineVersion).
			Delete()
		_, _ = g.DB().Exec(ctx, "SELECT 1")
	})
}
