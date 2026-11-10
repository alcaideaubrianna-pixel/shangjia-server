package sys

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/hibiken/asynq"
)

func mediaOutboxLocalDB(t *testing.T) gdb.DB {
	t.Helper()
	if os.Getenv("YOUBAN_LOCAL_MEDIA_OUTBOX") != "1" {
		t.Skip("requires disposable PostgreSQL 35432 and Redis 16382 DB15")
	}
	before := gdb.GetConfig("default")
	t.Cleanup(func() { gdb.SetConfigGroup("default", before) })
	node := gdb.ConfigNode{Type: "pgsql", Host: "127.0.0.1", Port: "35432", User: os.Getenv("USER"), Name: "postgres"}
	root, err := gdb.New(node)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("media_outbox_%d", time.Now().UnixNano())
	if _, err = root.Exec(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = root.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
	node.Namespace = schema
	gdb.SetConfigGroup("default", gdb.ConfigGroup{node})
	db := g.DB()
	_, err = db.Exec(context.Background(), `CREATE TABLE hg_youban_publish_media(id bigint PRIMARY KEY,processing_status text,storage_path text,file_url text,attachment_id bigint,deleted_at timestamp,updated_at timestamp DEFAULT now())`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../resource/sql/media_process_outbox.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err = db.Exec(context.Background(), string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func mediaOutboxRedis(t *testing.T, ids ...int64) (*asynq.Client, *asynq.Inspector) {
	t.Helper()
	opt := asynq.RedisClientOpt{Addr: "127.0.0.1:16382", DB: 15}
	client := asynq.NewClient(opt)
	inspector := asynq.NewInspector(opt)
	t.Cleanup(func() {
		for _, id := range ids {
			for _, suffix := range []string{"", ":repair:1"} {
				_ = inspector.DeleteTask(tgQueueNameMediaProcess, fmt.Sprintf("media-process:%d%s", id, suffix))
			}
		}
		_ = inspector.Close()
		_ = client.Close()
	})
	return client, inspector
}

func TestMediaProcessOutboxAtomicLocalPostgres(t *testing.T) {
	db := mediaOutboxLocalDB(t)
	ctx := context.Background()
	_, err := db.Exec(ctx, `INSERT INTO hg_youban_publish_media(id,processing_status) VALUES(1,'uploaded')`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err = (&sSysPublish{}).enqueueMediaProcess(ctx, 1, 0); err != nil {
			t.Fatal(err)
		}
	}
	row, err := db.GetOne(ctx, `SELECT generation,status FROM hg_youban_publish_media_process_outbox WHERE media_id=1`)
	if err != nil || row["generation"].Int() != 1 || row["status"].String() != "pending" {
		t.Fatalf("requests did not coalesce: %v %v", row, err)
	}
	want := errors.New("rollback")
	err = db.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Exec(`INSERT INTO hg_youban_publish_media(id,processing_status) VALUES(2,'uploaded')`); err != nil {
			return err
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
	n, err := db.GetValue(ctx, `SELECT COUNT(*) FROM hg_youban_publish_media_process_outbox WHERE media_id=2`)
	if err != nil || n.Int() != 0 {
		t.Fatalf("rollback left orphan intent: %v %v", n, err)
	}
	if _, err = db.Exec(ctx, `UPDATE hg_youban_publish_media SET processing_status='retry' WHERE id=1; UPDATE hg_youban_publish_media_process_outbox SET status='sent' WHERE media_id=1; UPDATE hg_youban_publish_media SET processing_status='processing' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	row, err = db.GetOne(ctx, `SELECT status,generation FROM hg_youban_publish_media_process_outbox WHERE media_id=1`)
	if err != nil || row["status"].String() != "sent" || row["generation"].Int() != 1 {
		t.Fatalf("queue retry recreated intent: %v %v", row, err)
	}
	if _, err = db.Exec(ctx, `UPDATE hg_youban_publish_media SET processing_status='uploaded' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	row, err = db.GetOne(ctx, `SELECT status,generation FROM hg_youban_publish_media_process_outbox WHERE media_id=1`)
	if err != nil || row["status"].String() != "pending" || row["generation"].Int() != 2 {
		t.Fatalf("explicit repair not durable: %v %v", row, err)
	}
	if _, err = db.Exec(ctx, `UPDATE hg_youban_publish_media_process_outbox SET status='leased',bootstrap=true,task_id='old',lease_token='old' WHERE media_id=1`); err != nil {
		t.Fatal(err)
	}
	if err = (&sSysPublish{}).enqueueMediaProcess(ctx, 1, 0); err != nil {
		t.Fatal(err)
	}
	row, err = db.GetOne(ctx, `SELECT generation,status,bootstrap,task_id FROM hg_youban_publish_media_process_outbox WHERE media_id=1`)
	if err != nil || row["generation"].Int() != 3 || row["status"].String() != "pending" || row["bootstrap"].Bool() || row["task_id"].String() != "" {
		t.Fatalf("explicit request lost behind bootstrap lease: %v %v", row, err)
	}
}

func TestMediaProcessOutboxCrashAndRedeliveryLocalPostgres(t *testing.T) {
	db := mediaOutboxLocalDB(t)
	ctx := context.Background()
	id := time.Now().UnixNano()
	client, inspector := mediaOutboxRedis(t, id)
	if _, err := db.Exec(ctx, `INSERT INTO hg_youban_publish_media(id,processing_status) VALUES(?,'uploaded')`, id); err != nil {
		t.Fatal(err)
	}
	claimed, err := claimMediaProcessDeliveries(ctx, "worker-a")
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: %v %v", claimed, err)
	}
	if _, _, err = deliverMediaProcessIntent(ctx, claimed[0], client, inspector); err != nil {
		t.Fatal(err)
	}
	// Crash after Redis accepted the task but before PostgreSQL acknowledged it.
	if _, err = db.Exec(ctx, `UPDATE hg_youban_publish_media_process_outbox SET available_at=clock_timestamp()-interval '1 second' WHERE media_id=?`, id); err != nil {
		t.Fatal(err)
	}
	newClaim, err := claimMediaProcessDeliveries(ctx, "worker-b")
	if err != nil || len(newClaim) != 1 {
		t.Fatalf("expired lease was not reclaimed: %v %v", newClaim, err)
	}
	if _, _, err = deliverMediaProcessIntent(ctx, claimed[0], client, inspector); err == nil {
		t.Fatal("stale worker changed frozen task ID")
	}
	if err = inspector.ArchiveTask(tgQueueNameMediaProcess, newClaim[0].TaskID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE hg_youban_publish_media_process_outbox SET available_at=clock_timestamp()-interval '1 second' WHERE media_id=?`, id); err != nil {
		t.Fatal(err)
	}
	err = dispatchMediaProcessDeliveries(ctx, func(ctx context.Context, row mediaProcessDelivery) (string, string, error) {
		return deliverMediaProcessIntent(ctx, row, client, inspector)
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := inspector.GetTaskInfo(tgQueueNameMediaProcess, newClaim[0].TaskID)
	if err != nil || info.State != asynq.TaskStateArchived {
		t.Fatalf("redelivery revived exhausted task: %+v %v", info, err)
	}
	if err = (&sSysPublish{}).enqueueMediaProcess(ctx, id, 0); err != nil {
		t.Fatal(err)
	}
	err = dispatchMediaProcessDeliveries(ctx, func(ctx context.Context, row mediaProcessDelivery) (string, string, error) {
		return deliverMediaProcessIntent(ctx, row, client, inspector)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = inspector.GetTaskInfo(tgQueueNameMediaProcess, fmt.Sprintf("media-process:%d:repair:1", id)); err != nil {
		t.Fatal("explicit repair was lost", err)
	}
}

func TestMediaProcessOutboxConcurrentClaimsLocalPostgres(t *testing.T) {
	db := mediaOutboxLocalDB(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `INSERT INTO hg_youban_publish_media(id,processing_status) SELECT n,'uploaded' FROM generate_series(1,100) n`); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	seen := map[int64]bool{}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rows, err := claimMediaProcessDeliveries(ctx, fmt.Sprint(i))
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, row := range rows {
				if seen[row.MediaID] {
					t.Errorf("double claim %d", row.MediaID)
				}
				seen[row.MediaID] = true
			}
		}(i)
	}
	wg.Wait()
	if len(seen) != 100 {
		t.Fatalf("claimed %d rows", len(seen))
	}
	if err := dispatchMediaProcessDeliveries(ctx, func(context.Context, mediaProcessDelivery) (string, string, error) {
		t.Fatal("unexpired lease claimed")
		return "sent", "", nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO hg_youban_publish_media_process_outbox(media_id,status) SELECT -n,'sent' FROM generate_series(1,50000) n; INSERT INTO hg_youban_publish_media_process_outbox(media_id,status,available_at) SELECT n,'pending',clock_timestamp()+interval '1 year' FROM generate_series(100001,150000) n; ANALYZE hg_youban_publish_media_process_outbox`); err != nil {
		t.Fatal(err)
	}
	plan, err := db.GetAll(ctx, `EXPLAIN SELECT media_id FROM hg_youban_publish_media_process_outbox WHERE status IN ('pending','leased') AND available_at<=statement_timestamp() ORDER BY available_at,media_id LIMIT 50 FOR UPDATE SKIP LOCKED`)
	if err != nil || (!strings.Contains(fmt.Sprint(plan), "idx_ybp_media_process_outbox_due") || !strings.Contains(fmt.Sprint(plan), "Index Cond")) {
		t.Fatalf("due index unused: %v %v", plan, err)
	}
}

func TestMediaProcessOutboxRedisFailureLocalPostgres(t *testing.T) {
	db := mediaOutboxLocalDB(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `INSERT INTO hg_youban_publish_media(id,processing_status) VALUES(1,'uploaded')`); err != nil {
		t.Fatal(err)
	}
	want := errors.New("Redis unavailable")
	if err := dispatchMediaProcessDeliveries(ctx, func(context.Context, mediaProcessDelivery) (string, string, error) { return "", "", want }); !errors.Is(err, want) {
		t.Fatal(err)
	}
	row, err := db.GetOne(ctx, `SELECT status,attempt_count,available_at>clock_timestamp() AS delayed FROM hg_youban_publish_media_process_outbox WHERE media_id=1`)
	if err != nil || row["status"].String() != "pending" || !row["delayed"].Bool() || row["attempt_count"].Int() != 1 {
		t.Fatalf("failed delivery was lost: %v %v", row, err)
	}
}

func TestMediaProcessOutboxLegacyCutoverLocalPostgres(t *testing.T) {
	db := mediaOutboxLocalDB(t)
	ctx := context.Background()
	base := time.Now().UnixNano()
	client, inspector := mediaOutboxRedis(t, base, base+1)
	for i := int64(0); i < 2; i++ {
		id := base + i
		if _, err := db.Exec(ctx, `INSERT INTO hg_youban_publish_media(id,processing_status,updated_at) VALUES(?,'retry',clock_timestamp()-interval '2 minutes')`, id); err != nil {
			t.Fatal(err)
		}
		if err := enqueueMediaProcessTask(ctx, client, id, fmt.Sprintf("media-process:%d", id), 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := inspector.ArchiveTask(tgQueueNameMediaProcess, fmt.Sprintf("media-process:%d", base+1)); err != nil {
		t.Fatal(err)
	}
	backfill, err := os.ReadFile("../../resource/sql/media_process_outbox_backfill.pgsql.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, string(backfill)); err != nil {
		t.Fatal(err)
	}
	if err = dispatchMediaProcessDeliveries(ctx, func(ctx context.Context, row mediaProcessDelivery) (string, string, error) {
		return deliverMediaProcessIntent(ctx, row, client, inspector)
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.GetAll(ctx, `SELECT media_id,status FROM hg_youban_publish_media_process_outbox ORDER BY media_id`)
	if err != nil || len(rows) != 2 || rows[0]["status"].String() != "sent" || rows[1]["status"].String() != "blocked" {
		t.Fatalf("legacy policy reset: %v %v", rows, err)
	}
}
