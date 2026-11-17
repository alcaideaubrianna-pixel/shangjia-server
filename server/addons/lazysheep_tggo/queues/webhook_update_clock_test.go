package queues

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/internal/library/collectobserve"
)

func TestWebhookDiskQueueClockPreservesBodyAndReceipt(t *testing.T) {
	received := time.Unix(1800000000, 123456789)
	body := []byte(`{"update_id":42,"message":{"date":123}}`)
	task := sysin.WebhookUpdateTask{BotKey: "test", Payload: body, ReceivedAtUnixNano: received.UnixNano()}
	raw, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	var restored sysin.WebhookUpdateTask
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	ctx := collectobserve.RestoreIngressClock(context.Background(), restored.ReceivedAtUnixNano, "webhook_received", received.Add(24*time.Hour))
	meta := collectobserve.From(ctx)
	if string(restored.Payload) != string(body) || !meta.ReceivedAt.Equal(received) || meta.TimeBasis != "webhook_received" {
		t.Fatalf("diskqueue changed body/receipt: body=%s meta=%+v", restored.Payload, meta)
	}
	var legacy sysin.WebhookUpdateTask
	if err := json.Unmarshal([]byte(`{"botKey":"test","payload":"e30="}`), &legacy); err != nil {
		t.Fatal(err)
	}
	legacyCtx := collectobserve.RestoreIngressClock(ctx, legacy.ReceivedAtUnixNano, "webhook_received", received.Add(time.Hour))
	if !collectobserve.From(legacyCtx).ReceivedAt.IsZero() || collectobserve.From(legacyCtx).TimeBasis != "" {
		t.Fatal("legacy diskqueue task must not inherit/manufacture webhook receipt")
	}
}
