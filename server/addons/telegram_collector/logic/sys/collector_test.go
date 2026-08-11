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
	first := updateEventID("bot-key", 7, 42, []byte(`{"ok":true}`))
	second := updateEventID("bot-key", 7, 42, []byte(`{"ok":false}`))
	if first != second {
		t.Fatalf("expected update id to be the stable identity: %s != %s", first, second)
	}
}

func TestUpdateEventIDUsesPayloadHashWithoutUpdateID(t *testing.T) {
	first := updateEventID("bot-key", 7, 0, []byte(`{"ok":true}`))
	second := updateEventID("bot-key", 7, 0, []byte(`{"ok":false}`))
	if first == second {
		t.Fatal("expected payload hash to distinguish updates without an id")
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
