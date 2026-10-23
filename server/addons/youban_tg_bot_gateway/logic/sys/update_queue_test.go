package sys

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"hotgo/utility/scanconfig"
)

func TestGatewayUpdatePayloadRoundTrip(t *testing.T) {
	update := &models.Update{ID: 12345}
	body, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("marshal update: %v", err)
	}
	payload, err := json.Marshal(gatewayUpdatePayload{Key: "bot-key", Body: body})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	var decoded gatewayUpdatePayload
	if err = json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if decoded.Key != "bot-key" {
		t.Fatalf("unexpected key: %q", decoded.Key)
	}
	var decodedUpdate models.Update
	if err = json.Unmarshal(decoded.Body, &decodedUpdate); err != nil {
		t.Fatalf("unmarshal update: %v", err)
	}
	if decodedUpdate.ID != update.ID {
		t.Fatalf("unexpected update id: %d", decodedUpdate.ID)
	}
}

func TestGatewayLongScanBudgetDoesNotExtendOtherUpdates(t *testing.T) {
	ctx := context.Background()
	adapter := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
	const key = "youbanPublish.mediaSearch.queryTimeoutSeconds"
	original := g.Cfg().MustGet(ctx, key).Val()
	t.Cleanup(func() { _ = adapter.Set(key, original) })
	if err := adapter.Set(key, 600); err != nil {
		t.Fatal(err)
	}
	if scanconfig.SearchTimeout(ctx) != 10*time.Minute {
		t.Fatal("parent search budget still truncates the configured query")
	}
	for _, tc := range []struct {
		name   string
		update *models.Update
		want   time.Duration
	}{
		{"photo", &models.Update{Message: &models.Message{Chat: models.Chat{Type: "private"}, Photo: []models.PhotoSize{{FileID: "photo"}}}}, 630 * time.Second},
		{"video", &models.Update{Message: &models.Message{Chat: models.Chat{Type: "private"}, Video: &models.Video{}}}, 630 * time.Second},
		{"text", &models.Update{Message: &models.Message{Chat: models.Chat{Type: "private"}, Text: "hello"}}, 20 * time.Second},
		{"callback", &models.Update{CallbackQuery: &models.CallbackQuery{}}, 20 * time.Second},
		{"channel", &models.Update{ChannelPost: &models.Message{Chat: models.Chat{Type: "channel"}, Photo: []models.PhotoSize{{FileID: "photo"}}}}, 20 * time.Second},
		{"nil", nil, 20 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := gatewayMediaSearchTaskTimeout(ctx, tc.update, 20*time.Second); got != tc.want {
				t.Fatalf("timeout=%s want=%s", got, tc.want)
			}
		})
	}
	if err := adapter.Set(key, 5); err != nil {
		t.Fatal(err)
	}
	if got := gatewayMediaSearchTaskTimeout(ctx, &models.Update{Message: &models.Message{Chat: models.Chat{Type: "private"}, Video: &models.Video{}}}, 20*time.Second); got != 20*time.Second {
		t.Fatalf("default queue lease changed: %s", got)
	}
}

func TestGatewayQueueConcurrencyHasSafeMinimum(t *testing.T) {
	if gatewayBulkQueueConcurrency(context.Background()) < 1 {
		t.Fatal("gateway bulk queue concurrency must be positive")
	}
	if gatewayInteractiveQueueConcurrency(context.Background()) < 1 {
		t.Fatal("gateway interactive queue concurrency must be positive")
	}
}

func TestGatewayInteractiveQueueClassification(t *testing.T) {
	tests := []struct {
		name   string
		update *models.Update
		want   bool
	}{
		{name: "callback", update: &models.Update{CallbackQuery: &models.CallbackQuery{ID: "callback"}}, want: true},
		{name: "inline", update: &models.Update{InlineQuery: &models.InlineQuery{ID: "inline"}}, want: true},
		{name: "private message", update: &models.Update{Message: &models.Message{Chat: models.Chat{Type: "private"}}}, want: true},
		{name: "edited private message", update: &models.Update{EditedMessage: &models.Message{Chat: models.Chat{Type: "private"}}}, want: true},
		{name: "channel post", update: &models.Update{ChannelPost: &models.Message{Chat: models.Chat{Type: "channel"}}}, want: false},
		{name: "group message", update: &models.Update{Message: &models.Message{Chat: models.Chat{Type: "supergroup"}}}, want: false},
		{name: "nil", update: nil, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := updateNeedsInteractiveQueue(test.update); got != test.want {
				t.Fatalf("updateNeedsInteractiveQueue() = %t, want %t", got, test.want)
			}
		})
	}
}
