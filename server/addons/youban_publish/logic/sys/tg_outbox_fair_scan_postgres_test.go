package sys

import (
	"context"
	"testing"
)

// Opt-in only: this test uses the disposable loopback PostgreSQL fixture and
// executes the exact fair-scan SQL, including its anonymous result mapping.
func TestTelegramOutboxFairScanPostgresKeysetMapping(t *testing.T) {
	db := telegramOutboxRetryLocalDB(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `CREATE TABLE `+publishTgJobTable+` (
		id bigint PRIMARY KEY, account_id bigint, tenant_id bigint, channel_id bigint,
		priority integer, status text, dispatch_status text, asynq_task_id text,
		next_retry_at timestamp)`); err != nil {
		t.Fatal(err)
	}
	when := "2026-11-17 19:00:00.123456"
	for id := int64(1); id <= 3; id++ {
		if _, err := db.Exec(ctx, `INSERT INTO `+publishTgJobTable+` VALUES (?,?,?,?,?,?,?,?,?)`, id, 1, 1, id, 1, "pending", "idle", "", nil); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO `+publishTgOutboxTable+` (id,job_id,event_type,available_at,status,lease_expires_at,attempt_count) VALUES (?,?,?,?,?,?,?)`, id, id, telegramOutboxEventPublish, when, telegramOutboxPending, nil, 0); err != nil {
			t.Fatal(err)
		}
	}
	fence := "2026-11-17 19:01:00.000000"
	query, args := telegramOutboxFairScanSQL(telegramOutboxFairCursor{Fence: fence}, fence, 10, "pgsql")
	if got := len(args); got != 7 {
		t.Fatalf("first page args=%d, want 7", got)
	}
	var rows []telegramOutboxFairCandidate
	if err := db.GetScan(ctx, &rows, query, args...); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].Id != 1 || rows[2].Id != 3 {
		t.Fatalf("first page rows=%+v", rows)
	}
	query, args = telegramOutboxFairScanSQL(telegramOutboxFairCursor{Fence: fence, At: when, ID: 1}, fence, 10, "pgsql")
	if got := len(args); got != 9 {
		t.Fatalf("keyset args=%d, want 9", got)
	}
	rows = nil
	if err := db.GetScan(ctx, &rows, query, args...); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Id != 2 || rows[1].Id != 3 || rows[0].CursorAt != when {
		t.Fatalf("keyset/mapping rows=%+v", rows)
	}
	if rows[0].AvailableAt == nil || rows[0].AvailableAt.String() == "" {
		t.Fatal("anonymous embedded available_at was not mapped")
	}
}
