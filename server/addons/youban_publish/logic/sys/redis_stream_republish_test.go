package sys

import (
	"context"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"os"
	"testing"
)

func TestRestoreTelegramCleanupIntentPayload(t *testing.T) {
	got, err := restoreTelegramCleanupIntentPayload(
		42,
		7,
		"unpublish:10:abc",
		`{"jobId":7,"cleanupOperationNo":"unpublish:10:abc","cleanupJobIds":[7,8],"forceCompletion":true}`,
	)
	if err != nil {
		t.Fatalf("restore payload: %v", err)
	}
	if got.JobId != 7 || got.CleanupOutboxId != 42 || got.CleanupOperationNo != "unpublish:10:abc" {
		t.Fatalf("restored payload = %+v", got)
	}
	if len(got.CleanupJobIds) != 2 || got.CleanupJobIds[1] != 8 || !got.ForceCompletion {
		t.Fatalf("lost persisted cleanup context: %+v", got)
	}
}

func TestRestoreTelegramCleanupIntentPayloadRejectsRebinding(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "job", body: `{"jobId":8}`},
		{name: "outbox", body: `{"cleanupOutboxId":43}`},
		{name: "operation", body: `{"cleanupOperationNo":"unpublish:10:new"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := restoreTelegramCleanupIntentPayload(42, 7, "unpublish:10:abc", test.body); err == nil {
				t.Fatal("expected persisted cleanup context mismatch")
			}
		})
	}
}

func TestRestoreTelegramCleanupIntentPayloadRejectsLegacyIdentity(t *testing.T) {
	if _, err := restoreTelegramCleanupIntentPayload(0, 7, "", `{"jobId":7}`); err == nil {
		t.Fatal("expected legacy payload without outbox identity to be rejected")
	}
}

func TestCompleteTelegramCleanupIntentLegacyDoesNotAcknowledge(t *testing.T) {
	if err := (&sSysPublish{}).completeTelegramCleanupIntent(context.Background(), 7, tgQueuePayload{JobId: 7}); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreTelegramCleanupIntentLegacyPayload(t *testing.T) {
	got, err := restoreTelegramCleanupIntentPayload(42, 7, "", "")
	if err != nil || got.CleanupOperationNo != "" || got.CleanupOutboxId != 42 || got.JobId != 7 {
		t.Fatalf("legacy restore: %+v %v", got, err)
	}
	if _, err := restoreTelegramCleanupIntentPayload(42, 7, "", "{"); err == nil {
		t.Fatal("invalid persisted JSON accepted")
	}
}

func TestCompleteTelegramCleanupIntentExactIdentityPostgres(t *testing.T) {
	if os.Getenv("YOUBAN_CLEANUP_INTENT_TEST_PG") != "1" {
		t.Skip("requires disposable local PostgreSQL at 127.0.0.1:55497")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: "55497", User: "test", Name: "postgres"}})
	err := g.DB().Transaction(context.Background(), func(ctx context.Context, tx gdb.TX) error {
		// Temporary table shadows the real table only within this transaction.
		if _, err := tx.Exec(`CREATE TEMP TABLE hg_youban_publish_tg_outbox(id bigint PRIMARY KEY, job_id bigint, operation_no text, event_type text, status text, delivered_at timestamp, lease_owner text, lease_expires_at timestamp, last_error text, updated_at timestamp) ON COMMIT DROP`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_tg_outbox(id,job_id,operation_no,event_type,status) VALUES (41,7,'old','telegram_job.cleanup.manual','processing'),(42,7,'new','telegram_job.cleanup.manual','processing')`); err != nil {
			return err
		}
		service := &sSysPublish{}
		for _, payload := range []tgQueuePayload{{JobId: 7}, {JobId: 7, CleanupOutboxId: 42, CleanupOperationNo: "old"}} {
			if err := service.completeTelegramCleanupIntent(ctx, 7, payload); err != nil {
				return err
			}
		}
		count, err := tx.Model(publishTgOutboxTable).Where("status", telegramOutboxDelivered).Count()
		if err != nil {
			return err
		}
		if count != 0 {
			t.Fatalf("legacy/mismatched ack delivered %d intents", count)
		}
		old := tgQueuePayload{JobId: 7, CleanupOutboxId: 41, CleanupOperationNo: "old"}
		if err := service.completeTelegramCleanupIntent(ctx, 7, old); err != nil {
			return err
		}
		if err := service.completeTelegramCleanupIntent(ctx, 7, old); err != nil {
			return err
		}
		rows, err := tx.Model(publishTgOutboxTable).OrderAsc("id").All()
		if err != nil {
			return err
		}
		if rows[0]["status"].String() != telegramOutboxDelivered || rows[1]["status"].String() != telegramOutboxProcessing {
			t.Fatalf("old consumer changed new intent: %v", rows)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
