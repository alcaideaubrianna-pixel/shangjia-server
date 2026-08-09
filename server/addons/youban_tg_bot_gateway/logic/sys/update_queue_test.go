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
	if gatewayQueueConcurrency(context.Background()) < 1 {
		t.Fatal("gateway queue concurrency must be positive")
	}
}
