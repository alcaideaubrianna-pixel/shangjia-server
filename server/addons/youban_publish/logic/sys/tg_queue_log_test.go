package sys

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gogf/gf/v2/frame/g"
)

func TestTelegramQueueLogEnvelopeAndPrivacy(t *testing.T) {
	base := g.Map{"jobId": int64(41), "channelId": int64(2), "token": "private-token"}
	ctx := context.WithValue(context.Background(), telegramPublishTraceContextKey{}, base)
	fields := g.Map{
		"channelId": int64(3), "stage": "claim", "durationMs": int64(12),
		"token": "private-token", "password": "private-password", "body": "private-media",
		"pending": []string{"private-payload"}, "log_type": "overridden", "event": "overridden",
		"traceId": "forged", "schemaVersion": 99,
	}
	data := telegramQueueLogFields(ctx, "channel_claim", fields, errors.New("POST https://api.telegram.org/bot123456789:ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghi/sendMediaGroup: timeout"))
	if data["log_type"] != "telegram_queue" || data["event"] != "channel_claim" || data["schemaVersion"] != 1 {
		t.Fatalf("invalid envelope: %+v", data)
	}
	if data["jobId"] != int64(41) || data["channelId"] != int64(3) || data["durationMs"] != int64(12) {
		t.Fatalf("missing correlation fields: %+v", data)
	}
	if data["result"] != "error" || data["instance"] == nil || data["pid"] == nil || data["traceId"] == "forged" {
		t.Fatalf("invalid execution metadata: %+v", data)
	}
	payload, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-token", "private-password", "private-media", "private-payload", "api.telegram.org", "ABCDEFGHIJKLMNOPQRSTUVWXYZ_abcdefghi"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("sensitive field leaked: %s", secret)
		}
	}
	if base["channelId"] != int64(2) || fields["schemaVersion"] != 99 {
		t.Fatal("logging mutated caller-owned maps")
	}
}

func TestTelegramQueueLogClaimIdentity(t *testing.T) {
	first := telegramQueueClaimID("first-secret-claim")
	if len(first) != 16 || first != telegramQueueClaimID("first-secret-claim") || first == telegramQueueClaimID("second-secret-claim") {
		t.Fatalf("invalid diagnostic claim identity: %q", first)
	}
	if telegramQueueClaimID("") != "" {
		t.Fatal("empty claim must not create an identity")
	}
}

func TestTelegramQueueLogDoesNotEquateBusyWithErrorOrSent(t *testing.T) {
	for _, result := range []string{"busy", "lost", "rejected", "cancelled", "requeued"} {
		fields := telegramQueueLogFields(context.Background(), "lease", g.Map{"result": result}, nil)
		if fields["result"] != result || fields["error"] != nil {
			t.Fatalf("operation outcome was misclassified: %+v", fields)
		}
	}
}
