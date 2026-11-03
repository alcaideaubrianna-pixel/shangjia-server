package sys

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

// Production metadata only: two enabled source-78 events were media_pending
// while their processing leases had expired on 2026-10-29. Never use live DB.
func TestObservedExpiredOutboxSurvivesQueueFailureLocalPostgres(t *testing.T) {
	port := os.Getenv("YOUBAN_DELIVERY_TEST_PORT")
	if port == "" {
		t.Skip("requires disposable loopback PostgreSQL")
	}
	_ = g.DB()
	old := gdb.GetConfig("default")
	defer gdb.SetConfigGroup("default", old)
	gdb.SetConfigGroup("default", gdb.ConfigGroup{{Type: "pgsql", Host: "127.0.0.1", Port: port, User: os.Getenv("USER"), Name: "postgres"}})
	ctx := context.Background()
	_ = g.DB().GetCore().ClearTableFieldsAll(ctx)
	defer g.DB().GetCore().ClearTableFieldsAll(ctx)
	redis := miniredis.RunT(t)
	opt := asynq.RedisClientOpt{Addr: redis.Addr()}
	broken := asynq.NewClient(opt)
	_ = broken.Close()
	client := asynq.NewClient(opt)
	defer client.Close()
	inspector := asynq.NewInspector(opt)
	defer inspector.Close()
	s := &sSysPublish{tgQueueClient: broken}
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, sql := range []string{
			createRealtimeOutboxTestSQL,
			`CREATE TEMP TABLE hg_youban_publish_collect_event(id bigint,received_at timestamp,created_at timestamp) ON COMMIT DROP`,
			`INSERT INTO hg_youban_publish_collect_event VALUES(2651517,'2026-09-10 23:50:54','2026-09-19 01:53:40.714834'),(2651648,'2026-09-11 00:03:40','2026-09-19 01:55:09.048239')`,
			`INSERT INTO hg_youban_publish_collect_process_outbox(id,event_id,tenant_id,account_id,source_id,status,available_at,created_at,updated_at,lease_expires_at) VALUES(10650733,2651517,25,473,78,'processing','2026-10-29 08:23:37','2026-10-29 08:23:37','2026-10-29 08:29:26','2026-10-29 08:34:26'),(10650755,2651648,25,473,78,'processing','2026-10-29 08:23:38','2026-10-29 08:23:38','2026-10-29 08:29:26','2026-10-29 08:34:26')`,
		} {
			if _, err := tx.Exec(sql); err != nil {
				return err
			}
		}
		if err := s.dispatchDueCollectProcessOutbox(ctx, 10); err != nil {
			return err
		}
		rows, err := tx.GetAll(`SELECT status,attempt_count,lease_owner,last_error FROM hg_youban_publish_collect_process_outbox ORDER BY id`)
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			t.Fatal("lost original task")
		}
		for _, row := range rows {
			if row["status"].String() != "pending" || row["attempt_count"].Int() != 1 || row["lease_owner"].String() != "" || row["last_error"].String() == "" {
				t.Fatal("enqueue failure did not leave durable retryable state")
			}
		}
		// Dependency recovers. No business row or queue is cleared/reset; only the
		// isolated test clock deadline is advanced so the retry is due now.
		if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_process_outbox SET available_at=LOCALTIMESTAMP-INTERVAL '1 second'`); err != nil {
			return err
		}
		s.tgQueueClient = client
		if err := s.dispatchDueCollectProcessOutbox(ctx, 10); err != nil {
			return err
		}
		tasks, err := inspector.ListPendingTasks(tgQueueNameCollectProcessRecoveryV2)
		if err != nil {
			return err
		}
		if len(tasks) != 2 {
			t.Fatalf("recovered task count=%d", len(tasks))
		}
		before, err := tx.GetAll(`SELECT id,lease_owner FROM hg_youban_publish_collect_process_outbox ORDER BY id`)
		if err != nil {
			return err
		}
		for _, row := range before {
			if !strings.HasPrefix(row["lease_owner"].String(), "recovery-") {
				t.Fatal("historical sample leaked into realtime")
			}
		}
		// Simulate a dispatcher/worker interruption after Redis accepted the task
		// and before processing ACK. Expired handoff must retain the task ID.
		for i := 0; i < 3; i++ {
			if _, err := tx.Exec(`UPDATE hg_youban_publish_collect_process_outbox SET lease_expires_at=LOCALTIMESTAMP-INTERVAL '1 second'`); err != nil {
				return err
			}
			if err := s.dispatchDueCollectProcessOutbox(ctx, 10); err != nil {
				return err
			}
		}
		tasks, err = inspector.ListPendingTasks(tgQueueNameCollectProcessRecoveryV2)
		if err != nil {
			return err
		}
		if len(tasks) != 2 {
			t.Fatal("interrupted handoff duplicated queue tasks")
		}
		after, err := tx.GetAll(`SELECT id,lease_owner FROM hg_youban_publish_collect_process_outbox ORDER BY id`)
		if err != nil {
			return err
		}
		for i, row := range after {
			if row["lease_owner"].String() != before[i]["lease_owner"].String() {
				t.Fatal("interrupted handoff changed generation")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
