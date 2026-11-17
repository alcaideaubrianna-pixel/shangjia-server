package sys

import (
	"context"
	"testing"
	"time"

	"hotgo/internal/library/collectobserve"

	"github.com/gogf/gf/v2/container/gvar"
	"github.com/gogf/gf/v2/database/gdb"
)

func TestCollectBusinessEventContextKeepsBranchIdentityIsolated(t *testing.T) {
	receivedAt := time.Unix(1794850000, 0)
	ingress := collectobserve.With(context.Background(), collectobserve.Metadata{
		Mode: "history", ReceivedAt: receivedAt,
	})
	event := gdb.Record{
		"id": gvar.New(18990346), "tenant_id": gvar.New(78),
		"account_id": gvar.New(0), "source_id": gvar.New(351),
		"source_type": gvar.New("bot"), "source_chat_id": gvar.New("-1003763589169"),
		"bot_id": gvar.New(0), "tg_account_id": gvar.New(0),
	}
	source := gdb.Record{
		"account_id": gvar.New(615), "bot_id": gvar.New(109), "source_type": gvar.New("bot"),
	}
	observation, err := collectobserve.StampPayload(ingress, []byte(`{}`), receivedAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	event["media_json"] = gvar.New(string(observation))
	raw := collectBusinessEventContext(ingress, event, nil, 0)
	business := collectBusinessEventContext(raw, event, source, 0)
	branchA := collectBusinessEventContext(business, event, nil, 338)
	branchB := collectBusinessEventContext(business, event, nil, 339)
	branchA = collectobserve.With(branchA, collectobserve.Metadata{ProfileID: 813124})

	for _, item := range []struct {
		name                   string
		ctx                    context.Context
		account, rule, profile int64
	}{
		{"raw", raw, 0, 0, 0},
		{"business", business, 615, 0, 0},
		{"branch_a", branchA, 615, 338, 813124},
		{"branch_b", branchB, 615, 339, 0},
	} {
		meta := collectobserve.From(item.ctx)
		if meta.AccountID != item.account || meta.RuleID != item.rule || meta.ProfileID != item.profile {
			t.Fatalf("%s identity leaked: %+v", item.name, meta)
		}
		if meta.EventID != 18990346 || meta.SourceID != 351 || meta.TenantID != 78 ||
			meta.ChatID != "-1003763589169" || meta.Mode != "history" || !meta.ReceivedAt.Equal(receivedAt) {
			t.Fatalf("%s event or ingress metadata changed: %+v", item.name, meta)
		}
		if item.account > 0 && meta.BotID != 109 {
			t.Fatalf("%s lost source bot identity: %+v", item.name, meta)
		}
	}
	if event["account_id"].Int64() != 0 || source["account_id"].Int64() != 615 {
		t.Fatal("observation binding mutated business records")
	}
	if meta := collectobserve.From(ingress); meta.AccountID != 0 || meta.EventID != 0 || meta.RuleID != 0 {
		t.Fatalf("ingress context mutated: %+v", meta)
	}
}
