package sys

import (
	"context"
	"os"
	"testing"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

func TestCleanupIntentImmutablePayload(t *testing.T) {
	row := telegramCleanupIntent{Id: 91, JobId: 10, EventType: telegramOutboxCleanupPrefix + "manual", OperationNo: "unpublish:A", Payload: `{"jobId":10,"cleanupOperationNo":"unpublish:A","cleanupJobIds":[10,11],"forceCompletion":true}`}
	payload, err := decodeTelegramCleanupIntentPayload(row)
	if err != nil || payload.CleanupOperationNo != "unpublish:A" || !payload.ForceCompletion || len(payload.CleanupJobIds) != 2 {
		t.Fatalf("restore: %+v %v", payload, err)
	}
	row.Payload = `{"jobId":10,"cleanupOperationNo":"unpublish:B"}`
	if _, err = decodeTelegramCleanupIntentPayload(row); err == nil {
		t.Fatal("must reject changed operation")
	}
	row.Payload = `{"jobId":11}`
	if _, err = decodeTelegramCleanupIntentPayload(row); err == nil {
		t.Fatal("must reject changed job")
	}
	row.OperationNo = ""
	row.Payload = ""
	payload, err = decodeTelegramCleanupIntentPayload(row)
	if err != nil || payload.CleanupOperationNo != "" || payload.JobId != 10 {
		t.Fatalf("legacy rebound: %+v %v", payload, err)
	}
	row.Payload = `{broken`
	if _, err = decodeTelegramCleanupIntentPayload(row); err == nil {
		t.Fatal("corrupt payload accepted")
	}
	a := telegramCleanupIntentDedupeKey(10, row.EventType, "A")
	b := telegramCleanupIntentDedupeKey(10, row.EventType, "B")
	if a == b || len(a) > 160 {
		t.Fatal("operation dedupe invalid")
	}
	if got := telegramCleanupIntentDedupeKey(10, row.EventType, ""); got != "telegram-cleanup:10" {
		t.Fatal(got)
	}
}

// Uses a disposable database only; never point this setting at production.
func TestCleanupIntentPersistence(t *testing.T) {
	link := os.Getenv("YOUBAN_CLEANUP_INTENT_TEST_LINK")
	if link == "" {
		t.Skip("set YOUBAN_CLEANUP_INTENT_TEST_LINK to a disposable PostgreSQL database")
	}
	previous, _ := gdb.GetConfigGroup("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", previous) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Link: link}})
	ctx := context.Background()
	db := g.DB()
	_, err := db.Exec(ctx, `CREATE TABLE hg_youban_publish_tg_outbox (id bigserial PRIMARY KEY,job_id bigint NOT NULL,event_type varchar(64) NOT NULL,dedupe_key varchar(160) UNIQUE NOT NULL,status varchar(16) DEFAULT 'pending',available_at timestamp,lease_owner text DEFAULT '',lease_expires_at timestamp,attempt_count int DEFAULT 0,last_error text DEFAULT '',delivered_at timestamp,created_at timestamp,updated_at timestamp)`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../resource/sql/cleanup_intent_context.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = db.Exec(ctx, string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	event := telegramOutboxCleanupPrefix + "manual"
	input := tgQueuePayload{JobId: 10, CleanupOperationNo: "A", CleanupJobIds: []int64{10, 11}, ForceCompletion: true}
	id, created, err := ensureTelegramCleanupIntent(ctx, 10, event, "A", input)
	if err != nil || !created || id <= 0 {
		t.Fatalf("insert: %d %v %v", id, created, err)
	}
	if _, err = db.Exec(ctx, `UPDATE hg_youban_publish_tg_outbox SET status='delivered' WHERE id=$1`, id); err != nil {
		t.Fatal(err)
	}
	same, created, err := ensureTelegramCleanupIntent(ctx, 10, event, "A", input)
	if err != nil || created || same != id {
		t.Fatalf("dedupe: %d %v %v", same, created, err)
	}
	input.CleanupOperationNo = "B"
	next, _, err := ensureTelegramCleanupIntent(ctx, 10, event, "B", input)
	if err != nil || next == id {
		t.Fatalf("new operation: %d %v", next, err)
	}
	restored, err := hydrateTelegramCleanupIntentPayload(ctx, id)
	if err != nil || restored.CleanupOperationNo != "A" || !restored.ForceCompletion {
		t.Fatalf("old intent rebound: %+v %v", restored, err)
	}
	status, err := db.Model(publishTgOutboxTable).Ctx(ctx).Where("id", id).Value("status")
	if err != nil || status.String() != "delivered" {
		t.Fatalf("delivered reactivated: %v %v", status, err)
	}
	input.ForceCompletion = false
	if _, _, err = ensureTelegramCleanupIntent(ctx, 10, event, "B", input); err == nil {
		t.Fatal("changed context accepted")
	}
	if _, err = hydrateTelegramCleanupIntentPayload(ctx, 999999); err == nil {
		t.Fatal("missing intent accepted")
	}
	legacy, _, err := ensureTelegramCleanupIntent(ctx, 20, event, "", tgQueuePayload{})
	if err != nil {
		t.Fatal(err)
	}
	restored, err = hydrateTelegramCleanupIntentPayload(ctx, legacy)
	if err != nil || restored.CleanupOperationNo != "" {
		t.Fatalf("legacy: %+v %v", restored, err)
	}
}
