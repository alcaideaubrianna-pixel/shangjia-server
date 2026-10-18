package sys

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-telegram/bot/models"
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
